package postgres

import (
	"context"
	"fmt"
	"io"
)

// Metrics uses bounded label sets, never owner IDs, payloads, or credentials.
func (s *Store) Metrics(ctx context.Context, w io.Writer) error {
	rows, err := s.Pool.Query(ctx, "SELECT state,count(*) FROM linha_jobs GROUP BY state")
	if err != nil {
		return err
	}
	for rows.Next() {
		var state string
		var count int64
		if err = rows.Scan(&state, &count); err != nil {
			rows.Close()
			return err
		}
		fmt.Fprintf(w, "linha_jobs{state=%q} %d\n", state, count)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	var age float64
	var running, expired, workers, capacity, conditions, results int64
	err = s.Pool.QueryRow(ctx, `SELECT
 COALESCE(EXTRACT(EPOCH FROM clock_timestamp()-(SELECT min(submitted_at) FROM linha_jobs WHERE state IN ('QUEUED','RETRYING'))),0)::float8,
 (SELECT count(*) FROM linha_attempts WHERE state='RUNNING'),
 (SELECT count(*) FROM linha_attempts WHERE state='RUNNING' AND lease_expires_at<clock_timestamp()),
 (SELECT count(*) FROM linha_workers WHERE heartbeat>clock_timestamp()-interval '60 seconds' AND NOT draining),
 (SELECT COALESCE(sum(capacity),0) FROM linha_workers WHERE heartbeat>clock_timestamp()-interval '60 seconds' AND NOT draining),
 (SELECT count(*) FROM linha_contexts WHERE condition<>''),
 (SELECT count(*) FROM linha_jobs WHERE result IS NOT NULL AND result->>'expired'='false')`).Scan(&age, &running, &expired, &workers, &capacity, &conditions, &results)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "linha_queue_oldest_seconds %g\nlinha_running_attempts %d\nlinha_expired_leases %d\nlinha_ready_workers %d\nlinha_worker_slots %d\nlinha_backend_conditions %d\nlinha_retained_results %d\n", age, running, expired, workers, capacity, conditions, results)
	return err
}
