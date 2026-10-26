package postgres

import (
	"context"
	"errors"
	"log/slog"
	"slices"

	"github.com/jackc/pgx/v5"
	"linha/server/internal/domain"
)

func (s *Store) AdminBatch(ctx context.Context, actor string, in domain.AdminBatchRequest) (out domain.AdminBatchResult, err error) {
	if actor == "" || len(in.Jobs) < 1 || len(in.Jobs) > 200 {
		return out, domain.Bad("actor and 1-200 selected requests are required")
	}
	if in.Action != "move" && in.Action != "cancel" {
		return out, domain.Bad("action must be move or cancel")
	}
	if in.Action == "move" && (in.TargetContextID == "" || len(in.TargetContextID) > 256) || in.Action == "cancel" && in.TargetContextID != "" {
		return out, domain.Bad("only moves require a targetContextId")
	}
	seen := map[string]bool{}
	for _, job := range in.Jobs {
		if job.ID == "" || job.ContextID == "" || len(job.ID) > 256 || len(job.ContextID) > 256 || seen[job.ID] {
			return out, domain.Bad("selected requests require unique IDs and expected context IDs")
		}
		seen[job.ID] = true
	}
	out.Items = make([]domain.AdminBatchItem, 0, len(in.Jobs))
	for _, selected := range in.Jobs {
		item := domain.AdminBatchItem{ID: selected.ID}
		var job domain.Job
		e := s.tx(ctx, func(tx pgx.Tx) error {
			var e error
			if in.Action == "move" {
				job, item.Outcome, e = moveAdminJob(ctx, tx, actor, selected, in.TargetContextID)
			} else {
				job, e = cancelAdminJob(ctx, tx, actor, selected.ID, selected.ContextID)
				item.Outcome = "unchanged"
				if job.State == "CANCELLED" {
					item.Outcome = "cancelled"
				}
				if job.State == "CANCELLING" {
					item.Outcome = "cancelling"
				}
			}
			return e
		})
		if e != nil {
			item.Outcome = "failed"
			if !errors.As(e, &item.Error) {
				slog.ErrorContext(ctx, "administrative batch action unavailable", "jobId", selected.ID, "error", e)
				item.Error = &domain.Error{Code: "UNAVAILABLE", Message: "request action temporarily unavailable", Status: 503}
			}
		} else {
			item.Job = &domain.AdminBatchJob{ID: job.ID, ContextID: job.ContextID, State: job.State, AttemptCount: job.AttemptCount, UpdatedAt: job.UpdatedAt}
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

func moveAdminJob(ctx context.Context, tx pgx.Tx, actor string, selected domain.AdminJobSelection, targetID string) (out domain.Job, outcome string, err error) {
	rows, err := tx.Query(ctx, "SELECT "+contextCols+" FROM linha_contexts WHERE id=ANY($1::text[]) ORDER BY id FOR NO KEY UPDATE", []string{selected.ContextID, targetID})
	if err != nil {
		return
	}
	contexts := map[string]domain.BackendContext{}
	for rows.Next() {
		c, e := scanContext(rows)
		if e != nil {
			rows.Close()
			return out, "", e
		}
		contexts[c.ID] = c
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return
	}
	source, sourceOK := contexts[selected.ContextID]
	target, targetOK := contexts[targetID]
	if !sourceOK || !targetOK {
		return out, "", domain.NotFound
	}
	j, err := scanJob(tx.QueryRow(ctx, "SELECT "+jobCols+" FROM linha_jobs WHERE id=$1 FOR UPDATE", selected.ID))
	if err != nil {
		return out, "", err
	}
	if j.ContextID == targetID {
		return j, "unchanged", nil
	}
	if j.ContextID != selected.ContextID {
		return out, "", domain.Conflict("request context changed; refresh the selection")
	}
	if j.State != "QUEUED" && j.State != "RETRYING" {
		return out, "", domain.Conflict("only queued or retrying requests can move")
	}
	if source.Owner != target.Owner || source.Name != target.Name || source.Spec.Engine.Type != target.Spec.Engine.Type || source.Spec.Engine.Version != target.Spec.Engine.Version {
		return out, "", domain.Conflict("destination must be a version of the same owner's logical context and engine")
	}
	if target.State != "READY" && target.State != "STARTING" {
		return out, "", domain.Conflict("destination context must be active, not stopped or draining")
	}
	if source.Spec.Storage != nil {
		for _, name := range source.Spec.Storage.Destinations {
			if target.Spec.Storage == nil || !slices.Contains(target.Spec.Storage.Destinations, name) || source.Spec.Storage.Fingerprints[name] != target.Spec.Storage.Fingerprints[name] {
				return out, "", domain.Conflict("destination must retain source workload storage access")
			}
		}
	}
	var eligible bool
	err = tx.QueryRow(ctx, "SELECT (deadline IS NULL OR deadline>clock_timestamp()) AND NOT EXISTS(SELECT 1 FROM linha_attempts WHERE job_id=$1 AND state='RUNNING') FROM linha_jobs WHERE id=$1", j.ID).Scan(&eligible)
	if err != nil {
		return
	}
	if !eligible {
		return out, "", domain.Conflict("request expired or still has an active attempt")
	}
	if _, err = tx.Exec(ctx, "UPDATE linha_jobs SET context_id=$2,updated_at=clock_timestamp() WHERE id=$1", j.ID, targetID); err != nil {
		return
	}
	if _, err = tx.Exec(ctx, "INSERT INTO linha_admin_audit(id,actor,action,owner,context_id,source_context_id,job_id,outcome) VALUES($1,$2,'move',$3,$4,$5,$6,'MOVED')", domain.ID(), actor, source.Owner, targetID, source.ID, j.ID); err != nil {
		return
	}
	if _, err = tx.Exec(ctx, "UPDATE linha_contexts SET reconcile_after=clock_timestamp() WHERE id=ANY($1::text[])", []string{source.ID, targetID}); err != nil {
		return
	}
	out, err = scanJob(tx.QueryRow(ctx, "SELECT "+jobCols+" FROM linha_jobs WHERE id=$1", j.ID))
	return out, "moved", err
}
