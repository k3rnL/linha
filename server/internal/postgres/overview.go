package postgres

import (
	"context"
	"github.com/jackc/pgx/v5"
	"linha/server/internal/domain"
	"time"
)

func (s *Store) AdminOverview(ctx context.Context) (out domain.AdminOverview, err error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	out.ActiveContextDetails = []domain.OverviewContext{}
	out.RecentFailures = []domain.OverviewFailure{}
	if err = tx.QueryRow(ctx, "SELECT transaction_timestamp(),transaction_timestamp()-interval '24 hours'").Scan(&out.ObservedAt, &out.Since); err != nil {
		return out, err
	}
	err = tx.QueryRow(ctx, `SELECT
        count(*) FILTER (WHERE state='RUNNING'),
        count(*) FILTER (WHERE state IN ('QUEUED','RETRYING')),
        count(*) FILTER (WHERE state='RETRYING'),
        count(*) FILTER (WHERE state='CANCELLING'),
        greatest(COALESCE(EXTRACT(EPOCH FROM $1::timestamptz-min(submitted_at) FILTER (WHERE state IN ('QUEUED','RETRYING'))),0),0)::float8
        FROM linha_jobs WHERE state IN ('QUEUED','RETRYING','RUNNING','CANCELLING')`, out.ObservedAt).Scan(&out.RunningJobs, &out.QueuedJobs, &out.RetryingJobs, &out.CancellingJobs, &out.OldestQueuedSeconds)
	if err != nil {
		return out, err
	}
	err = tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE state='SUCCEEDED'),count(*) FILTER (WHERE state='FAILED'),count(*) FILTER (WHERE state='CANCELLED')
        FROM linha_jobs WHERE state IN ('SUCCEEDED','FAILED','CANCELLED') AND updated_at >= $1 AND updated_at <= $2`, out.Since, out.ObservedAt).Scan(&out.SucceededJobs24h, &out.FailedJobs24h, &out.CancelledJobs24h)
	if err != nil {
		return out, err
	}
	err = tx.QueryRow(ctx, `SELECT avg(greatest(EXTRACT(EPOCH FROM finished_at-started_at),0))::float8 FROM linha_attempts WHERE finished_at >= $1 AND finished_at <= $2`, out.Since, out.ObservedAt).Scan(&out.MeanProcessingSeconds24h)
	if err != nil {
		return out, err
	}
	err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM linha_contexts WHERE state<>'STOPPED'),(SELECT count(*) FROM linha_client_leases WHERE expires_at>$1)`, out.ObservedAt).Scan(&out.ActiveContexts, &out.LiveClients)
	if err != nil {
		return out, err
	}
	err = tx.QueryRow(ctx, `SELECT count(*),COALESCE(sum(capacity),0) FROM linha_workers WHERE NOT draining AND heartbeat>$1::timestamptz-interval '60 seconds'`, out.ObservedAt).Scan(&out.ReadyWorkers, &out.WorkerSlots)
	if err != nil {
		return out, err
	}
	err = tx.QueryRow(ctx, `SELECT count(*) FROM linha_attempts a JOIN linha_workers w ON w.id=a.worker_id
        WHERE a.state='RUNNING' AND a.lease_expires_at>$1 AND NOT w.draining AND w.heartbeat>$1::timestamptz-interval '60 seconds'`, out.ObservedAt).Scan(&out.OccupiedSlots)
	if err != nil {
		return out, err
	}
	err = tx.QueryRow(ctx, `WITH latest AS (SELECT DISTINCT ON (pod) * FROM linha_servers WHERE heartbeat_at >= $1 ORDER BY pod,heartbeat_at DESC,id DESC)
        SELECT count(*) FILTER (WHERE state='ready' AND heartbeat_at>$2::timestamptz-interval '30 seconds'),
        count(*) FILTER (WHERE state='unready' AND heartbeat_at>$2::timestamptz-interval '30 seconds'),
        count(*) FILTER (WHERE state<>'stopping' AND heartbeat_at<=$2::timestamptz-interval '30 seconds') FROM latest`, out.Since, out.ObservedAt).Scan(&out.ServersReady, &out.ServersUnready, &out.ServersStale)
	if err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, `SELECT c.id,c.name,c.owner,c.state,left(c.condition,300),
        (SELECT count(*) FROM linha_jobs j WHERE j.context_id=c.id AND j.state IN ('RUNNING','CANCELLING')),
        (SELECT count(*) FROM linha_jobs j WHERE j.context_id=c.id AND j.state IN ('QUEUED','RETRYING'))
        FROM linha_contexts c WHERE c.state<>'STOPPED' ORDER BY (c.condition<>'') DESC,c.created_at DESC,c.id DESC LIMIT 5`)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var v domain.OverviewContext
		if err = rows.Scan(&v.ID, &v.Name, &v.Owner, &v.State, &v.Condition, &v.RunningJobs, &v.QueuedJobs); err != nil {
			rows.Close()
			return out, err
		}
		out.ActiveContextDetails = append(out.ActiveContextDetails, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	rows, err = tx.Query(ctx, `SELECT j.id,j.context_id,c.name,j.owner,j.request->>'handler',left(COALESCE(j.failure->>'message','Query failed'),300),j.updated_at
        FROM linha_jobs j JOIN linha_contexts c ON c.id=j.context_id WHERE j.state='FAILED' AND j.updated_at >= $1 AND j.updated_at <= $2 ORDER BY j.updated_at DESC,j.id DESC LIMIT 5`, out.Since, out.ObservedAt)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var v domain.OverviewFailure
		if err = rows.Scan(&v.ID, &v.ContextID, &v.ContextName, &v.Owner, &v.Handler, &v.Message, &v.UpdatedAt); err != nil {
			rows.Close()
			return out, err
		}
		out.RecentFailures = append(out.RecentFailures, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}
