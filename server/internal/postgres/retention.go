package postgres

import (
	"context"
	"encoding/json"
	"linha/server/internal/domain"
)

// ExpireResults retains all files, objects, job IDs, and completion receipts.
// Physical deletion is deliberately not enabled by this operation.
func (s *Store) ExpireResults(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx, `UPDATE linha_jobs SET result=jsonb_set(result,'{expired}','true') WHERE state='SUCCEEDED' AND result->>'expired'='false' AND (result->>'expiresAt')::timestamptz<=clock_timestamp()`)
	return err
}
func (s *Store) Attempts(ctx context.Context, owner, job string) ([]domain.Attempt, error) {
	if _, err := s.Job(ctx, owner, job); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, "SELECT id,number,state,worker_id,started_at,finished_at,failure FROM linha_attempts WHERE job_id=$1 ORDER BY number", job)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.Attempt{}
	for rows.Next() {
		var a domain.Attempt
		var failure []byte
		if err = rows.Scan(&a.ID, &a.Number, &a.State, &a.WorkerID, &a.StartedAt, &a.FinishedAt, &failure); err != nil {
			return nil, err
		}
		if failure != nil {
			if err = json.Unmarshal(failure, &a.Failure); err != nil {
				return nil, err
			}
		}
		result = append(result, a)
	}
	return result, rows.Err()
}
