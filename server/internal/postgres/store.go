// Package postgres implements durable request management using PostgreSQL transactions.
package postgres

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"linha/server/internal/domain"
	"linha/server/internal/pathspec"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schema string

//go:embed controller.sql
var controllerSchema string

//go:embed datasets.sql
var datasetSchema string

//go:embed operations.sql
var operationsSchema string

//go:embed admin.sql
var adminSchema string

//go:embed metrics.sql
var metricsSchema string

//go:embed overview.sql
var overviewSchema string

type Store struct {
	Pool  *pgxpool.Pool
	Lease time.Duration
}

func Open(ctx context.Context, dsn string, tracer ...pgx.QueryTracer) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if len(tracer) > 0 {
		cfg.ConnConfig.Tracer = tracer[0]
	}
	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	s := &Store{p, 30 * time.Second}
	if err = s.migrate(ctx); err != nil {
		p.Close()
		return nil, err
	}
	return s, nil
}
func (s *Store) Close() { s.Pool.Close() }
func (s *Store) Ping(ctx context.Context) error {
	var version int
	if err := s.Pool.QueryRow(ctx, "SELECT COALESCE(max(version),0) FROM linha_schema").Scan(&version); err != nil {
		return err
	}
	if version != 10 {
		return fmt.Errorf("database schema %d is incompatible with this server", version)
	}
	return nil
}
func (s *Store) migrate(ctx context.Context) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(782416912)"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "CREATE TABLE IF NOT EXISTS linha_schema(version integer PRIMARY KEY)"); err != nil {
			return err
		}
		var version int
		if err := tx.QueryRow(ctx, "SELECT COALESCE(max(version),0) FROM linha_schema").Scan(&version); err != nil {
			return err
		}
		if version > 10 {
			return fmt.Errorf("unsupported database schema %d", version)
		}
		if version == 10 {
			return nil
		}
		if version == 0 {
			if _, err := tx.Exec(ctx, schema); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, "INSERT INTO linha_schema(version) VALUES(1)"); err != nil {
				return err
			}
		}
		if version < 2 {
			if _, err := tx.Exec(ctx, controllerSchema); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, "INSERT INTO linha_schema(version) VALUES(2)"); err != nil {
				return err
			}
		}
		if version < 3 {
			if _, err := tx.Exec(ctx, "ALTER TABLE linha_contexts ADD COLUMN reconcile_after timestamptz NOT NULL DEFAULT '-infinity'; ALTER TABLE linha_contexts ADD COLUMN startup_failures integer NOT NULL DEFAULT 0"); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, "INSERT INTO linha_schema(version) VALUES(3)"); err != nil {
				return err
			}
		}
		if version < 4 {
			if _, err := tx.Exec(ctx, "ALTER TABLE linha_contexts ADD COLUMN requested_spec jsonb; INSERT INTO linha_schema(version) VALUES(4)"); err != nil {
				return err
			}
		}
		if version < 5 {
			if err := migrateVersions(ctx, tx); err != nil {
				return err
			}
		}
		if version < 6 {
			if _, err := tx.Exec(ctx, datasetSchema); err != nil {
				return err
			}
		}
		if version < 7 {
			if _, err := tx.Exec(ctx, operationsSchema); err != nil {
				return err
			}
		}
		if version < 8 {
			if _, err := tx.Exec(ctx, metricsSchema); err != nil {
				return err
			}
		}
		if version < 9 {
			if _, err := tx.Exec(ctx, adminSchema); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, overviewSchema)
		return err
	})
}
func (s *Store) tx(ctx context.Context, f func(pgx.Tx) error) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = f(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func missing(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NotFound
	}
	return err
}
func scanContext(row pgx.Row) (domain.BackendContext, error) {
	var c domain.BackendContext
	var b []byte
	err := row.Scan(&c.ID, &c.Owner, &c.Name, &b, &c.State, &c.Condition, &c.CreatedAt, &c.Version)
	if err != nil {
		return c, missing(err)
	}
	err = json.Unmarshal(b, &c.Spec)
	return c, err
}

const contextCols = "id,owner,name,spec,state,condition,created_at,version"

func (s *Store) Ensure(ctx context.Context, owner string, in domain.EnsureRequest) (out domain.BackendContext, err error) {
	if in.Name == "" || len(in.Name) > 128 {
		return out, domain.Bad("context name must contain 1-128 bytes")
	}
	if in.ClientID == "" {
		in.ClientID = domain.ID()
	}
	if len(in.ClientID) > 128 {
		return out, domain.Bad("clientId exceeds 128 bytes")
	}
	if err = validateHostname(in.Hostname); err != nil {
		return out, err
	}
	requested := in.Spec
	if in.RequestedSpec != nil {
		requested = *in.RequestedSpec
	}
	version := domain.ConfigVersion(in.Spec)
	err = s.tx(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "INSERT INTO linha_contexts(id,owner,name,spec,requested_spec,version) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(owner,name,version) DO NOTHING", domain.ID(), owner, in.Name, domain.JSON(in.Spec), domain.JSON(requested), version)
		if e != nil {
			return e
		}
		out, e = scanContext(tx.QueryRow(ctx, "SELECT "+contextCols+" FROM linha_contexts WHERE owner=$1 AND name=$2 AND version=$3 FOR NO KEY UPDATE", owner, in.Name, version))
		if e != nil {
			return e
		}
		out.ClientLease, e = attachClient(ctx, tx, out.ID, in.ClientID, in.Hostname)
		if out.State == "STOPPED" || out.State == "DRAINING" {
			out.State = "STARTING"
		}
		return e
	})
	return
}
func (s *Store) Context(ctx context.Context, owner, id string) (domain.BackendContext, error) {
	return scanContext(s.Pool.QueryRow(ctx, "SELECT "+contextCols+" FROM linha_contexts WHERE id=$1 AND owner=$2", id, owner))
}

const jobCols = "id,context_id,request,state,submitted_at,updated_at,attempt_count,progress,failure,result"

func scanJob(row pgx.Row) (domain.Job, error) {
	var j domain.Job
	var req, p, f, r []byte
	err := row.Scan(&j.ID, &j.ContextID, &req, &j.State, &j.SubmittedAt, &j.UpdatedAt, &j.AttemptCount, &p, &f, &r)
	if err != nil {
		return j, missing(err)
	}
	if err = json.Unmarshal(req, &j.Request); err != nil {
		return j, err
	}
	if len(p) > 0 {
		if err = json.Unmarshal(p, &j.Progress); err != nil {
			return j, err
		}
	}
	if len(f) > 0 {
		if err = json.Unmarshal(f, &j.Failure); err != nil {
			return j, err
		}
	}
	if len(r) > 0 {
		err = json.Unmarshal(r, &j.Result)
	}
	return j, err
}
func (s *Store) Submit(ctx context.Context, owner, contextID string, in domain.SubmitRequest) (out domain.Job, err error) {
	if in.Handler == "" || len(in.Handler) > 256 || in.Version < 1 || in.IdempotencyKey == "" || len(in.IdempotencyKey) > 256 || !json.Valid(in.Payload) {
		return out, domain.Bad("handler, version, JSON payload and idempotency key are required")
	}
	if in.Retry.MaxAttempts == 0 {
		in.Retry.MaxAttempts = 1
	}
	if in.Retry.BackoffSeconds == 0 {
		in.Retry.BackoffSeconds = 1
	}
	if in.Retry.MaxAttempts < 1 || in.Retry.MaxAttempts > 10 || in.Retry.BackoffSeconds < 1 || in.Retry.BackoffSeconds > 3600 {
		return out, domain.Bad("invalid bounded retry policy")
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		c, e := scanContext(tx.QueryRow(ctx, "SELECT "+contextCols+" FROM linha_contexts WHERE id=$1 AND owner=$2 FOR NO KEY UPDATE", contextID, owner))
		if e != nil {
			return e
		}
		// Return an already accepted request even if its submitting version has since retired.
		var existing bool
		if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM linha_jobs WHERE owner=$1 AND context_name=$2 AND idempotency_key=$3)", owner, c.Name, in.IdempotencyKey).Scan(&existing); e != nil {
			return e
		}
		if !existing {
			var live bool
			if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM linha_client_leases WHERE id=$1 AND context_id=$2 AND expires_at>clock_timestamp())", in.LeaseID, contextID).Scan(&live); e != nil {
				return e
			}
			if !live {
				return domain.ClientLeaseExpired
			}
		}
		_, e = tx.Exec(ctx, "INSERT INTO linha_jobs(id,owner,context_id,idempotency_key,request,policy,state,deadline,context_name) VALUES($1,$2,$3,$4,$5,$6,'QUEUED',$7,$8) ON CONFLICT(owner,context_name,idempotency_key) DO NOTHING", domain.ID(), owner, contextID, in.IdempotencyKey, domain.JSON(in), domain.JSON(c.Spec.Results), in.Deadline, c.Name)
		if e != nil {
			return e
		}
		var equal bool
		e = tx.QueryRow(ctx, "SELECT request=$4::jsonb AND policy=$5::jsonb FROM linha_jobs WHERE owner=$1 AND context_name=$2 AND idempotency_key=$3", owner, c.Name, in.IdempotencyKey, domain.JSON(in), domain.JSON(c.Spec.Results)).Scan(&equal)
		if e != nil {
			return e
		}
		if !equal {
			return domain.Conflict("idempotency key was used for a different request")
		}
		out, e = scanJob(tx.QueryRow(ctx, "SELECT "+jobCols+" FROM linha_jobs WHERE owner=$1 AND context_name=$2 AND idempotency_key=$3", owner, c.Name, in.IdempotencyKey))
		return e
	})
	return
}
func (s *Store) Job(ctx context.Context, owner, id string) (domain.Job, error) {
	return scanJob(s.Pool.QueryRow(ctx, "SELECT "+jobCols+" FROM linha_jobs WHERE id=$1 AND owner=$2", id, owner))
}
func (s *Store) List(ctx context.Context, owner, contextID, state, cursor string, limit int) (domain.JobPage, error) {
	if limit < 1 || limit > 100 {
		return domain.JobPage{}, domain.Bad("limit must be 1-100")
	}
	rows, err := s.Pool.Query(ctx, "SELECT "+jobCols+" FROM linha_jobs WHERE owner=$1 AND ($2='' OR context_id=$2) AND ($3='' OR state=$3) AND id>$4 ORDER BY id LIMIT $5", owner, contextID, state, cursor, limit+1)
	if err != nil {
		return domain.JobPage{}, err
	}
	defer rows.Close()
	page := domain.JobPage{Items: []domain.Job{}}
	for rows.Next() {
		j, e := scanJob(rows)
		if e != nil {
			return page, e
		}
		page.Items = append(page.Items, j)
	}
	if err = rows.Err(); err != nil {
		return page, err
	}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		page.NextCursor = page.Items[limit-1].ID
	}
	return page, nil
}
func (s *Store) Cancel(ctx context.Context, owner, id string) (out domain.Job, err error) {
	err = s.tx(ctx, func(tx pgx.Tx) error {
		j, e := scanJob(tx.QueryRow(ctx, "SELECT "+jobCols+" FROM linha_jobs WHERE id=$1 AND owner=$2 FOR UPDATE", id, owner))
		if e != nil {
			return e
		}
		state := j.State
		switch state {
		case "QUEUED", "RETRYING":
			state = "CANCELLED"
		case "RUNNING":
			state = "CANCELLING"
		}
		if state != j.State {
			if _, e = tx.Exec(ctx, "UPDATE linha_jobs SET state=$2,updated_at=clock_timestamp() WHERE id=$1", id, state); e != nil {
				return e
			}
		}
		out, e = scanJob(tx.QueryRow(ctx, "SELECT "+jobCols+" FROM linha_jobs WHERE id=$1", id))
		return e
	})
	return
}
func (s *Store) Register(ctx context.Context, w domain.Worker) error {
	if w.ID == "" || w.Incarnation == "" || w.Capacity < 1 || w.Capacity > 128 || len(w.Capabilities) == 0 || len(w.Capabilities) > 256 {
		return domain.Bad("invalid worker registration")
	}
	seen := map[string]bool{}
	for _, c := range w.Capabilities {
		key := fmt.Sprintf("%s/%d", c.Handler, c.Version)
		if c.Handler == "" || c.Version < 1 || c.Result.Version < 1 || c.Result.Schema == "" || (c.Result.Kind != "json" && c.Result.Kind != "file" && c.Result.Kind != "dataset") || seen[key] {
			return domain.Bad("invalid or duplicate capability")
		}
		seen[key] = true
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		// Serialize late registration with retirement before the worker can claim work.
		var contextID string
		if err := tx.QueryRow(ctx, "SELECT id FROM linha_contexts WHERE id=$1 FOR NO KEY UPDATE", w.ContextID).Scan(&contextID); err != nil {
			return missing(err)
		}
		tag, err := tx.Exec(ctx, `INSERT INTO linha_workers(id,context_id,incarnation,capacity,capabilities) SELECT $1,id,$3,$4,$5 FROM linha_contexts WHERE id=$2 AND NOT EXISTS(SELECT 1 FROM linha_instances WHERE context_id=$2 AND pod_uid=$1 AND state IN ('DRAINING','DEAD')) ON CONFLICT(id) DO UPDATE SET heartbeat=clock_timestamp() WHERE linha_workers.context_id=excluded.context_id AND linha_workers.incarnation=excluded.incarnation AND linha_workers.capacity=excluded.capacity AND linha_workers.capabilities=excluded.capabilities`, w.ID, w.ContextID, w.Incarnation, w.Capacity, domain.JSON(w.Capabilities))
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return domain.Conflict("worker identity or capabilities conflict")
		}
		_, err = tx.Exec(ctx, "UPDATE linha_contexts SET state='READY',condition='' WHERE id=$1 AND state NOT IN ('DRAINING','STOPPED')", w.ContextID)
		return err
	})
}
func (s *Store) Heartbeat(ctx context.Context, id, inc string) (bool, error) {
	var draining bool
	err := s.Pool.QueryRow(ctx, "UPDATE linha_workers SET heartbeat=clock_timestamp() WHERE id=$1 AND incarnation=$2 RETURNING draining", id, inc).Scan(&draining)
	return draining, missing(err)
}
func (s *Store) Claim(ctx context.Context, id, inc string) (out *domain.Assignment, err error) {
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var contextID string
		var capacity, active int
		var caps []byte
		var draining, alive bool
		e := tx.QueryRow(ctx, "SELECT context_id,capacity,capabilities,draining,heartbeat>clock_timestamp()-$3*interval '1 second' FROM linha_workers WHERE id=$1 AND incarnation=$2 FOR UPDATE", id, inc, s.Lease.Seconds()*2).Scan(&contextID, &capacity, &caps, &draining, &alive)
		if e != nil {
			return missing(e)
		}
		if draining || !alive {
			return nil
		}
		e = tx.QueryRow(ctx, "SELECT count(*) FROM linha_attempts WHERE worker_id=$1 AND state='RUNNING'", id).Scan(&active)
		if e != nil {
			return e
		}
		if active >= capacity {
			return nil
		}
		j, e := scanJob(tx.QueryRow(ctx, "SELECT "+jobCols+` FROM linha_jobs WHERE context_id=$1 AND state IN ('QUEUED','RETRYING') AND available_at<=clock_timestamp() AND (deadline IS NULL OR deadline>clock_timestamp()) AND EXISTS(SELECT 1 FROM jsonb_array_elements($2::jsonb) c WHERE c->>'handler'=request->>'handler' AND c->>'version'=request->>'version') ORDER BY submitted_at,id FOR UPDATE SKIP LOCKED LIMIT 1`, contextID, caps))
		if errors.Is(e, domain.NotFound) {
			return nil
		}
		if e != nil {
			return e
		}
		var capabilities []domain.Capability
		if e = json.Unmarshal(caps, &capabilities); e != nil {
			return e
		}
		var descriptor domain.ResultDescriptor
		for _, c := range capabilities {
			if c.Handler == j.Request.Handler && c.Version == j.Request.Version {
				descriptor = c.Result
			}
		}
		a := domain.Assignment{LeaseDurationMillis: s.Lease.Milliseconds(), Job: j, AttemptID: domain.ID(), Fence: int64(j.AttemptCount + 1), ResultDescriptor: descriptor}
		var policy []byte
		e = tx.QueryRow(ctx, "SELECT policy FROM linha_jobs WHERE id=$1", j.ID).Scan(&policy)
		if e != nil {
			return e
		}
		if e = json.Unmarshal(policy, &a.Policy); e != nil {
			return e
		}
		e = tx.QueryRow(ctx, "INSERT INTO linha_attempts(id,job_id,number,worker_id,incarnation,fence,descriptor,state,lease_expires_at) VALUES($1,$2,$3::integer,$4,$5,$3::integer,$6,'RUNNING',clock_timestamp()+$7*interval '1 second') RETURNING lease_expires_at", a.AttemptID, j.ID, a.Fence, id, inc, domain.JSON(descriptor), s.Lease.Seconds()).Scan(&a.LeaseExpiresAt)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, "UPDATE linha_jobs SET state='RUNNING',current_attempt=$2,attempt_count=attempt_count+1,progress=NULL,failure=NULL,updated_at=clock_timestamp() WHERE id=$1", j.ID, a.AttemptID)
		if e != nil {
			return e
		}
		a.Job.State = "RUNNING"
		a.Job.AttemptCount++
		out = &a
		return nil
	})
	return
}

// Ownership is checked under the job lock, serializing completion with cancellation and recovery.
func owned(ctx context.Context, tx pgx.Tx, job string, u domain.AttemptUpdate, allowCancelling bool) error {
	var state string
	var valid bool
	err := tx.QueryRow(ctx, `SELECT j.state,a.id=$2 AND a.worker_id=$3 AND a.incarnation=$4 AND a.fence=$5 AND a.state='RUNNING' AND a.lease_expires_at>clock_timestamp() AND (j.deadline IS NULL OR j.deadline>clock_timestamp()) FROM linha_jobs j JOIN linha_attempts a ON a.id=j.current_attempt WHERE j.id=$1 FOR UPDATE OF j`, job, u.AttemptID, u.WorkerID, u.Incarnation, u.Fence).Scan(&state, &valid)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Stale
	}
	if err != nil {
		return err
	}
	if !valid || (state != "RUNNING" && !(allowCancelling && state == "CANCELLING")) {
		return domain.Stale
	}
	return nil
}
func (s *Store) Renew(ctx context.Context, job string, u domain.AttemptUpdate, p *domain.Progress) (expiry time.Time, err error) {
	if p != nil && (p.Fraction < 0 || p.Fraction > 1 || math.IsNaN(p.Fraction) || len(p.Message) > 4096) {
		return expiry, domain.Bad("invalid progress")
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if e := owned(ctx, tx, job, u, false); e != nil {
			return e
		}
		if p != nil {
			if _, e := tx.Exec(ctx, "UPDATE linha_jobs SET progress=$2,updated_at=clock_timestamp() WHERE id=$1", job, domain.JSON(p)); e != nil {
				return e
			}
		}
		return tx.QueryRow(ctx, "UPDATE linha_attempts SET lease_expires_at=clock_timestamp()+$2*interval '1 second' WHERE id=$1 RETURNING lease_expires_at", u.AttemptID, s.Lease.Seconds()).Scan(&expiry)
	})
	return
}
func (s *Store) Fail(ctx context.Context, job string, u domain.AttemptUpdate, f domain.Failure) error {
	if err := f.Validate(); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		if e := owned(ctx, tx, job, u, true); e != nil {
			return e
		}
		return finishFailure(ctx, tx, job, u.AttemptID, "FAILED", f)
	})
}
func finishFailure(ctx context.Context, tx pgx.Tx, job, attempt, attemptState string, f domain.Failure) error {
	var state string
	var n int
	var raw []byte
	var expired bool
	if err := tx.QueryRow(ctx, "SELECT state,attempt_count,request,deadline IS NOT NULL AND deadline<=clock_timestamp() FROM linha_jobs WHERE id=$1", job).Scan(&state, &n, &raw, &expired); err != nil {
		return err
	}
	request, err := domain.Decode[domain.SubmitRequest](raw)
	if err != nil {
		return err
	}
	attemptFailure := f
	target := "FAILED"
	if state == "CANCELLING" {
		target = "CANCELLED"
		attemptState = "CANCELLED"
	} else if expired {
		f = domain.Failure{Code: "DEADLINE_EXCEEDED", Message: "job deadline expired", Retryable: false}
		attemptState = "FAILED"
	} else if f.Retryable && n < request.Retry.MaxAttempts {
		target = "RETRYING"
	}
	if _, err = tx.Exec(ctx, "UPDATE linha_attempts SET state=$2,failure=$3,finished_at=clock_timestamp() WHERE id=$1", attempt, attemptState, domain.JSON(attemptFailure)); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "UPDATE linha_jobs SET state=$2,failure=$3,available_at=clock_timestamp()+$4*interval '1 second',updated_at=clock_timestamp() WHERE id=$1", job, target, domain.JSON(f), request.Retry.BackoffSeconds)
	return err
}
func (s *Store) Allocate(ctx context.Context, job string, in domain.OutputRequest) (a domain.Allocation, err error) {
	if e := pathspec.Name(in.Name); e != nil {
		return a, domain.Bad(e.Error())
	}
	if in.Kind != "json" && in.Kind != "file" && in.Kind != "dataset" {
		return a, domain.Bad("invalid output kind")
	}
	if in.ContentType == "" || len(in.ContentType) > 256 || strings.ContainsAny(in.ContentType, "\r\n") {
		return a, domain.Bad("invalid content type")
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if e := owned(ctx, tx, job, in.AttemptUpdate, false); e != nil {
			return e
		}
		existing, e := scanAllocation(tx.QueryRow(ctx, "SELECT "+outputCols+" FROM linha_outputs WHERE attempt_id=$1 AND name=$2", in.AttemptID, in.Name))
		if e == nil {
			if existing.Kind != in.Kind || existing.ContentType != in.ContentType {
				return domain.Conflict("artifact name already allocated differently")
			}
			a = existing
			return nil
		}
		if !errors.Is(e, domain.NotFound) {
			return e
		}
		var overlapping bool
		e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM linha_outputs WHERE attempt_id=$1 AND (starts_with(name,$2||'/') OR starts_with($2,name||'/')))", in.AttemptID, in.Name).Scan(&overlapping)
		if e != nil {
			return e
		}
		if overlapping {
			return domain.Conflict("overlapping artifact path")
		}
		var contextID string
		var policy []byte
		var stamp time.Time
		e = tx.QueryRow(ctx, "SELECT context_id,policy,submitted_at FROM linha_jobs WHERE id=$1", job).Scan(&contextID, &policy, &stamp)
		if e != nil {
			return e
		}
		if e = json.Unmarshal(policy, &a.Policy); e != nil {
			return e
		}
		template := a.Policy.Path.Template
		if a.Policy.Type == "local" {
			template = pathspec.Default
		}
		key, e := pathspec.Expand(template, pathspec.Values{ContextID: contextID, RequestID: job, AttemptID: in.AttemptID, File: in.Name, SubmittedAt: stamp})
		if e != nil {
			return domain.Bad(e.Error())
		}
		a.ID = domain.ID()
		a.JobID = job
		a.AttemptID = in.AttemptID
		a.Name = in.Name
		a.Kind = in.Kind
		a.ContentType = in.ContentType
		a.Key = key
		_, e = tx.Exec(ctx, "INSERT INTO linha_outputs(id,job_id,attempt_id,name,kind,content_type,key,policy) VALUES($1,$2,$3,$4,$5,$6,$7,$8)", a.ID, job, in.AttemptID, a.Name, a.Kind, a.ContentType, a.Key, policy)
		return e
	})
	return
}

const outputCols = "id,job_id,attempt_id,name,kind,content_type,key,policy"

func scanAllocation(row pgx.Row) (a domain.Allocation, err error) {
	var policy []byte
	err = row.Scan(&a.ID, &a.JobID, &a.AttemptID, &a.Name, &a.Kind, &a.ContentType, &a.Key, &policy)
	if err != nil {
		return a, missing(err)
	}
	err = json.Unmarshal(policy, &a.Policy)
	return
}
func (s *Store) Output(ctx context.Context, job string, u domain.AttemptUpdate, id string) (a domain.Allocation, err error) {
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if e := owned(ctx, tx, job, u, false); e != nil {
			return e
		}
		var e error
		a, e = scanAllocation(tx.QueryRow(ctx, "SELECT "+outputCols+" FROM linha_outputs WHERE id=$1 AND job_id=$2 AND attempt_id=$3", id, job, u.AttemptID))
		return e
	})
	return
}
func (s *Store) Complete(ctx context.Context, job string, c domain.Completion) (result domain.Result, err error) {
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var state string
		var same bool
		var raw []byte
		e := tx.QueryRow(ctx, "SELECT state,COALESCE(completion=$2::jsonb,false),result FROM linha_jobs WHERE id=$1 FOR UPDATE", job, domain.JSON(c)).Scan(&state, &same, &raw)
		if e != nil {
			return missing(e)
		}
		if state == "SUCCEEDED" {
			if !same {
				return domain.Conflict("completion differs from committed receipt")
			}
			return json.Unmarshal(raw, &result)
		}
		if e = owned(ctx, tx, job, c.AttemptUpdate, false); e != nil {
			return e
		}
		var expected []byte
		var policy []byte
		e = tx.QueryRow(ctx, "SELECT a.descriptor,j.policy FROM linha_attempts a JOIN linha_jobs j ON j.id=a.job_id WHERE a.id=$1", c.AttemptID).Scan(&expected, &policy)
		if e != nil {
			return e
		}
		descriptor, e := domain.Decode[domain.ResultDescriptor](expected)
		if e != nil {
			return e
		}
		if descriptor != c.Descriptor {
			return domain.Bad("result descriptor does not match handler")
		}
		if c.Descriptor.Kind == "json" && len(c.Files) != 1 {
			return domain.Bad("JSON result requires exactly one file")
		}
		if len(c.Files) == 0 {
			return domain.Bad("result requires at least one file")
		}
		var sum int64
		seen := map[string]bool{}
		for _, f := range c.Files {
			if seen[f.AllocationID] || f.Size < 0 || len(f.SHA256) != 64 {
				return domain.Bad("invalid result files")
			}
			seen[f.AllocationID] = true
			var valid bool
			e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM linha_outputs WHERE id=$1 AND job_id=$2 AND attempt_id=$3 AND name=$4 AND content_type=$5)", f.AllocationID, job, c.AttemptID, f.Name, f.ContentType).Scan(&valid)
			if e != nil {
				return e
			}
			if !valid {
				return domain.Bad("result file is not allocated to this attempt")
			}
			if sum > math.MaxInt64-f.Size {
				return domain.Bad("result size overflow")
			}
			sum += f.Size
		}
		p, e := domain.Decode[domain.ResultPolicy](policy)
		if e != nil {
			return e
		}
		if sum > p.MaxBytes {
			return &domain.Error{Code: "RESULT_TOO_LARGE", Message: "result exceeds configured limit", Status: 413}
		}
		if c.Descriptor.Kind == "dataset" {
			if c.Dataset == nil || len(c.Files) != 1 || c.Files[0] != c.Dataset.Manifest {
				return domain.Bad("dataset requires its sealed manifest")
			}
			var valid bool
			if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM linha_outputs WHERE id=$1 AND attempt_id=$2 AND dataset=$3::jsonb)", c.Dataset.AllocationID, c.AttemptID, domain.JSON(c.Dataset)).Scan(&valid); e != nil {
				return e
			}
			if !valid {
				return domain.Bad("dataset is not sealed by this attempt")
			}
			if c.Dataset.TotalBytes > p.MaxBytes {
				return domain.Bad("dataset exceeds configured byte limit")
			}
		} else if c.Dataset != nil {
			return domain.Bad("unexpected dataset descriptor")
		}
		result.Dataset = c.Dataset
		result.Descriptor = c.Descriptor
		result.Files = c.Files
		e = tx.QueryRow(ctx, "SELECT clock_timestamp()+$1*interval '1 second'", p.RetentionSeconds).Scan(&result.ExpiresAt)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, "UPDATE linha_attempts SET state='SUCCEEDED',finished_at=clock_timestamp() WHERE id=$1", c.AttemptID)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, "UPDATE linha_jobs SET state='SUCCEEDED',result=$2,completion=$3,updated_at=clock_timestamp() WHERE id=$1", job, domain.JSON(result), domain.JSON(c))
		return e
	})
	return
}
func (s *Store) ResultOutput(ctx context.Context, owner, job, id string) (a domain.Allocation, f domain.File, err error) {
	j, err := s.Job(ctx, owner, job)
	if err != nil {
		return a, f, err
	}
	if j.State != "SUCCEEDED" || j.Result == nil {
		return a, f, &domain.Error{Code: "RESULT_NOT_READY", Message: "result is not ready", Status: 409}
	}
	var expired bool
	err = s.Pool.QueryRow(ctx, "SELECT clock_timestamp()>$1", j.Result.ExpiresAt).Scan(&expired)
	if err != nil {
		return a, f, err
	}
	if expired || j.Result.Expired {
		return a, f, &domain.Error{Code: "RESULT_EXPIRED", Message: "result retention expired", Status: 410}
	}
	for _, file := range j.Result.Files {
		if file.AllocationID == id {
			a, err = scanAllocation(s.Pool.QueryRow(ctx, "SELECT "+outputCols+" FROM linha_outputs WHERE id=$1 AND job_id=$2", id, job))
			return a, file, err
		}
	}
	return a, f, domain.NotFound
}
func (s *Store) Recover(ctx context.Context) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT j.id,j.current_attempt FROM linha_jobs j JOIN linha_attempts a ON a.id=j.current_attempt WHERE j.state IN ('RUNNING','CANCELLING') AND (a.lease_expires_at<=clock_timestamp() OR j.deadline<=clock_timestamp()) FOR UPDATE OF j SKIP LOCKED LIMIT 100`)
		if err != nil {
			return err
		}
		var jobs [][2]string
		for rows.Next() {
			var x [2]string
			if err = rows.Scan(&x[0], &x[1]); err != nil {
				rows.Close()
				return err
			}
			jobs = append(jobs, x)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, j := range jobs {
			if err = finishFailure(ctx, tx, j[0], j[1], "LOST", domain.Failure{Code: "ATTEMPT_LOST", Message: "attempt lease or deadline expired", Retryable: true}); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(ctx, `UPDATE linha_jobs SET state='FAILED',failure='{"code":"DEADLINE_EXCEEDED","message":"job deadline expired","retryable":false}',updated_at=clock_timestamp() WHERE state IN ('QUEUED','RETRYING') AND deadline<=clock_timestamp()`); err != nil {
			return err
		}

		if _, err = tx.Exec(ctx, `UPDATE linha_jobs j SET state='FAILED', failure='{"code":"UNSUPPORTED_HANDLER","message":"ready backend does not support the requested handler/version","retryable":false}',updated_at=clock_timestamp()
          WHERE j.state IN ('QUEUED','RETRYING')
          AND EXISTS(SELECT 1 FROM linha_workers w WHERE w.context_id=j.context_id AND NOT w.draining AND w.heartbeat>clock_timestamp()-$1*interval '1 second')
          AND NOT EXISTS(SELECT 1 FROM linha_workers w, jsonb_array_elements(w.capabilities) cap WHERE w.context_id=j.context_id AND NOT w.draining AND w.heartbeat>clock_timestamp()-$1*interval '1 second' AND cap->>'handler'=j.request->>'handler' AND cap->>'version'=j.request->>'version')`, s.Lease.Seconds()*2); err != nil {
			return err
		}

		_, err = tx.Exec(ctx, `UPDATE linha_contexts c SET state=CASE WHEN EXISTS(SELECT 1 FROM linha_workers w WHERE w.context_id=c.id AND w.heartbeat>clock_timestamp()-$1*interval '1 second' AND NOT w.draining) THEN 'READY' ELSE 'STARTING' END WHERE c.state NOT IN ('DRAINING','STOPPED')`, s.Lease.Seconds()*2)
		return err
	})
}

var _ domain.Repository = (*Store)(nil)
