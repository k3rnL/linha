package postgres

import (
	"bytes"
	"context"
	"errors"
	"linha/server/internal/domain"
	"linha/server/internal/storage"
	"testing"
	"time"
)

func TestCleanupProtectsActiveAndRetainedResults(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	c := backend(t, s)
	store, err := storage.NewLocal(c.Spec.Results.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	registry := storage.Registry{Local: store}
	job := submit(t, s, c, "cleanup", 1)
	w := worker(t, s, c, "cleanup-worker", 1)
	a, err := s.Claim(ctx, w.ID, w.Incarnation)
	if err != nil {
		t.Fatal(err)
	}
	u := update(a, w)
	allocation, err := s.Allocate(ctx, job.ID, domain.OutputRequest{AttemptUpdate: u, Name: "data", Kind: "json", ContentType: "application/json"})
	if err != nil {
		t.Fatal(err)
	}
	file, err := store.Put(ctx, allocation, bytes.NewBufferString(`{"value":42}`))
	if err != nil {
		t.Fatal(err)
	}
	if n, err := s.CleanupOutputs(ctx, registry, time.Minute, 10); err != nil || n != 0 {
		t.Fatal("active output cleaned", n, err)
	}
	completion := domain.Completion{AttemptUpdate: u, Descriptor: a.ResultDescriptor, Files: []domain.File{file}}
	if _, err = s.Complete(ctx, job.ID, completion); err != nil {
		t.Fatal(err)
	}
	s.Pool.Exec(ctx, "UPDATE linha_attempts SET finished_at=clock_timestamp()-interval '1 day'; UPDATE linha_outputs SET created_at=clock_timestamp()-interval '1 day'")
	if n, err := s.CleanupOutputs(ctx, registry, time.Minute, 10); err != nil || n != 0 {
		t.Fatal("retained result cleaned", n, err)
	}
	s.Pool.Exec(ctx, "UPDATE linha_jobs SET result=jsonb_set(result,'{expiresAt}',to_jsonb(clock_timestamp()-interval '1 hour')) WHERE id=$1", job.ID)
	s.Pool.Exec(ctx, "UPDATE linha_outputs SET write_until=clock_timestamp()+interval '1 hour'")
	if n, err := s.CleanupOutputs(ctx, registry, time.Minute, 10); err != nil || n != 0 {
		t.Fatal("live write grant cleaned", n, err)
	}
	s.Pool.Exec(ctx, "UPDATE linha_outputs SET write_until=clock_timestamp()-interval '1 day'")
	if n, err := s.CleanupOutputs(ctx, registry, time.Minute, 10); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if _, err = store.Open(ctx, allocation); err == nil {
		t.Fatal("expired output remains")
	}
	if _, err = s.Complete(ctx, job.ID, completion); err != nil {
		t.Fatal("completion receipt lost", err)
	}
	if got, err := s.Job(ctx, "owner", job.ID); err != nil || got.State != "SUCCEEDED" {
		t.Fatal("job tombstone lost", err)
	}
	if _, _, err = s.ResultOutput(ctx, "owner", job.ID, file.AllocationID); err == nil {
		t.Fatal("expired result authorized")
	}
}
func TestDatasetPartsFreezeFencingAndPaging(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	c := backend(t, s)
	j := submit(t, s, c, "parts", 2)
	w := worker(t, s, c, "worker", 1)
	a, err := s.Claim(ctx, w.ID, w.Incarnation)
	if err != nil {
		t.Fatal(err)
	}
	u := update(a, w)
	allocation, err := s.Allocate(ctx, j.ID, domain.OutputRequest{AttemptUpdate: u, Name: "dataset", Kind: "dataset", ContentType: "application/x-parquet"})
	if err != nil {
		t.Fatal(err)
	}
	parts := []domain.DatasetPart{{Path: "a.parquet", Size: 3, SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, {Path: "b.parquet", Size: 4, SHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}
	if err = s.RegisterParts(ctx, j.ID, u, allocation.ID, parts); err != nil {
		t.Fatal(err)
	}
	if err = s.RegisterParts(ctx, j.ID, u, allocation.ID, parts); err != nil {
		t.Fatal("lost batch response not idempotent", err)
	}
	first, err := s.DatasetParts(ctx, allocation.ID, "", 1)
	if err != nil || len(first.Items) != 1 || first.NextCursor != "a.parquet" {
		t.Fatal(first, err)
	}
	second, err := s.DatasetParts(ctx, allocation.ID, first.NextCursor, 1)
	if err != nil || second.Items[0].Path != "b.parquet" || second.NextCursor != "" {
		t.Fatal(second, err)
	}
	count, total, err := s.FreezeDataset(ctx, j.ID, u, allocation.ID)
	if err != nil || count != 2 || total != 7 {
		t.Fatal(count, total, err)
	}
	if err = s.RegisterParts(ctx, j.ID, u, allocation.ID, parts); err == nil {
		t.Fatal("mutated sealed part list")
	}
	if _, _, err = s.PublishedDataset(ctx, "other", j.ID); !errors.Is(err, domain.NotFound) {
		t.Fatal("foreign result lookup", err)
	}
	if err = s.Fail(ctx, j.ID, u, domain.Failure{Code: "FAILED", Message: "test", Retryable: true}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.FreezeDataset(ctx, j.ID, u, allocation.ID); !errors.Is(err, domain.Stale) {
		t.Fatal("stale worker froze dataset", err)
	}
}
