package postgres

import (
	"context"
	"encoding/hex"

	"linha/server/internal/domain"
	"linha/server/internal/pathspec"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Store) DelegateOutput(ctx context.Context, job string, u domain.AttemptUpdate, id string, until time.Time) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		if err := owned(ctx, tx, job, u, false); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, "UPDATE linha_outputs SET write_until=GREATEST(write_until,$4) WHERE id=$1 AND job_id=$2 AND attempt_id=$3 AND NOT frozen", id, job, u.AttemptID, until)
		if err == nil && tag.RowsAffected() != 1 {
			return domain.Conflict("output is frozen or unavailable")
		}
		return err
	})
}
func (s *Store) RegisterParts(ctx context.Context, job string, u domain.AttemptUpdate, id string, parts []domain.DatasetPart) error {
	if len(parts) < 1 || len(parts) > 200 {
		return domain.Bad("register 1 to 200 parts per batch")
	}
	for _, p := range parts {
		if err := pathspec.Name(p.Path); err != nil {
			return domain.Bad("invalid dataset part path")
		}
		if p.Size < 0 || len(p.SHA256) != 64 {
			return domain.Bad("invalid part metadata")
		}
		if _, err := hex.DecodeString(p.SHA256); err != nil {
			return domain.Bad("invalid part digest")
		}
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		if err := owned(ctx, tx, job, u, false); err != nil {
			return err
		}
		var frozen bool
		var policy []byte
		if err := tx.QueryRow(ctx, "SELECT frozen,policy FROM linha_outputs WHERE id=$1 AND job_id=$2 AND attempt_id=$3 AND kind='dataset' FOR UPDATE", id, job, u.AttemptID).Scan(&frozen, &policy); err != nil {
			return missing(err)
		}
		if frozen {
			return domain.Conflict("dataset part list is frozen")
		}
		for _, p := range parts {
			tag, err := tx.Exec(ctx, `INSERT INTO linha_parts(allocation_id,path,size,sha256) VALUES($1,$2,$3,$4) ON CONFLICT(allocation_id,path) DO UPDATE SET size=linha_parts.size WHERE linha_parts.size=EXCLUDED.size AND linha_parts.sha256=EXCLUDED.sha256`, id, p.Path, p.Size, p.SHA256)
			if err != nil {
				return err
			}
			if tag.RowsAffected() != 1 {
				return domain.Conflict("part metadata differs from previous registration")
			}
		}
		p, err := domain.Decode[domain.ResultPolicy](policy)
		if err != nil {
			return err
		}
		var count, total int64
		if err = tx.QueryRow(ctx, "SELECT count(*),COALESCE(sum(size),0) FROM linha_parts WHERE allocation_id=$1", id).Scan(&count, &total); err != nil {
			return err
		}
		if count > 100000 || total > p.MaxBytes {
			return &domain.Error{Code: "RESULT_TOO_LARGE", Message: "dataset exceeds 100000 parts or configured byte limit", Status: 413}
		}
		return nil
	})
}
func (s *Store) DatasetParts(ctx context.Context, id, cursor string, limit int) (out domain.PartPage, err error) {
	if limit < 1 || limit > 200 {
		return out, domain.Bad("part page limit must be 1-200")
	}
	out.Items = []domain.DatasetPart{}
	rows, err := s.Pool.Query(ctx, "SELECT path,size,sha256 FROM linha_parts WHERE allocation_id=$1 AND path>$2 ORDER BY path LIMIT $3", id, cursor, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var p domain.DatasetPart
		if err = rows.Scan(&p.Path, &p.Size, &p.SHA256); err != nil {
			return out, err
		}
		out.Items = append(out.Items, p)
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = out.Items[limit-1].Path
	}
	return out, rows.Err()
}
func (s *Store) FreezeDataset(ctx context.Context, job string, u domain.AttemptUpdate, id string) (count, total int64, err error) {
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := owned(ctx, tx, job, u, false); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, "UPDATE linha_outputs SET frozen=true WHERE id=$1 AND job_id=$2 AND attempt_id=$3 AND kind='dataset'", id, job, u.AttemptID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return domain.NotFound
		}
		return tx.QueryRow(ctx, "SELECT count(*),COALESCE(sum(size),0) FROM linha_parts WHERE allocation_id=$1", id).Scan(&count, &total)
	})
	return
}
func (s *Store) SealDataset(ctx context.Context, job string, u domain.AttemptUpdate, d domain.Dataset) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		if err := owned(ctx, tx, job, u, false); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, "UPDATE linha_outputs SET dataset=$4 WHERE id=$1 AND job_id=$2 AND attempt_id=$3 AND frozen AND (dataset IS NULL OR dataset=$4::jsonb)", d.AllocationID, job, u.AttemptID, domain.JSON(d))
		if err == nil && tag.RowsAffected() != 1 {
			return domain.Conflict("dataset was sealed differently")
		}
		return err
	})
}
func (s *Store) PublishedDataset(ctx context.Context, owner, job string) (a domain.Allocation, d domain.Dataset, err error) {
	j, err := s.Job(ctx, owner, job)
	if err != nil {
		return a, d, err
	}
	if j.Result == nil || j.Result.Dataset == nil {
		return a, d, domain.NotFound
	}
	d = *j.Result.Dataset
	if _, _, err = s.ResultOutput(ctx, owner, job, d.Manifest.AllocationID); err != nil {
		return a, d, err
	}
	a, err = scanAllocation(s.Pool.QueryRow(ctx, "SELECT "+outputCols+" FROM linha_outputs WHERE id=$1 AND job_id=$2", d.AllocationID, job))
	return
}

func (s *Store) DatasetPart(ctx context.Context, id, name string) (p domain.DatasetPart, err error) {
	err = s.Pool.QueryRow(ctx, "SELECT path,size,sha256 FROM linha_parts WHERE allocation_id=$1 AND path=$2", id, name).Scan(&p.Path, &p.Size, &p.SHA256)
	return p, missing(err)
}

func (s *Store) ContextResultPolicy(ctx context.Context, id string) (p domain.ResultPolicy, err error) {
	var raw []byte
	err = s.Pool.QueryRow(ctx, "SELECT spec->'results' FROM linha_contexts WHERE id=$1", id).Scan(&raw)
	if err != nil {
		return p, missing(err)
	}
	return domain.Decode[domain.ResultPolicy](raw)
}
func (s *Store) StorageCondition(ctx context.Context, id, message string) error {
	_, err := s.Pool.Exec(ctx, "UPDATE linha_contexts SET condition=$2 WHERE id=$1", id, message)
	return err
}
