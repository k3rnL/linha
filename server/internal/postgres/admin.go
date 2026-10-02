package postgres

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"linha/server/internal/domain"
	"time"
)

func validateSubmission(in *domain.SubmitRequest) error {
	if in.Handler == "" || len(in.Handler) > 256 || in.Version < 1 || in.IdempotencyKey == "" || len(in.IdempotencyKey) > 256 || !json.Valid(in.Payload) {
		return domain.Bad("handler, version, JSON payload and idempotency key are required")
	}
	if len(in.Payload) > 1<<20 {
		return domain.Bad("payload exceeds 1 MiB")
	}
	if in.Retry.MaxAttempts == 0 {
		in.Retry.MaxAttempts = 1
	}
	if in.Retry.BackoffSeconds == 0 {
		in.Retry.BackoffSeconds = 1
	}
	if in.Retry.MaxAttempts < 1 || in.Retry.MaxAttempts > 10 || in.Retry.BackoffSeconds < 1 || in.Retry.BackoffSeconds > 3600 {
		return domain.Bad("invalid bounded retry policy")
	}
	return nil
}

// AdminSubmit shares context locking, owner/idempotency scope and accepted-work lifetime with SDK submission.
func (s *Store) AdminSubmit(ctx context.Context, actor, contextID string, in domain.AdminSubmitRequest) (out domain.Job, err error) {
	if actor == "" {
		return out, domain.Bad("actor required")
	}
	if err = validateSubmission(&in.SubmitRequest); err != nil {
		return
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		c, e := scanContext(tx.QueryRow(ctx, "SELECT "+contextCols+" FROM linha_contexts WHERE id=$1 FOR NO KEY UPDATE", contextID))
		if e != nil {
			return e
		}
		var existing string
		e = tx.QueryRow(ctx, "SELECT id FROM linha_jobs WHERE owner=$1 AND context_name=$2 AND idempotency_key=$3", c.Owner, c.Name, in.IdempotencyKey).Scan(&existing)
		if e != nil && e != pgx.ErrNoRows {
			return e
		}
		if existing != "" {
			var equal bool
			e = tx.QueryRow(ctx, `SELECT submission_source='admin' AND context_id=$2 AND submitted_by=$3 AND replayed_from=$4 AND admin_activation=$5 AND request=$6::jsonb AND policy=$7::jsonb FROM linha_jobs WHERE id=$1`, existing, contextID, actor, in.ReplayedFrom, in.ActivateIfStopped, domain.JSON(in.SubmitRequest), domain.JSON(c.Spec.Results)).Scan(&equal)
			if e != nil {
				return e
			}
			if !equal {
				return domain.Conflict("idempotency key was used for a different request or provenance")
			}
			out, e = scanJob(tx.QueryRow(ctx, "SELECT "+jobCols+" FROM linha_jobs WHERE id=$1", existing))
			return e
		}
		if in.Deadline != nil && !in.Deadline.After(time.Now()) {
			return domain.Bad("a new deadline must be in the future")
		}
		if in.ReplayedFrom != "" {
			var owner, name string
			e = tx.QueryRow(ctx, "SELECT owner,context_name FROM linha_jobs WHERE id=$1", in.ReplayedFrom).Scan(&owner, &name)
			if e != nil {
				return missing(e)
			}
			if owner != c.Owner || name != c.Name {
				return domain.Conflict("replay target must be a version of the source owner's logical context")
			}
		}
		if (c.State == "STOPPED" || c.State == "DRAINING") && !in.ActivateIfStopped {
			return domain.Conflict("context is stopped or draining; set activateIfStopped to accept work explicitly")
		}
		id := domain.ID()
		_, e = tx.Exec(ctx, `INSERT INTO linha_jobs(id,owner,context_id,idempotency_key,request,policy,state,deadline,context_name,submission_source,submitted_by,replayed_from,admin_activation) VALUES($1,$2,$3,$4,$5,$6,'QUEUED',$7,$8,'admin',$9,$10,$11) ON CONFLICT(owner,context_name,idempotency_key) DO NOTHING`, id, c.Owner, c.ID, in.IdempotencyKey, domain.JSON(in.SubmitRequest), domain.JSON(c.Spec.Results), in.Deadline, c.Name, actor, in.ReplayedFrom, in.ActivateIfStopped)
		if e != nil {
			return e
		}
		var committed string
		if e = tx.QueryRow(ctx, "SELECT id FROM linha_jobs WHERE owner=$1 AND context_name=$2 AND idempotency_key=$3", c.Owner, c.Name, in.IdempotencyKey).Scan(&committed); e != nil {
			return e
		}
		if committed != id {
			return domain.Conflict("idempotency key concurrently accepted on a different context version")
		}
		_, e = tx.Exec(ctx, "UPDATE linha_contexts SET state=CASE WHEN state='STOPPED' THEN 'STARTING' ELSE state END,reconcile_after=clock_timestamp() WHERE id=$1", c.ID)
		if e != nil {
			return e
		}
		action := "create"
		if in.ReplayedFrom != "" {
			action = "replay"
		}
		if e = acceptedAudit(ctx, tx, actor, action, c.Owner, c.ID, id, in.ReplayedFrom, "QUEUED"); e != nil {
			return e
		}
		out, e = scanJob(tx.QueryRow(ctx, "SELECT "+jobCols+" FROM linha_jobs WHERE id=$1", id))
		return e
	})
	return
}
func acceptedAudit(ctx context.Context, tx pgx.Tx, actor, action, owner, contextID, job, source, outcome string) error {
	_, e := tx.Exec(ctx, `INSERT INTO linha_admin_audit(id,actor,action,owner,context_id,job_id,replayed_from,outcome) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(job_id,action) DO NOTHING`, domain.ID(), actor, action, owner, contextID, job, source, outcome)
	return e
}
func (s *Store) AdminCancel(ctx context.Context, actor, id string) (out domain.Job, err error) {
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var owner string
		if e := tx.QueryRow(ctx, "SELECT owner FROM linha_jobs WHERE id=$1", id).Scan(&owner); e != nil {
			return missing(e)
		}
		j, e := scanJob(tx.QueryRow(ctx, "SELECT "+jobCols+" FROM linha_jobs WHERE id=$1 FOR UPDATE", id))
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
			_, e = tx.Exec(ctx, "UPDATE linha_jobs SET state=$2,updated_at=clock_timestamp() WHERE id=$1", id, state)
			if e != nil {
				return e
			}
		}
		if e = acceptedAudit(ctx, tx, actor, "cancel", owner, j.ContextID, id, "", state); e != nil {
			return e
		}
		out, e = scanJob(tx.QueryRow(ctx, "SELECT "+jobCols+" FROM linha_jobs WHERE id=$1", id))
		return e
	})
	return
}

var adminJobProjection = `jsonb_build_object('id',j.id,'contextId',j.context_id,'request',j.request,'state',j.state,'submittedAt',j.submitted_at,'updatedAt',j.updated_at,'attemptCount',j.attempt_count,'progress',j.progress,'failure',j.failure,'result',j.result,'owner',j.owner,'contextName',c.name,'contextVersion',c.version,'metricContext',c.metric_context,'submittedBy',j.submitted_by,'replayedFrom',j.replayed_from)`

func (s *Store) AdminJob(ctx context.Context, id string) (out domain.AdministrativeJob, e error) {
	var b []byte
	e = s.Pool.QueryRow(ctx, "SELECT "+adminJobProjection+" FROM linha_jobs j JOIN linha_contexts c ON c.id=j.context_id WHERE j.id=$1", id).Scan(&b)
	if e != nil {
		return out, missing(e)
	}
	e = json.Unmarshal(b, &out)
	return
}
func (s *Store) AdminJobs(ctx context.Context, f domain.OperationalFilter) (page domain.Page[domain.AdministrativeJob], e error) {
	limit, e := pageLimit(f)
	if e != nil {
		return
	}
	cursor, e := decodeOperationalCursor(f)
	if e != nil {
		return
	}
	rows, e := s.Pool.Query(ctx, "SELECT "+adminJobProjection+` FROM linha_jobs j JOIN linha_contexts c ON c.id=j.context_id WHERE ($1='' OR j.owner=$1) AND ($2='' OR c.name=$2) AND ($3='' OR c.spec->'engine'->>'type'=$3) AND ($4='' OR j.state=$4) AND ($5='' OR j.context_id=$5) AND ($6='' OR c.metric_context=$6) AND ($7::timestamptz IS NULL OR j.submitted_at>=$7) AND ($8::timestamptz IS NULL OR j.submitted_at<=$8) AND (j.submitted_at,j.id)<($9,$10) ORDER BY j.submitted_at DESC,j.id DESC LIMIT $11`, f.Owner, f.Name, f.Engine, f.State, f.ContextID, f.MetricContext, f.From, f.Until, cursor.Time, cursor.Key, limit+1)
	if e != nil {
		return
	}
	defer rows.Close()
	page.Items = []domain.AdministrativeJob{}
	for rows.Next() {
		var b []byte
		var j domain.AdministrativeJob
		if e = rows.Scan(&b); e != nil {
			return
		}
		if e = json.Unmarshal(b, &j); e != nil {
			return
		}
		page.Items = append(page.Items, j)
	}
	e = rows.Err()
	if e != nil {
		return
	}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		last := page.Items[limit-1]
		page.NextCursor = encodeOperationalCursor(f, last.ID, last.SubmittedAt)
	}
	return
}
func (s *Store) AdminAudit(ctx context.Context, job string, f domain.OperationalFilter) (page domain.Page[domain.AuditRecord], e error) {
	if _, e = s.AdminJob(ctx, job); e != nil {
		return
	}
	f.ContextID = job
	limit, e := pageLimit(f)
	if e != nil {
		return
	}
	cursor, e := decodeOperationalCursor(f)
	if e != nil {
		return
	}
	rows, e := s.Pool.Query(ctx, `SELECT id,actor,action,owner,context_id,job_id,replayed_from,accepted_at,outcome FROM linha_admin_audit WHERE job_id=$1 AND (accepted_at,id)<($2,$3) ORDER BY accepted_at DESC,id DESC LIMIT $4`, job, cursor.Time, cursor.Key, limit+1)
	if e != nil {
		return
	}
	defer rows.Close()
	page.Items = []domain.AuditRecord{}
	for rows.Next() {
		var a domain.AuditRecord
		if e = rows.Scan(&a.ID, &a.Actor, &a.Action, &a.Owner, &a.ContextID, &a.JobID, &a.ReplayedFrom, &a.AcceptedAt, &a.Outcome); e != nil {
			return
		}
		page.Items = append(page.Items, a)
	}
	e = rows.Err()
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		last := page.Items[limit-1]
		page.NextCursor = encodeOperationalCursor(f, last.ID, last.AcceptedAt)
	}
	return
}
func (s *Store) OperationalWorkers(ctx context.Context, owner, id string, f domain.OperationalFilter) (page domain.Page[domain.WorkerObservation], e error) {
	if _, e = s.OperationalContext(ctx, owner, id); e != nil {
		return
	}
	f.Owner = owner
	f.ContextID = id
	limit, e := pageLimit(f)
	if e != nil {
		return
	}
	cur, e := decodeOperationalCursor(f)
	if e != nil {
		return
	}
	rows, e := s.Pool.Query(ctx, `SELECT id,context_id,incarnation,capacity,capabilities,draining,heartbeat,CASE WHEN heartbeat<=clock_timestamp()-interval '60 seconds' THEN 'stale' WHEN draining THEN 'draining' ELSE 'ready' END,(SELECT count(*) FROM linha_attempts WHERE worker_id=w.id AND state='RUNNING' AND lease_expires_at>clock_timestamp()) FROM linha_workers w WHERE context_id=$1 AND ($2='' OR id<$2) ORDER BY id DESC LIMIT $3`, id, cur.Key, limit+1)
	if e != nil {
		return
	}
	defer rows.Close()
	page.Items = []domain.WorkerObservation{}
	for rows.Next() {
		var w domain.WorkerObservation
		var caps []byte
		if e = rows.Scan(&w.ID, &w.ContextID, &w.Incarnation, &w.Capacity, &caps, &w.Draining, &w.HeartbeatAt, &w.State, &w.OccupiedSlots); e != nil {
			return
		}
		if e = json.Unmarshal(caps, &w.Capabilities); e != nil {
			return
		}
		page.Items = append(page.Items, w)
	}
	e = rows.Err()
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		page.NextCursor = encodeOperationalCursor(f, page.Items[limit-1].ID, time.Time{})
	}
	return
}
func (s *Store) SaveLogin(ctx context.Context, l domain.LoginTransaction) error {
	_, e := s.Pool.Exec(ctx, "INSERT INTO linha_browser_logins(hash,nonce,verifier,expires_at) VALUES($1,$2,$3,$4)", l.Hash, l.Nonce, l.Verifier, l.ExpiresAt)
	return e
}
func (s *Store) TakeLogin(ctx context.Context, hash string) (out domain.LoginTransaction, e error) {
	e = s.Pool.QueryRow(ctx, "DELETE FROM linha_browser_logins WHERE hash=$1 AND expires_at>clock_timestamp() RETURNING hash,nonce,verifier,expires_at", hash).Scan(&out.Hash, &out.Nonce, &out.Verifier, &out.ExpiresAt)
	return out, missing(e)
}
func (s *Store) SaveSession(ctx context.Context, v domain.BrowserSession) error {
	_, e := s.Pool.Exec(ctx, "INSERT INTO linha_browser_sessions(hash,claims,csrf,expires_at) VALUES($1,$2,$3,$4)", v.Hash, v.Claims, v.CSRF, v.ExpiresAt)
	return e
}
func (s *Store) Session(ctx context.Context, hash string) (v domain.BrowserSession, e error) {
	e = s.Pool.QueryRow(ctx, "SELECT hash,claims,csrf,expires_at FROM linha_browser_sessions WHERE hash=$1 AND expires_at>clock_timestamp()", hash).Scan(&v.Hash, &v.Claims, &v.CSRF, &v.ExpiresAt)
	return v, missing(e)
}
func (s *Store) DeleteSession(ctx context.Context, hash string) error {
	_, e := s.Pool.Exec(ctx, "DELETE FROM linha_browser_sessions WHERE hash=$1", hash)
	return e
}
func (s *Store) ExpireBrowserRecords(ctx context.Context) error {
	for _, table := range []string{"linha_browser_logins", "linha_browser_sessions"} {
		_, e := s.Pool.Exec(ctx, "DELETE FROM "+table+" WHERE hash IN (SELECT hash FROM "+table+" WHERE expires_at<=clock_timestamp() LIMIT 200)")
		if e != nil {
			return e
		}
	}
	return nil
}

var _ domain.AdminRepository = (*Store)(nil)
var _ domain.BrowserRepository = (*Store)(nil)

// AdminAllocation authorizes only references in the committed result, including expired metadata.
func (s *Store) AdminAllocation(ctx context.Context, job, id string) (domain.Allocation, error) {
	return scanAllocation(s.Pool.QueryRow(ctx, `SELECT o.id,o.job_id,o.attempt_id,o.name,o.kind,o.content_type,o.key,o.policy FROM linha_outputs o JOIN linha_jobs j ON j.id=o.job_id WHERE j.id=$1 AND o.id=$2 AND j.state='SUCCEEDED' AND (EXISTS(SELECT 1 FROM jsonb_array_elements(j.result->'files') f WHERE f->>'allocationId'=o.id) OR j.result->'dataset'->>'allocationId'=o.id OR j.result->'dataset'->'manifest'->>'allocationId'=o.id)`, job, id))
}
