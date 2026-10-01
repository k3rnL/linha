package postgres

import (
	"context"
	"errors"
	"linha/server/internal/domain"
	"testing"
)

func TestAllocationReceiptTraversalAndExpiredMetadata(t *testing.T) {
	s := testStore(t)
	c := backend(t, s)
	ctx := context.Background()
	j := submit(t, s, c, "allocation", 1)
	w := worker(t, s, c, "w", 1)
	a, e := s.Claim(ctx, w.ID, w.Incarnation)
	if e != nil {
		t.Fatal(e)
	}
	u := update(a, w)
	request := domain.OutputRequest{AttemptUpdate: u, Name: "result.json", Kind: "json", ContentType: "application/json"}
	one, e := s.Allocate(ctx, j.ID, request)
	if e != nil {
		t.Fatal(e)
	}
	two, e := s.Allocate(ctx, j.ID, request)
	if e != nil || one.ID != two.ID || one.Key != two.Key {
		t.Fatalf("lost allocation response: %+v %+v %v", one, two, e)
	}
	for _, name := range []string{"../result", "%2e%2e/result", "_linha-owned", "result.json/child"} {
		bad := request
		bad.Name = name
		if _, e = s.Allocate(ctx, j.ID, bad); e == nil {
			t.Fatalf("unsafe/overlapping name accepted: %s", name)
		}
	}
	file := domain.File{AllocationID: one.ID, Name: one.Name, ContentType: one.ContentType, SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", Size: 2}
	completion := domain.Completion{AttemptUpdate: u, Descriptor: a.ResultDescriptor, Files: []domain.File{file}}
	if _, e = s.Complete(ctx, j.ID, completion); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Pool.Exec(ctx, `UPDATE linha_jobs SET result=jsonb_set(result,'{expiresAt}',to_jsonb(clock_timestamp()-interval '1 second')) WHERE id=$1`, j.ID); e != nil {
		t.Fatal(e)
	}
	if e = s.ExpireResults(ctx); e != nil {
		t.Fatal(e)
	}
	history, e := s.Attempts(ctx, "owner", j.ID)
	if e != nil || len(history) != 1 || history[0].State != "SUCCEEDED" {
		t.Fatalf("history: %+v %v", history, e)
	}
	result, e := s.Job(ctx, "owner", j.ID)
	if e != nil || result.State != "SUCCEEDED" || !result.Result.Expired {
		t.Fatalf("expiry changed durable outcome: %+v %v", result, e)
	}
	_, _, e = s.ResultOutput(ctx, "owner", j.ID, one.ID)
	var status *domain.Error
	if !errors.As(e, &status) || status.Code != "RESULT_EXPIRED" {
		t.Fatalf("expiry: %v", e)
	}
	var allocations int
	if e = s.Pool.QueryRow(ctx, "SELECT count(*) FROM linha_outputs WHERE job_id=$1", j.ID).Scan(&allocations); e != nil || allocations != 1 {
		t.Fatal("expiry removed retained allocation metadata")
	}
	if _, e = s.Attempts(ctx, "other", j.ID); !errors.Is(e, domain.NotFound) {
		t.Fatal("cross-owner attempt history")
	}
}

func TestRetryExhaustionAndDeadlineRecovery(t *testing.T) {
	s := testStore(t)
	c := backend(t, s)
	ctx := context.Background()
	j := submit(t, s, c, "retry-exhaustion", 2)
	w := worker(t, s, c, "worker", 1)
	for attempt := 1; attempt <= 2; attempt++ {
		a, e := s.Claim(ctx, w.ID, w.Incarnation)
		if e != nil || a == nil {
			t.Fatalf("attempt %d: %v", attempt, e)
		}
		if e = s.Fail(ctx, j.ID, update(a, w), domain.Failure{Code: "TRANSIENT", Message: "retryable error", Retryable: true}); e != nil {
			t.Fatal(e)
		}
		if _, e = s.Pool.Exec(ctx, "UPDATE linha_jobs SET available_at=clock_timestamp() WHERE id=$1", j.ID); e != nil {
			t.Fatal(e)
		}
	}
	result, e := s.Job(ctx, "owner", j.ID)
	if e != nil || result.State != "FAILED" || result.AttemptCount != 2 {
		t.Fatalf("exhaustion %+v %v", result, e)
	}
	deadlineJob := submit(t, s, c, "deadline", 3)
	if _, e = s.Pool.Exec(ctx, "UPDATE linha_jobs SET deadline=clock_timestamp()-interval '1 second' WHERE id=$1", deadlineJob.ID); e != nil {
		t.Fatal(e)
	}
	if e = s.Recover(ctx); e != nil {
		t.Fatal(e)
	}
	result, e = s.Job(ctx, "owner", deadlineJob.ID)
	if e != nil || result.State != "FAILED" || result.Failure.Code != "DEADLINE_EXCEEDED" || result.AttemptCount != 0 {
		t.Fatalf("deadline %+v %v", result, e)
	}
}
