package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"linha/server/internal/domain"
	"linha/server/internal/pathspec"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("LINHA_TEST_DATABASE")
	if dsn == "" {
		t.Skip("set LINHA_TEST_DATABASE to run isolated PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	schema := "test_" + domain.ID()
	if _, e = admin.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	config, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		t.Fatal(e)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	p, e := pgxpool.NewWithConfig(ctx, config)
	if e != nil {
		t.Fatal(e)
	}
	s := &Store{Pool: p, Lease: time.Second * 30}
	t.Cleanup(func() { p.Close(); _, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); admin.Close() })
	if e = s.migrate(ctx); e != nil {
		t.Fatal(e)
	}
	return s
}
func backend(t *testing.T, s *Store) domain.BackendContext {
	t.Helper()
	c, e := s.Ensure(context.Background(), "owner", domain.EnsureRequest{Name: "example", Spec: domain.BackendSpec{Image: "test", Engine: domain.EngineSpec{Type: "fake", Version: "1", Settings: json.RawMessage(`{}`)}, Results: domain.ResultPolicy{Type: "local", Root: t.TempDir(), Path: domain.PathTemplate{Version: 1, Template: pathspec.Default}, MaxBytes: 1024, RetentionSeconds: 3600}}})
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func submit(t *testing.T, s *Store, c domain.BackendContext, key string, max int) domain.Job {
	t.Helper()
	j, e := s.Submit(context.Background(), "owner", c.ID, domain.SubmitRequest{LeaseID: c.ClientLease.ID, Handler: "sum", Version: 1, Payload: json.RawMessage(`{"n":123}`), IdempotencyKey: key, Retry: domain.RetryPolicy{MaxAttempts: max}})
	if e != nil {
		t.Fatal(e)
	}
	return j
}
func worker(t *testing.T, s *Store, c domain.BackendContext, id string, capacity int) domain.Worker {
	t.Helper()
	w := domain.Worker{ID: id, ContextID: c.ID, Incarnation: domain.ID(), Capacity: capacity, Capabilities: []domain.Capability{{Handler: "sum", Version: 1, Result: domain.ResultDescriptor{Kind: "json", Schema: "sum.result", Version: 1}}}}
	if e := s.Register(context.Background(), w); e != nil {
		t.Fatal(e)
	}
	return w
}
func update(a *domain.Assignment, w domain.Worker) domain.AttemptUpdate {
	return domain.AttemptUpdate{WorkerID: w.ID, Incarnation: w.Incarnation, AttemptID: a.AttemptID, Fence: a.Fence}
}
func TestAcceptanceIdempotencyOwnershipAndConcurrentEnsure(t *testing.T) {
	s := testStore(t)
	c := backend(t, s)
	ctx := context.Background()
	var wg sync.WaitGroup
	ids := make(chan string, 20)
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ensured, e := s.Ensure(ctx, "owner", domain.EnsureRequest{Name: c.Name, Spec: c.Spec})
			if e != nil {
				errs <- e
				return
			}
			if ensured.ID != c.ID {
				errs <- errors.New("duplicate context")
				return
			}
			j, e := s.Submit(ctx, "owner", c.ID, domain.SubmitRequest{LeaseID: c.ClientLease.ID, Handler: "sum", Version: 1, Payload: json.RawMessage(`{"n":1}`), IdempotencyKey: "same"})
			if e != nil {
				errs <- e
			} else {
				ids <- j.ID
			}
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for e := range errs {
		t.Error(e)
	}
	first := ""
	for id := range ids {
		if first == "" {
			first = id
		}
		if id != first {
			t.Error("duplicate public IDs")
		}
	}
	if _, e := s.Job(ctx, "other", first); !errors.Is(e, domain.NotFound) {
		t.Fatalf("cross-owner job: %v", e)
	}
	if _, e := s.Cancel(ctx, "other", first); !errors.Is(e, domain.NotFound) {
		t.Fatalf("cross-owner cancel: %v", e)
	}
	_, e := s.Submit(ctx, "owner", c.ID, domain.SubmitRequest{LeaseID: c.ClientLease.ID, Handler: "sum", Version: 1, Payload: json.RawMessage(`{"n":2}`), IdempotencyKey: "same"})
	var conflict *domain.Error
	if !errors.As(e, &conflict) || conflict.Code != "CONFLICT" {
		t.Fatalf("conflict: %v", e)
	}
	changed := c.Spec
	changed.Image = "replacement:image"
	if v, err := s.Ensure(ctx, "owner", domain.EnsureRequest{Name: c.Name, Spec: changed}); err != nil || v.ID == c.ID {
		t.Fatal("new configuration must create a version", err)
	}
	page, e := s.List(ctx, "owner", "", "", "", 1)
	if e != nil || len(page.Items) != 1 {
		t.Fatalf("list: %v %+v", e, page)
	}
}
func TestClaimsCapacityAndRetryFencing(t *testing.T) {
	s := testStore(t)
	c := backend(t, s)
	ctx := context.Background()
	j := submit(t, s, c, "a", 2)
	w := worker(t, s, c, "w", 1)
	submit(t, s, c, "b", 1)
	a, e := s.Claim(ctx, w.ID, w.Incarnation)
	if e != nil || a == nil {
		t.Fatalf("claim: %v", e)
	}
	if a.Job.ID != j.ID {
		t.Fatal("queue ordering")
	}
	other, e := s.Claim(ctx, w.ID, w.Incarnation)
	if e != nil || other != nil {
		t.Fatalf("capacity exceeded %v", e)
	}
	u := update(a, w)
	if _, e = s.Renew(ctx, j.ID, u, &domain.Progress{Fraction: 0.5, Message: "working"}); e != nil {
		t.Fatal(e)
	}
	_, e = s.Pool.Exec(ctx, "UPDATE linha_attempts SET lease_expires_at=clock_timestamp()-interval '1 second' WHERE id=$1", a.AttemptID)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Recover(ctx); e != nil {
		t.Fatal(e)
	}
	after, e := s.Job(ctx, "owner", j.ID)
	if e != nil || after.State != "RETRYING" || !after.SubmittedAt.Equal(j.SubmittedAt) {
		t.Fatalf("retry %+v %v", after, e)
	}
	if _, e = s.Renew(ctx, j.ID, u, nil); !errors.Is(e, domain.Stale) {
		t.Fatalf("stale renew: %v", e)
	}
	if _, e = s.Allocate(ctx, j.ID, domain.OutputRequest{AttemptUpdate: u, Name: "result.json", Kind: "json", ContentType: "application/json"}); !errors.Is(e, domain.Stale) {
		t.Fatalf("stale output: %v", e)
	}
	_, _ = s.Pool.Exec(ctx, "UPDATE linha_jobs SET available_at=clock_timestamp() WHERE id=$1", j.ID)
	a2, e := s.Claim(ctx, w.ID, w.Incarnation)
	if e != nil || a2 == nil || a2.AttemptID == a.AttemptID || a2.Fence <= a.Fence {
		t.Fatalf("replacement: %+v %v", a2, e)
	}
}
func TestCancelCompletionOrderingAndRestartReceipt(t *testing.T) {
	s := testStore(t)
	c := backend(t, s)
	ctx := context.Background()
	j := submit(t, s, c, "completed", 1)
	w := worker(t, s, c, "w", 1)
	a, e := s.Claim(ctx, w.ID, w.Incarnation)
	if e != nil {
		t.Fatal(e)
	}
	u := update(a, w)
	output, e := s.Allocate(ctx, j.ID, domain.OutputRequest{AttemptUpdate: u, Name: "result.json", Kind: "json", ContentType: "application/json"})
	if e != nil {
		t.Fatal(e)
	}
	completion := domain.Completion{AttemptUpdate: u, Descriptor: a.ResultDescriptor, Files: []domain.File{{AllocationID: output.ID, Name: output.Name, ContentType: output.ContentType, Size: 10, SHA256: strings.Repeat("a", 64)}}}
	r, e := s.Complete(ctx, j.ID, completion)
	if e != nil {
		t.Fatal(e)
	}
	replica := &Store{Pool: s.Pool, Lease: s.Lease}
	again, e := replica.Complete(ctx, j.ID, completion)
	if e != nil || !again.ExpiresAt.Equal(r.ExpiresAt) {
		t.Fatalf("receipt: %v", e)
	}
	cancelled, e := replica.Cancel(ctx, "owner", j.ID)
	if e != nil || cancelled.State != "SUCCEEDED" {
		t.Fatal("cancel overwrote success")
	}
	_, _, e = replica.ResultOutput(ctx, "owner", j.ID, output.ID)
	if e != nil {
		t.Fatal(e)
	}
	j2 := submit(t, s, c, "cancelled", 1)
	a, e = s.Claim(ctx, w.ID, w.Incarnation)
	if e != nil {
		t.Fatal(e)
	}
	u = update(a, w)
	if _, e = s.Cancel(ctx, "owner", j2.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Renew(ctx, j2.ID, u, nil); !errors.Is(e, domain.Stale) {
		t.Fatalf("cancel did not fence renewal: %v", e)
	}
	if e = s.Fail(ctx, j2.ID, u, domain.Failure{Code: "CANCELLED", Message: "stopped"}); e != nil {
		t.Fatal(e)
	}
	after, e := s.Job(ctx, "owner", j2.ID)
	if e != nil || after.State != "CANCELLED" {
		t.Fatalf("cancelled state: %+v %v", after, e)
	}
}
func TestCompetingWorkersNeverClaimSameAttempt(t *testing.T) {
	s := testStore(t)
	c := backend(t, s)
	ctx := context.Background()
	submit(t, s, c, "one", 1)
	w1 := worker(t, s, c, "w1", 1)
	w2 := worker(t, s, c, "w2", 1)
	var wg sync.WaitGroup
	results := make(chan *domain.Assignment, 2)
	errs := make(chan error, 2)
	for _, w := range []domain.Worker{w1, w2} {
		wg.Add(1)
		go func(w domain.Worker) {
			defer wg.Done()
			a, e := s.Claim(ctx, w.ID, w.Incarnation)
			results <- a
			errs <- e
		}(w)
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Error(e)
		}
	}
	n := 0
	for a := range results {
		if a != nil {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d concurrent claims", n)
	}
}

func TestUnsupportedHandlerAndUnavailableDatabase(t *testing.T) {
	s := testStore(t)
	c := backend(t, s)
	ctx := context.Background()
	worker(t, s, c, "w", 1)
	j, e := s.Submit(ctx, "owner", c.ID, domain.SubmitRequest{LeaseID: c.ClientLease.ID, Handler: "unknown", Version: 1, Payload: json.RawMessage(`{}`), IdempotencyKey: "unsupported"})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Recover(ctx); e != nil {
		t.Fatal(e)
	}
	j, e = s.Job(ctx, "owner", j.ID)
	if e != nil || j.State != "FAILED" || j.Failure.Code != "UNSUPPORTED_HANDLER" {
		t.Fatalf("unsupported handler: %+v %v", j, e)
	}
	s.Close()
	if _, e = s.Submit(ctx, "owner", c.ID, domain.SubmitRequest{LeaseID: c.ClientLease.ID, Handler: "sum", Version: 1, Payload: json.RawMessage(`{}`), IdempotencyKey: "must-not-accept"}); e == nil {
		t.Fatal("accepted while database unavailable")
	}
}
