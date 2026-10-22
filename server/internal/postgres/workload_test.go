package postgres

import (
	"context"
	"errors"
	"linha/server/internal/domain"
	"testing"
)

func applicationContext(t *testing.T, s *Store) domain.BackendContext {
	t.Helper()
	c := backend(t, s)
	c.Spec.Storage = &domain.WorkloadStoragePolicy{Destinations: []string{"data"}, Fingerprints: map[string]string{"data": "physical"}}
	out, e := s.Ensure(context.Background(), "owner", domain.EnsureRequest{Name: "application", Spec: c.Spec})
	if e != nil {
		t.Fatal(e)
	}
	return out
}
func applicationSession(t *testing.T, s *Store, c domain.BackendContext) domain.StorageSession {
	t.Helper()
	out, e := s.CreateStorageSession(context.Background(), domain.StorageSession{ContextID: c.ID, Owner: c.Owner, ClientLease: c.ClientLease.ID, Destination: "data", Fingerprint: "physical"})
	if e != nil {
		t.Fatal(e)
	}
	return out
}
func TestApplicationSessionHAOwnerDestinationAndExpiry(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	c := applicationContext(t, s)
	a := applicationSession(t, s, c)
	replica := &Store{Pool: s.Pool, Lease: s.Lease}
	if _, e := replica.StorageSession(ctx, a.ID, a.Token); e != nil {
		t.Fatal(e)
	}
	if _, e := replica.StorageSession(ctx, a.ID, "wrong"); e == nil {
		t.Fatal("accepted invalid token")
	}
	for _, in := range []domain.StorageSession{
		{ContextID: c.ID, Owner: "other", ClientLease: c.ClientLease.ID, Destination: "data", Fingerprint: "physical"},
		{ContextID: c.ID, Owner: c.Owner, ClientLease: c.ClientLease.ID, Destination: "different", Fingerprint: "physical"},
		{ContextID: c.ID, Owner: c.Owner, ClientLease: c.ClientLease.ID, Destination: "data", Fingerprint: "changed"},
	} {
		if _, e := s.CreateStorageSession(ctx, in); e == nil {
			t.Fatal("accepted unauthorized destination/owner")
		}
	}
	if e := s.ReleaseClient(ctx, c.Owner, c.ID, c.ClientLease.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := replica.StorageSession(ctx, a.ID, a.Token); !errors.Is(e, domain.Stale) {
		t.Fatalf("expired session: %v", e)
	}
}
func TestApplicationLocksSharePhysicalKeyAndFenceExpiredWriters(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	c := applicationContext(t, s)
	a := applicationSession(t, s, c)
	b := applicationSession(t, s, c)
	key := "physical:dataset/index"
	if e := s.AcquireStorageLock(ctx, a, key); e != nil {
		t.Fatal(e)
	}
	if e := s.AcquireStorageLock(ctx, b, key); e == nil {
		t.Fatal("concurrent writer acquired lock")
	}
	written := false
	if e := s.ProtectStorageWrite(ctx, a, key, func() error { written = true; return nil }); e != nil || !written {
		t.Fatal(e)
	}
	if _, e := s.Pool.Exec(ctx, "UPDATE linha_storage_locks SET lease_expires_at='-infinity' WHERE physical_key=$1", key); e != nil {
		t.Fatal(e)
	}
	if e := s.RenewStorageLock(ctx, a, key); !errors.Is(e, domain.Stale) {
		t.Fatalf("renew expired: %v", e)
	}
	if e := s.AcquireStorageLock(ctx, b, key); e != nil {
		t.Fatal(e)
	}
	written = false
	if e := s.ProtectStorageWrite(ctx, a, key, func() error { written = true; return nil }); !errors.Is(e, domain.Stale) || written {
		t.Fatalf("stale write: %v / %v", e, written)
	}
}
func TestStatusResultAndWorkerStorageAttemptFencing(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	c := applicationContext(t, s)
	descriptor := domain.ResultDescriptor{Kind: "status", Schema: "ingest.result", Version: 1}
	w := domain.Worker{ID: "application-worker", ContextID: c.ID, Incarnation: domain.ID(), Capacity: 1, Capabilities: []domain.Capability{{Handler: "sum", Version: 1, Result: descriptor}}}
	if e := s.Register(ctx, w); e != nil {
		t.Fatal(e)
	}
	j := submit(t, s, c, "application-file", 1)
	a, e := s.Claim(ctx, w.ID, w.Incarnation)
	if e != nil || a == nil {
		t.Fatal(e)
	}
	grant, e := s.CreateStorageSession(ctx, domain.StorageSession{ContextID: c.ID, JobID: j.ID, Identity: update(a, w), Destination: "data", Fingerprint: "physical"})
	if e != nil {
		t.Fatal(e)
	}
	bad := update(a, w)
	bad.Fence++
	if _, e := s.CreateStorageSession(ctx, domain.StorageSession{ContextID: c.ID, JobID: j.ID, Identity: bad, Destination: "data", Fingerprint: "physical"}); e == nil {
		t.Fatal("stale worker acquired storage")
	}
	completion := domain.Completion{AttemptUpdate: update(a, w), Descriptor: descriptor, Files: []domain.File{}}
	result, e := s.Complete(ctx, j.ID, completion)
	if e != nil {
		t.Fatal(e)
	}
	if len(result.Files) != 0 || result.Dataset != nil {
		t.Fatal("status result has artifacts")
	}
	if _, e := s.Complete(ctx, j.ID, completion); e != nil {
		t.Fatal("receipt replay", e)
	}
	if _, e := s.StorageSession(ctx, grant.ID, grant.Token); !errors.Is(e, domain.Stale) {
		t.Fatalf("completed attempt still accesses storage: %v", e)
	}
}
