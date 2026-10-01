package postgres

import (
	"context"
	"errors"
	"linha/server/internal/domain"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFailureDiagnosticsSurviveRetryRestartAndFencing(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	c := backend(t, s)
	j := submit(t, s, c, "diagnostics", 2)
	w := worker(t, s, c, "worker", 1)
	first, err := s.Claim(ctx, w.ID, w.Incarnation)
	if err != nil || first == nil {
		t.Fatal(first, err)
	}
	original := domain.Failure{Code: "HANDLER_FAILED", Message: "outer", Retryable: true, ExceptionType: "java.lang.IllegalStateException", Phase: "handler", StackTrace: "java.lang.IllegalStateException: outer\n\tat example.Job.run(Job.scala:12)\nSuppressed: java.io.IOException: cleanup\nCaused by: java.lang.IllegalArgumentException: inner"}
	if err = s.Fail(ctx, j.ID, update(first, w), original); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Pool.Exec(ctx, "UPDATE linha_jobs SET available_at=clock_timestamp() WHERE id=$1", j.ID); err != nil {
		t.Fatal(err)
	}
	second, err := s.Claim(ctx, w.ID, w.Incarnation)
	if err != nil || second == nil {
		t.Fatal(second, err)
	}
	current := original
	current.Retryable = false
	current.Message = "final failure"
	if err = s.Fail(ctx, j.ID, update(second, w), current); err != nil {
		t.Fatal(err)
	}
	if err = s.Fail(ctx, j.ID, update(first, w), original); !errors.Is(err, domain.Stale) {
		t.Fatal("stale diagnostics accepted", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, s.Pool.Config())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	restarted := &Store{Pool: pool, Lease: s.Lease}
	got, err := restarted.Job(ctx, "owner", j.ID)
	if err != nil || got.Failure == nil || *got.Failure != current {
		t.Fatal(got, err)
	}
	attempts, err := restarted.Attempts(ctx, "owner", j.ID)
	if err != nil || len(attempts) != 2 || attempts[0].Failure == nil || *attempts[0].Failure != original || *attempts[1].Failure != current {
		t.Fatal(attempts, err)
	}
	if _, err = restarted.Job(ctx, "other", j.ID); !errors.Is(err, domain.NotFound) {
		t.Fatal(err)
	}
	if _, err = restarted.Attempts(ctx, "other", j.ID); !errors.Is(err, domain.NotFound) {
		t.Fatal(err)
	}
}
func TestDeadlineFinalizationPreservesObservedAttemptException(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	c := backend(t, s)
	j := submit(t, s, c, "deadline-diagnostic", 1)
	w := worker(t, s, c, "worker", 1)
	a, err := s.Claim(ctx, w.ID, w.Incarnation)
	if err != nil || a == nil {
		t.Fatal(a, err)
	}
	failure := domain.Failure{Code: "HANDLER_FAILED", Message: "interrupted IO", ExceptionType: "java.io.IOException", StackTrace: "example.Worker.run(Worker.scala:10)", Phase: "handler"}
	// Model deadline expiry between the ownership check and finalization.
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if e := owned(ctx, tx, j.ID, update(a, w), true); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, "UPDATE linha_jobs SET deadline=clock_timestamp()-interval '1 second' WHERE id=$1", j.ID); e != nil {
			return e
		}
		return finishFailure(ctx, tx, j.ID, a.AttemptID, "FAILED", failure)
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Job(ctx, "owner", j.ID)
	if err != nil || got.Failure.Code != "DEADLINE_EXCEEDED" {
		t.Fatal(got, err)
	}
	attempts, err := s.Attempts(ctx, "owner", j.ID)
	if err != nil || len(attempts) != 1 || *attempts[0].Failure != failure {
		t.Fatal(attempts, err)
	}
}
