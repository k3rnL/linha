package postgres

import (
	"context"
	"errors"
	"linha/server/internal/domain"
	"testing"
)

func TestClientLeasesVersionsAndLogicalIdempotency(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	c := backend(t, s)
	second, err := s.Ensure(ctx, "owner", domain.EnsureRequest{Name: c.Name, Spec: c.Spec, ClientID: "api-b"})
	if err != nil || second.ID != c.ID || second.ClientLease.ID == c.ClientLease.ID {
		t.Fatal(second, err)
	}
	again, err := s.Attach(ctx, "owner", c.ID, "api-b", "")
	if err != nil || again.ClientLease.ID != second.ClientLease.ID {
		t.Fatal(again, err)
	}
	job := submit(t, s, c, "rollout-retry", 1)
	changed := c.Spec
	changed.Image = "image:2"
	next, err := s.Ensure(ctx, "owner", domain.EnsureRequest{Name: c.Name, Spec: changed, ClientID: "api-b"})
	if err != nil || next.ID == c.ID || next.Version == c.Version {
		t.Fatal(next, err)
	}
	request := job.Request
	request.LeaseID = next.ClientLease.ID
	same, err := s.Submit(ctx, "owner", next.ID, request)
	if err != nil || same.ID != job.ID || same.ContextID != c.ID {
		t.Fatal(same, err)
	}
	if err = s.ReleaseClient(ctx, "owner", c.ID, c.ClientLease.ID); err != nil {
		t.Fatal(err)
	}
	request.IdempotencyKey = "after-release"
	request.LeaseID = c.ClientLease.ID
	if _, err = s.Submit(ctx, "owner", c.ID, request); !errors.Is(err, domain.ClientLeaseExpired) {
		t.Fatal("released client accepted work", err)
	}
	request.LeaseID = second.ClientLease.ID
	if _, err = s.Submit(ctx, "owner", c.ID, request); err != nil {
		t.Fatal("live replica cannot submit", err)
	}
	if _, err = s.Attach(ctx, "other", c.ID, "api-b", ""); !errors.Is(err, domain.NotFound) {
		t.Fatal(err)
	}
	if _, err = s.RenewClient(ctx, "other", c.ID, second.ClientLease.ID); !errors.Is(err, domain.NotFound) {
		t.Fatal(err)
	}
	if _, err = s.Pool.Exec(ctx, "UPDATE linha_client_leases SET expires_at=clock_timestamp()-interval '1 second' WHERE context_id=$1", c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RenewClient(ctx, "owner", c.ID, second.ClientLease.ID); !errors.Is(err, domain.ClientLeaseExpired) {
		t.Fatal("expired token revived", err)
	}
	restored, err := s.Attach(ctx, "owner", c.ID, "api-b", "")
	if err != nil || restored.ClientLease.ID == second.ClientLease.ID {
		t.Fatal(restored, err)
	}
	request.IdempotencyKey = "after-reattach"
	request.LeaseID = second.ClientLease.ID
	if _, err = s.Submit(ctx, "owner", c.ID, request); !errors.Is(err, domain.ClientLeaseExpired) {
		t.Fatal("old token accepted", err)
	}
	// The original job remains retrievable independently of client/engine lifetime.
	if got, err := s.Job(ctx, "owner", job.ID); err != nil || got.ID != job.ID {
		t.Fatal(got, err)
	}
}

func TestRetiredInstanceCannotRegisterAfterReattachment(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	c := backend(t, s)
	if _, err := s.Pool.Exec(ctx, "INSERT INTO linha_instances(id,context_id,pod_uid,state) VALUES('retired',$1,'old-worker','DRAINING')", c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Attach(ctx, "owner", c.ID, "new-client", ""); err != nil {
		t.Fatal(err)
	}
	err := s.Register(ctx, domain.Worker{ID: "old-worker", ContextID: c.ID, Incarnation: "old-worker", Capacity: 1, Capabilities: []domain.Capability{{Handler: "sum", Version: 1, Result: domain.ResultDescriptor{Kind: "json", Schema: "sum", Version: 1}}}})
	if err == nil {
		t.Fatal("retired instance registered a fresh worker")
	}
}
