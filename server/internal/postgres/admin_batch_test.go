package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"linha/server/internal/domain"
)

func recoveryTarget(t *testing.T, s *Store, source domain.BackendContext) domain.BackendContext {
	t.Helper()
	spec := source.Spec
	spec.Image = "fixed-image"
	target, err := s.Ensure(context.Background(), "owner", domain.EnsureRequest{Name: source.Name, Spec: spec})
	if err != nil {
		t.Fatal(err)
	}
	return target
}
func batch(t *testing.T, s *Store, action, target string, jobs ...domain.Job) domain.AdminBatchResult {
	t.Helper()
	in := domain.AdminBatchRequest{Action: action, TargetContextID: target}
	for _, job := range jobs {
		in.Jobs = append(in.Jobs, domain.AdminJobSelection{ID: job.ID, ContextID: job.ContextID})
	}
	out, err := s.AdminBatch(context.Background(), "operator", in)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func TestAdminQueueMoveDurabilityIdempotencyAuditAndExecution(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	source := backend(t, s)
	job := submit(t, s, source, "move-source", 2)
	adminInput := domain.AdminSubmitRequest{SubmitRequest: domain.SubmitRequest{Handler: "sum", Version: 1, Payload: json.RawMessage(`{"n":2}`), IdempotencyKey: "admin-source"}}
	adminJob, err := s.AdminSubmit(ctx, "operator", source.ID, adminInput)
	if err != nil {
		t.Fatal(err)
	}
	target := recoveryTarget(t, s, source)
	if err = s.ReleaseClient(ctx, "owner", source.ID, source.ClientLease.ID); err != nil {
		t.Fatal(err)
	}
	var originalPolicy []byte
	if err = s.Pool.QueryRow(ctx, "SELECT policy FROM linha_jobs WHERE id=$1", job.ID).Scan(&originalPolicy); err != nil {
		t.Fatal(err)
	}
	result := batch(t, s, "move", target.ID, job, adminJob)
	for _, item := range result.Items {
		if item.Error != nil || item.Outcome != "moved" {
			t.Fatalf("move: %+v", item)
		}
	}
	if duplicate := batch(t, s, "move", target.ID, job); duplicate.Items[0].Outcome != "unchanged" {
		t.Fatal(duplicate)
	}
	duplicate, err := s.Submit(ctx, "owner", source.ID, job.Request)
	if err != nil || duplicate.ID != job.ID {
		t.Fatal("SDK idempotency changed", duplicate, err)
	}
	duplicate, err = s.AdminSubmit(ctx, "operator", source.ID, adminInput)
	if err != nil || duplicate.ID != adminJob.ID {
		t.Fatal("admin idempotency changed", duplicate, err)
	}
	otherPool, err := pgxpool.NewWithConfig(ctx, s.Pool.Config().Copy())
	if err != nil {
		t.Fatal(err)
	}
	defer otherPool.Close()
	second := &Store{Pool: otherPool, Lease: s.Lease}
	moved, err := second.Job(ctx, "owner", job.ID)
	if err != nil || moved.ContextID != target.ID || !moved.SubmittedAt.Equal(job.SubmittedAt) || moved.AttemptCount != job.AttemptCount || !jsonEqual(domain.JSON(job.Request), domain.JSON(moved.Request)) {
		t.Fatal("move changed request", moved, err)
	}
	var samePolicy bool
	if err = s.Pool.QueryRow(ctx, "SELECT policy=$2::jsonb FROM linha_jobs WHERE id=$1", job.ID, originalPolicy).Scan(&samePolicy); err != nil || !samePolicy {
		t.Fatal("result policy changed", err)
	}
	old, err := s.OperationalContext(ctx, "owner", source.ID)
	if err != nil || old.LiveClients != 0 || old.QueuedJobs != 0 {
		t.Fatal("source cannot drain", old, err)
	}
	to, err := s.OperationalContext(ctx, "owner", target.ID)
	if err != nil || to.QueuedJobs != 2 {
		t.Fatal("target queue incorrect", to, err)
	}
	var gauge float64
	if err = s.Pool.QueryRow(ctx, "SELECT sum(value) FROM linha_metric_values WHERE name='linha_job_records' AND labels->>'state'='QUEUED'").Scan(&gauge); err != nil || gauge != 2 {
		t.Fatal("queue gauge drift", gauge, err)
	}
	// A real later move creates another audit event, while no-op retries do not.
	_, err = s.Pool.Exec(ctx, "UPDATE linha_contexts SET state='READY' WHERE id=$1", source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result = batch(t, s, "move", source.ID, moved); result.Items[0].Error != nil {
		t.Fatal(result)
	}
	back, err := s.Job(ctx, "owner", job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result = batch(t, s, "move", target.ID, back); result.Items[0].Error != nil {
		t.Fatal(result)
	}
	audit, err := s.AdminAudit(ctx, job.ID, domain.OperationalFilter{})
	if err != nil || len(audit.Items) != 3 || audit.Items[0].SourceContextID != source.ID || audit.Items[0].ContextID != target.ID {
		t.Fatal("move history lost", audit, err)
	}
	w := worker(t, s, target, "destination-worker", 2)
	assignment, err := second.Claim(ctx, w.ID, w.Incarnation)
	if err != nil || assignment == nil || assignment.Job.ContextID != target.ID {
		t.Fatal("destination cannot execute", assignment, err)
	}
	oldWorker := worker(t, s, source, "old-worker", 1)
	if oldAssignment, e := s.Claim(ctx, oldWorker.ID, oldWorker.Incarnation); e != nil || oldAssignment != nil {
		t.Fatal("old queue claimed moved work", oldAssignment, e)
	}
}

func TestAdminBatchPartialResultsCompatibilityAndCancellation(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	source := backend(t, s)
	running := submit(t, s, source, "running", 1)
	w := worker(t, s, source, "worker", 1)
	a, err := s.Claim(ctx, w.ID, w.Incarnation)
	if err != nil || a == nil {
		t.Fatal(err)
	}
	queued := submit(t, s, source, "queued", 1)
	target := recoveryTarget(t, s, source)
	result := batch(t, s, "move", target.ID, queued, running)
	if result.Items[0].Outcome != "moved" || result.Items[1].Error == nil {
		t.Fatal(result)
	}
	result = batch(t, s, "cancel", "", running)
	if result.Items[0].Outcome != "cancelling" {
		t.Fatal(result)
	}
	queued.ContextID = target.ID
	result = batch(t, s, "cancel", "", queued)
	if result.Items[0].Outcome != "cancelled" {
		t.Fatal(result)
	}
	next := submit(t, s, source, "next", 1)
	for _, state := range []string{"DRAINING", "STOPPED"} {
		if _, err = s.Pool.Exec(ctx, "UPDATE linha_contexts SET state=$2 WHERE id=$1", target.ID, state); err != nil {
			t.Fatal(err)
		}
		if got := batch(t, s, "move", target.ID, next); got.Items[0].Error == nil {
			t.Fatal("retired destination accepted")
		}
	}
	_, err = s.Pool.Exec(ctx, "UPDATE linha_contexts SET state='READY',owner='different' WHERE id=$1", target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := batch(t, s, "move", target.ID, next); got.Items[0].Error == nil {
		t.Fatal("cross-owner move accepted")
	}
	_, err = s.Pool.Exec(ctx, "UPDATE linha_contexts SET owner='owner',name='different' WHERE id=$1", target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := batch(t, s, "move", target.ID, next); got.Items[0].Error == nil {
		t.Fatal("cross-name move accepted")
	}
	selection := domain.AdminJobSelection{ID: next.ID, ContextID: next.ContextID}
	if _, err = s.AdminBatch(ctx, "operator", domain.AdminBatchRequest{Action: "cancel", Jobs: []domain.AdminJobSelection{selection, selection}}); err == nil {
		t.Fatal("duplicate batch accepted")
	}
}

func TestAdminMoveRacesWithWorkerClaim(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	source := backend(t, s)
	target := recoveryTarget(t, s, source)
	w := worker(t, s, source, "racing-worker", 32)
	for i := 0; i < 12; i++ {
		job := submit(t, s, source, fmt.Sprintf("race-%d", i), 1)
		var wg sync.WaitGroup
		wg.Add(2)
		var a *domain.Assignment
		var claimErr, batchErr error
		var moved domain.AdminBatchResult
		go func() { defer wg.Done(); a, claimErr = s.Claim(ctx, w.ID, w.Incarnation) }()
		go func() {
			defer wg.Done()
			moved, batchErr = s.AdminBatch(ctx, "operator", domain.AdminBatchRequest{Action: "move", TargetContextID: target.ID, Jobs: []domain.AdminJobSelection{{ID: job.ID, ContextID: source.ID}}})
		}()
		wg.Wait()
		if claimErr != nil || batchErr != nil {
			t.Fatal(claimErr, batchErr)
		}
		if a != nil && moved.Items[0].Error == nil {
			t.Fatal("claim and move both succeeded")
		}
		if a == nil && moved.Items[0].Outcome != "moved" {
			t.Fatal("neither won", moved)
		}
	}
}

func TestAdminMoveRetainsRetryHistoryAndBackoff(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	source := backend(t, s)
	job := submit(t, s, source, "retry", 2)
	w := worker(t, s, source, "old-retry-worker", 1)
	a, err := s.Claim(ctx, w.ID, w.Incarnation)
	if err != nil || a == nil {
		t.Fatal(a, err)
	}
	if err = s.Fail(ctx, job.ID, update(a, w), domain.Failure{Code: "BUSINESS_ERROR", Message: "retry", Retryable: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Pool.Exec(ctx, "UPDATE linha_jobs SET available_at=clock_timestamp()+interval '1 hour' WHERE id=$1", job.ID); err != nil {
		t.Fatal(err)
	}
	target := recoveryTarget(t, s, source)
	before, err := s.Job(ctx, "owner", job.ID)
	if err != nil {
		t.Fatal(err)
	}
	var beforeAvailable, afterAvailable string
	if err = s.Pool.QueryRow(ctx, "SELECT available_at::text FROM linha_jobs WHERE id=$1", job.ID).Scan(&beforeAvailable); err != nil {
		t.Fatal(err)
	}
	got := batch(t, s, "move", target.ID, before)
	if got.Items[0].Error != nil || got.Items[0].Job.State != "RETRYING" || got.Items[0].Job.AttemptCount != 1 {
		t.Fatal(got)
	}
	if err = s.Pool.QueryRow(ctx, "SELECT available_at::text FROM linha_jobs WHERE id=$1", job.ID).Scan(&afterAvailable); err != nil || beforeAvailable != afterAvailable {
		t.Fatal("backoff reset", err)
	}
	next := worker(t, s, target, "new-retry-worker", 1)
	if a, err = s.Claim(ctx, next.ID, next.Incarnation); err != nil || a != nil {
		t.Fatal("backoff bypassed", a, err)
	}
	if _, err = s.Pool.Exec(ctx, "UPDATE linha_jobs SET available_at=clock_timestamp() WHERE id=$1", job.ID); err != nil {
		t.Fatal(err)
	}
	if a, err = s.Claim(ctx, next.ID, next.Incarnation); err != nil || a == nil || a.Fence != 2 {
		t.Fatal("attempt history reset", a, err)
	}
}
func TestAdminMoveRejectsStorageEngineDeadlineAndRollsBackAuditFailure(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	source := backend(t, s)
	job := submit(t, s, source, "validate", 1)
	target := recoveryTarget(t, s, source)
	if _, err := s.Pool.Exec(ctx, `UPDATE linha_contexts SET spec=jsonb_set(spec,'{storage}',$2::jsonb) WHERE id=$1`, source.ID, `{"destinations":["inputs"],"fingerprints":{"inputs":"original"}}`); err != nil {
		t.Fatal(err)
	}
	if got := batch(t, s, "move", target.ID, job); got.Items[0].Error == nil {
		t.Fatal("missing storage access accepted")
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE linha_contexts SET spec=jsonb_set(spec,'{storage}',$2::jsonb) WHERE id=$1`, target.ID, `{"destinations":["inputs"],"fingerprints":{"inputs":"changed"}}`); err != nil {
		t.Fatal(err)
	}
	if got := batch(t, s, "move", target.ID, job); got.Items[0].Error == nil {
		t.Fatal("changed storage destination accepted")
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE linha_contexts SET spec=spec-'storage'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE linha_contexts SET spec=jsonb_set(spec,'{engine,version}','"different"') WHERE id=$1`, target.ID); err != nil {
		t.Fatal(err)
	}
	if got := batch(t, s, "move", target.ID, job); got.Items[0].Error == nil {
		t.Fatal("different engine contract accepted")
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE linha_contexts SET spec=jsonb_set(spec,'{engine,version}','"1"') WHERE id=$1`, target.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE linha_jobs SET deadline=clock_timestamp()-interval '1 second' WHERE id=$1`, job.ID); err != nil {
		t.Fatal(err)
	}
	if got := batch(t, s, "move", target.ID, job); got.Items[0].Error == nil {
		t.Fatal("expired request moved")
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE linha_jobs SET deadline=NULL WHERE id=$1`, job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `ALTER TABLE linha_admin_audit ADD CONSTRAINT reject_move CHECK (action<>'move')`); err != nil {
		t.Fatal(err)
	}
	if got := batch(t, s, "move", target.ID, job); got.Items[0].Error == nil || got.Items[0].Error.Code != "UNAVAILABLE" {
		t.Fatal("audit failure not surfaced", got)
	}
	current, err := s.Job(ctx, "owner", job.ID)
	if err != nil || current.ContextID != source.ID {
		t.Fatal("audit failure did not roll back move", current, err)
	}
}
