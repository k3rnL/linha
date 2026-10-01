// Package controller reconciles durable capacity for live client leases and accepted work.
package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"linha/server/internal/domain"
	"linha/server/internal/engine"
	"log/slog"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Controller struct {
	Pool    *pgxpool.Pool
	Adapter engine.Adapter
	Resolve func(context.Context, domain.BackendSpec) (string, error)
	ID      string
}
type record struct {
	instance  engine.Instance
	state     string
	lastBusy  time.Time
	createdAt time.Time
}

func (c *Controller) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := c.Tick(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("backend reconciliation unavailable", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (c *Controller) Tick(ctx context.Context) error {
	rows, err := c.Pool.Query(ctx, "SELECT id FROM linha_contexts WHERE spec->'engine'->>'type'='spark' AND reconcile_after<=clock_timestamp()")
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err = c.reconcile(ctx, id); err != nil {
			_, _ = c.Pool.Exec(ctx, "UPDATE linha_contexts SET condition=$2,startup_failures=LEAST(startup_failures+1,6),reconcile_after=clock_timestamp()+LEAST(300,5*power(2,startup_failures))*interval '1 second' WHERE id=$1 AND reconcile_owner=$3 AND reconcile_until>clock_timestamp()", id, "PROVISIONING_FAILED: "+err.Error(), c.ID)
			slog.Warn("backend reconcile failed", "contextId", id, "error", err)
		}
	}
	return nil
}
func (c *Controller) valid(ctx context.Context, id string, epoch int64) error {
	tag, err := c.Pool.Exec(ctx, "UPDATE linha_contexts SET reconcile_until=clock_timestamp()+interval '30 seconds' WHERE id=$1 AND reconcile_owner=$2 AND reconcile_epoch=$3 AND reconcile_until>clock_timestamp()", id, c.ID, epoch)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("reconciliation ownership expired")
	}
	return nil
}
func (c *Controller) reconcile(ctx context.Context, id string) error {
	var epoch int64
	var spec []byte
	var image string
	var now, lastScaled time.Time
	err := c.Pool.QueryRow(ctx, `UPDATE linha_contexts SET reconcile_epoch=CASE WHEN reconcile_owner=$2 AND reconcile_until>clock_timestamp() THEN reconcile_epoch ELSE reconcile_epoch+1 END,reconcile_owner=$2,reconcile_until=clock_timestamp()+interval '30 seconds' WHERE id=$1 AND (reconcile_until<=clock_timestamp() OR reconcile_owner=$2) RETURNING reconcile_epoch,spec,resolved_image,clock_timestamp(),COALESCE(NULLIF(last_scaled,'-infinity'),'1970-01-01'::timestamptz)`, id, c.ID).Scan(&epoch, &spec, &image, &now, &lastScaled)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var backend domain.BackendSpec
	if err = json.Unmarshal(spec, &backend); err != nil {
		return err
	}
	settings, err := engine.ParseSpark(backend.Engine)
	if err != nil {
		return err
	}
	contextRecord := domain.BackendContext{ID: id, Spec: backend}
	live, work, err := c.lifetime(ctx, id, epoch)
	if err != nil {
		return err
	}
	if image == "" && (live || work) {
		resolveContext, resolveCancel := context.WithTimeout(ctx, 15*time.Second)
		image, err = c.Resolve(resolveContext, backend)
		resolveCancel()
		if err != nil {
			return err
		}
		if err = c.valid(ctx, id, epoch); err != nil {
			return err
		}
		if _, err = c.Pool.Exec(ctx, "UPDATE linha_contexts SET resolved_image=$2 WHERE id=$1 AND resolved_image='' AND reconcile_owner=$3 AND reconcile_epoch=$4 AND reconcile_until>clock_timestamp()", id, image, c.ID, epoch); err != nil {
			return err
		}
		if err = c.Pool.QueryRow(ctx, "SELECT resolved_image FROM linha_contexts WHERE id=$1", id).Scan(&image); err != nil {
			return err
		}
	}
	if err = c.cleanup(ctx, id, epoch, image); err != nil {
		return err
	}
	_, err = c.fenced(ctx, id, epoch, "UPDATE linha_contexts SET condition='' WHERE id=$1", id)
	if err != nil {
		return err
	}
	records, err := c.instances(ctx, id, image)
	if err != nil {
		return err
	}
	for _, r := range records {
		if err = c.valid(ctx, id, epoch); err != nil {
			return err
		}
		observed, e := c.Adapter.Observe(ctx, r.instance)
		if errors.Is(e, domain.NotFound) {
			if r.instance.Incarnation == "" && r.instance.ResourceUID == "" && r.state != "DRAINING" {
				ensureContext, ensureCancel := context.WithTimeout(ctx, 20*time.Second)
				e = c.Adapter.Ensure(ensureContext, contextRecord, r.instance)
				ensureCancel()
				if e != nil {
					return e
				}
				continue
			}
			if _, e = c.fenced(ctx, id, epoch, "UPDATE linha_workers SET draining=true WHERE id=$1", r.instance.Incarnation); e != nil {
				return e
			}
			if _, e = c.fenced(ctx, id, epoch, "UPDATE linha_instances SET state='DEAD' WHERE id=$1", r.instance.ID); e != nil {
				return e
			}
			continue
		}
		if e != nil {
			return e
		}
		if observed.Condition != "" {
			if _, e = c.fenced(ctx, id, epoch, "UPDATE linha_contexts SET condition=$2 WHERE id=$1", id, observed.Condition); e != nil {
				return e
			}
		}
		if _, e = c.fenced(ctx, id, epoch, "UPDATE linha_instances SET pod_uid=$2,resource_uid=$3 WHERE id=$1 AND (pod_uid='' OR pod_uid=$2) AND (resource_uid='' OR resource_uid=$3)", r.instance.ID, observed.Incarnation, observed.ResourceUID); e != nil {
			return e
		}
		if observed.Draining || r.state == "DRAINING" || r.state == "STARTING" && now.Sub(r.createdAt) > 5*time.Minute {
			if _, e = c.fenced(ctx, id, epoch, "UPDATE linha_workers SET draining=true WHERE id=$1", observed.Incarnation); e != nil {
				return e
			}
			var active int
			if e = c.Pool.QueryRow(ctx, "SELECT count(*) FROM linha_attempts WHERE worker_id=$1 AND state='RUNNING'", observed.Incarnation).Scan(&active); e != nil {
				return e
			}
			if active == 0 {
				if e = c.valid(ctx, id, epoch); e != nil {
					return e
				}
				if e = c.Adapter.Stop(ctx, observed); e != nil {
					return e
				}
				if _, e = c.fenced(ctx, id, epoch, "UPDATE linha_instances SET state='DRAINING' WHERE id=$1", observed.ID); e != nil {
					return e
				}
				if r.state == "STARTING" {
					return fmt.Errorf("driver exited before worker registration or startup exceeded five minutes")
				}
			}
			continue
		}
		var active int
		var ready bool
		if e = c.Pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM linha_workers WHERE id=$1 AND NOT draining AND heartbeat>clock_timestamp()-interval '60 seconds'),(SELECT count(*) FROM linha_attempts WHERE worker_id=$1 AND state='RUNNING')", observed.Incarnation).Scan(&ready, &active); e != nil {
			return e
		}
		state := "STARTING"
		if observed.Ready && ready {
			state = "READY"
			if _, e = c.fenced(ctx, id, epoch, "UPDATE linha_contexts SET startup_failures=0 WHERE id=$1", id); e != nil {
				return e
			}
		}
		if _, e = c.fenced(ctx, id, epoch, "UPDATE linha_instances SET state=$2,last_busy=CASE WHEN $3>0 THEN clock_timestamp() ELSE last_busy END WHERE id=$1", observed.ID, state, active); e != nil {
			return e
		}
	}
	records, err = c.instances(ctx, id, image)
	if err != nil {
		return err
	}
	live, work, err = c.lifetime(ctx, id, epoch)
	if err != nil {
		return err
	}
	if !live && !work {
		return nil
	}
	var queued, running int
	var wait float64
	err = c.Pool.QueryRow(ctx, "SELECT count(*) FILTER(WHERE state IN ('QUEUED','RETRYING')),count(*) FILTER(WHERE state IN ('RUNNING','CANCELLING')),COALESCE(EXTRACT(EPOCH FROM clock_timestamp()-min(submitted_at) FILTER(WHERE state IN ('QUEUED','RETRYING'))),0)::float8 FROM linha_jobs WHERE context_id=$1", id).Scan(&queued, &running, &wait)
	if err != nil {
		return err
	}
	target := int(math.Ceil(float64(queued+running) / float64(settings.Drivers.Concurrency)))
	if live && target < settings.Drivers.Min {
		target = settings.Drivers.Min
	}
	if target > settings.Drivers.Max {
		target = settings.Drivers.Max
	}
	canScale := now.Sub(lastScaled) >= time.Duration(settings.Drivers.CooldownSeconds)*time.Second
	if target > len(records) && (len(records) < settings.Drivers.Min || len(records) == 0 || canScale && (queued >= settings.Drivers.QueueThreshold || wait >= float64(settings.Drivers.WaitSeconds))) {
		for n := len(records); n < target; n++ {
			if err = c.valid(ctx, id, epoch); err != nil {
				return err
			}
			instanceID := domain.ID()
			kind := "pod"
			if settings.Application != nil {
				kind = "sparkapplication"
			}
			tag, e := c.fenced(ctx, id, epoch, "INSERT INTO linha_instances(id,context_id,resource_kind) SELECT $2,id,$5 FROM linha_contexts WHERE id=$1 AND reconcile_owner=$3 AND reconcile_epoch=$4 AND reconcile_until>clock_timestamp()", id, instanceID, c.ID, epoch, kind)
			if e != nil {
				return e
			}
			if tag.RowsAffected() != 1 {
				return fmt.Errorf("reconciliation ownership lost")
			}
		}
		_, err = c.fenced(ctx, id, epoch, "UPDATE linha_contexts SET last_scaled=clock_timestamp() WHERE id=$1 AND reconcile_owner=$2 AND reconcile_epoch=$3", id, c.ID, epoch)
		return err
	}
	if target < len(records) && canScale {
		excess := len(records) - target
		for _, r := range records {
			if excess == 0 {
				break
			}
			if r.state != "READY" || now.Sub(r.lastBusy) < time.Duration(settings.Drivers.IdleSeconds)*time.Second {
				continue
			}
			if err = c.valid(ctx, id, epoch); err != nil {
				return err
			}
			tx, e := c.Pool.Begin(ctx)
			if e != nil {
				return e
			}
			var owned bool
			e = tx.QueryRow(ctx, "SELECT reconcile_owner=$2 AND reconcile_epoch=$3 AND reconcile_until>clock_timestamp() FROM linha_contexts WHERE id=$1 FOR NO KEY UPDATE", id, c.ID, epoch).Scan(&owned)
			if e == nil && !owned {
				e = fmt.Errorf("reconciliation ownership lost")
			}
			if e == nil {
				_, e = tx.Exec(ctx, "UPDATE linha_workers SET draining=true WHERE id=$1", r.instance.Incarnation)
			}
			if e == nil {
				_, e = tx.Exec(ctx, "UPDATE linha_instances SET state='DRAINING' WHERE id=$1", r.instance.ID)
			}
			if e != nil {
				tx.Rollback(ctx)
				return e
			}
			if e = tx.Commit(ctx); e != nil {
				return e
			}
			excess--
		}
		_, err = c.fenced(ctx, id, epoch, "UPDATE linha_contexts SET last_scaled=clock_timestamp() WHERE id=$1 AND reconcile_owner=$2 AND reconcile_epoch=$3", id, c.ID, epoch)
	}
	return err
}
func (c *Controller) instances(ctx context.Context, id, image string) ([]record, error) {
	rows, err := c.Pool.Query(ctx, "SELECT id,pod_uid,state,last_busy,created_at,resource_kind,resource_uid FROM linha_instances WHERE context_id=$1 AND state<>'DEAD' ORDER BY created_at,id", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []record
	for rows.Next() {
		r := record{instance: engine.Instance{ContextID: id, Image: image}}
		if err = rows.Scan(&r.instance.ID, &r.instance.Incarnation, &r.state, &r.lastBusy, &r.createdAt, &r.instance.Kind, &r.instance.ResourceUID); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
func (c *Controller) AuthorizeInstance(ctx context.Context, contextID, id, uid string) error {
	tag, err := c.Pool.Exec(ctx, "UPDATE linha_instances SET pod_uid=$3 WHERE context_id=$1 AND id=$2 AND (pod_uid='' OR pod_uid=$3) AND state IN ('STARTING','READY','DRAINING')", contextID, id, uid)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.Stale
	}
	return nil
}

// fenced locks the ownership row while applying a short database mutation. Network
// calls run outside transactions and are recovered through persisted instance IDs.
func (c *Controller) fenced(ctx context.Context, id string, epoch int64, sql string, args ...any) (pgconn.CommandTag, error) {
	tx, err := c.Pool.Begin(ctx)
	if err != nil {
		return pgconn.CommandTag{}, err
	}
	defer tx.Rollback(ctx)
	var valid bool
	if err = tx.QueryRow(ctx, "SELECT reconcile_owner=$2 AND reconcile_epoch=$3 AND reconcile_until>clock_timestamp() FROM linha_contexts WHERE id=$1 FOR NO KEY UPDATE", id, c.ID, epoch).Scan(&valid); err != nil {
		return pgconn.CommandTag{}, err
	}
	if !valid {
		return pgconn.CommandTag{}, fmt.Errorf("reconciliation ownership expired")
	}
	tag, err := tx.Exec(ctx, sql, args...)
	if err != nil {
		return tag, err
	}
	return tag, tx.Commit(ctx)
}

// Retired intents remain discoverable: an ambiguous, late create by a previous
// controller must not leave an untracked pod or service after failover.
func (c *Controller) cleanup(ctx context.Context, id string, epoch int64, image string) error {
	rows, err := c.Pool.Query(ctx, "SELECT id,pod_uid,resource_kind,resource_uid FROM linha_instances WHERE context_id=$1 AND state='DEAD'", id)
	if err != nil {
		return err
	}
	var retired []engine.Instance
	for rows.Next() {
		i := engine.Instance{ContextID: id, Image: image}
		if err = rows.Scan(&i.ID, &i.Incarnation, &i.Kind, &i.ResourceUID); err != nil {
			rows.Close()
			return err
		}
		retired = append(retired, i)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, i := range retired {
		if err = c.valid(ctx, id, epoch); err != nil {
			return err
		}
		probe := i
		probe.Incarnation = ""
		observed, e := c.Adapter.Observe(ctx, probe)
		if errors.Is(e, domain.NotFound) {
			if i.Incarnation != "" || i.ResourceUID != "" {
				if e = c.Adapter.Stop(ctx, i); e != nil {
					return e
				}
			}
			continue
		}
		if e != nil {
			return e
		}
		if e = c.Adapter.Stop(ctx, observed); e != nil {
			return e
		}
	}
	return nil
}

// Attachment, acceptance and retirement share this row lock. Once an instance is
// draining it never accepts new work, even if a client reattaches during teardown.
func (c *Controller) lifetime(ctx context.Context, id string, epoch int64) (live, work bool, err error) {
	tx, err := c.Pool.Begin(ctx)
	if err != nil {
		return false, false, err
	}
	defer tx.Rollback(ctx)
	var owned bool
	err = tx.QueryRow(ctx, "SELECT reconcile_owner=$2 AND reconcile_epoch=$3 AND reconcile_until>clock_timestamp() FROM linha_contexts WHERE id=$1 FOR NO KEY UPDATE", id, c.ID, epoch).Scan(&owned)
	if err != nil {
		return
	}
	if !owned {
		return false, false, fmt.Errorf("reconciliation ownership expired")
	}
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM linha_client_leases WHERE context_id=$1 AND expires_at>clock_timestamp()),EXISTS(SELECT 1 FROM linha_jobs WHERE context_id=$1 AND state IN ('QUEUED','RUNNING','RETRYING','CANCELLING'))`, id).Scan(&live, &work)
	if err != nil {
		return
	}
	if !live && !work {
		if _, err = tx.Exec(ctx, "UPDATE linha_workers SET draining=true WHERE context_id=$1", id); err != nil {
			return
		}
		if _, err = tx.Exec(ctx, "UPDATE linha_instances SET state='DRAINING' WHERE context_id=$1 AND state<>'DEAD'", id); err != nil {
			return
		}
	}
	_, err = tx.Exec(ctx, `UPDATE linha_contexts SET state=CASE WHEN NOT $2 AND NOT $3 AND NOT EXISTS(SELECT 1 FROM linha_instances WHERE context_id=$1 AND state<>'DEAD') THEN 'STOPPED' WHEN NOT $2 THEN 'DRAINING' WHEN EXISTS(SELECT 1 FROM linha_instances WHERE context_id=$1 AND state='READY') THEN 'READY' ELSE 'STARTING' END WHERE id=$1`, id, live, work)
	if err != nil {
		return
	}
	err = tx.Commit(ctx)
	return
}
