package postgres

import (
	"context"
	"errors"
	"linha/server/internal/domain"
	"linha/server/internal/storage"
	"time"

	"github.com/jackc/pgx/v5"
)

// CleanupOutputs removes only allocations proven unreferenced or expired while
// holding the job and output locks. Provider failures leave a retryable tombstone.
// A late writer can only recreate its own obsolete prefix; repeat sweeps remove it.
func (s *Store) CleanupOutputs(ctx context.Context, registry storage.Registry, grace time.Duration, limit int) (int, error) {
	if grace < time.Minute || limit < 1 || limit > 1000 {
		return 0, domain.Bad("cleanup needs at least one minute grace and batch size 1-1000")
	}
	cleaned := 0
	for cleaned < limit {
		found := false
		err := s.tx(ctx, func(tx pgx.Tx) error {
			row := tx.QueryRow(ctx, `SELECT o.id,o.job_id,o.attempt_id,o.name,o.kind,o.content_type,o.key,o.policy
    FROM linha_outputs o JOIN linha_attempts a ON a.id=o.attempt_id JOIN linha_jobs j ON j.id=o.job_id
    WHERE a.state<>'RUNNING' AND a.finished_at IS NOT NULL
      AND GREATEST(a.finished_at,o.created_at,o.write_until)+$1*interval '1 second'<clock_timestamp()
      AND (o.cleaned_at IS NULL OR o.cleaned_at<clock_timestamp()-interval '1 hour')
      AND (j.result IS NULL OR (j.result->>'expiresAt')::timestamptz<=clock_timestamp() OR
        (NOT EXISTS(SELECT 1 FROM jsonb_array_elements(j.result->'files') f WHERE f->>'allocationId'=o.id)
          AND COALESCE(j.result->'dataset'->>'allocationId','')<>o.id))
    ORDER BY o.cleaned_at NULLS FIRST,o.created_at LIMIT 1 FOR UPDATE OF j,o SKIP LOCKED`, grace.Seconds())
			allocation, err := scanAllocation(row)
			if errors.Is(err, domain.NotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			found = true
			provider, err := registry.For(allocation.Policy)
			if err != nil {
				return err
			}
			call, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			if allocation.Kind == "dataset" {
				d, ok := provider.(storage.DatasetBackend)
				if !ok {
					return domain.Bad("dataset cleanup unsupported")
				}
				err = d.DeleteDataset(call, allocation)
			} else {
				err = provider.Delete(call, allocation)
			}
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, "UPDATE linha_outputs SET cleaned_at=clock_timestamp() WHERE id=$1", allocation.ID)
			return err
		})
		if err != nil {
			return cleaned, err
		}
		if !found {
			break
		}
		cleaned++
	}
	return cleaned, nil
}
