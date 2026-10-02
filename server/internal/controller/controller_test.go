package controller

import (
	"context"
	"encoding/json"
	"errors"
	"linha/server/internal/domain"
	"linha/server/internal/engine"
	"linha/server/internal/postgres"
	"net/url"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func fixture(t *testing.T) (*postgres.Store, domain.BackendContext) {
	t.Helper()
	dsn := os.Getenv("LINHA_TEST_DATABASE")
	if dsn == "" {
		t.Skip("LINHA_TEST_DATABASE required")
	}
	ctx := context.Background()
	admin, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	schema := "controller_" + domain.ID()
	if _, e = admin.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	parsed, e := url.Parse(dsn)
	if e != nil {
		t.Fatal(e)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	s, e := postgres.Open(ctx, parsed.String())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close(); admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); admin.Close() })
	c, e := s.Ensure(ctx, "owner", domain.EnsureRequest{Name: "spark", Spec: domain.BackendSpec{Image: "image:stable", Engine: domain.EngineSpec{Type: "spark", Version: "3.5.6", Settings: json.RawMessage(`{"drivers":{"minDrivers":1,"maxDrivers":2,"cooldownSeconds":1,"idleSeconds":1}}`)}}})
	if e != nil {
		t.Fatal(e)
	}
	return s, c
}

type ambiguousAdapter struct {
	*engine.Fake
	lost bool
}

func (a *ambiguousAdapter) Ensure(ctx context.Context, c domain.BackendContext, i engine.Instance) error {
	if e := a.Fake.Ensure(ctx, c, i); e != nil {
		return e
	}
	if !a.lost {
		a.lost = true
		return errors.New("response lost after creating driver")
	}
	return nil
}
func TestFailoverReplacementAndStaleController(t *testing.T) {
	s, c := fixture(t)
	ctx := context.Background()
	adapter := &ambiguousAdapter{Fake: engine.NewFake()}
	resolveCalls := 0
	first := &Controller{Pool: s.Pool, Adapter: adapter, ID: "first", Resolve: func(context.Context, domain.BackendSpec) (string, error) {
		resolveCalls++
		return "image@sha256:fixed", nil
	}}
	second := &Controller{Pool: s.Pool, Adapter: adapter, ID: "second", Resolve: first.Resolve}
	// Competing replicas create one durable intent, even before any pod exists.
	var wg sync.WaitGroup
	for _, controller := range []*Controller{first, second} {
		wg.Add(1)
		go func(c *Controller) {
			defer wg.Done()
			if e := c.Tick(ctx); e != nil {
				t.Error(e)
			}
		}(controller)
	}
	wg.Wait()
	var owner string
	var epoch int64
	if e := s.Pool.QueryRow(ctx, "SELECT reconcile_owner,reconcile_epoch FROM linha_contexts WHERE id=$1", c.ID).Scan(&owner, &epoch); e != nil {
		t.Fatal(e)
	}
	current, next := first, second
	if owner == "second" {
		current, next = second, first
	}
	if e := current.Tick(ctx); e != nil {
		t.Fatal(e)
	} // ambiguous create is reported as a backend condition
	records, e := current.instances(ctx, c.ID, "image@sha256:fixed")
	if e != nil || len(records) != 1 {
		t.Fatalf("intents %v %v", records, e)
	}
	original := records[0].instance
	observed, e := adapter.Observe(ctx, original)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Pool.Exec(ctx, "UPDATE linha_contexts SET reconcile_until=clock_timestamp()-interval '1 second',reconcile_after=clock_timestamp()-interval '1 second' WHERE id=$1", c.ID); e != nil {
		t.Fatal(e)
	}
	if e = next.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	records, e = next.instances(ctx, c.ID, "image@sha256:fixed")
	if e != nil || len(records) != 1 || records[0].instance.Incarnation != observed.Incarnation {
		t.Fatalf("failover duplicated driver: %+v %v", records, e)
	}
	if _, e = current.fenced(ctx, c.ID, epoch, "UPDATE linha_instances SET state='DEAD' WHERE context_id=$1", c.ID); e == nil {
		t.Fatal("stale controller changed desired state")
	}
	if resolveCalls != 1 {
		t.Fatalf("pinned image was re-resolved %d times", resolveCalls)
	}
	// Driver loss is repaired without a connected client or a retryable request.
	if e = adapter.Stop(ctx, observed); e != nil {
		t.Fatal(e)
	}
	if e = next.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	if e = next.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	records, e = next.instances(ctx, c.ID, "image@sha256:fixed")
	if e != nil || len(records) != 1 || records[0].instance.ID == original.ID {
		t.Fatalf("replacement missing: %+v %v", records, e)
	}
	// A late create for a retired intent is cleaned, not counted as usable capacity.
	if e = adapter.Fake.Ensure(ctx, c, original); e != nil {
		t.Fatal(e)
	}
	if e = next.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = adapter.Observe(ctx, original); !errors.Is(e, domain.NotFound) {
		t.Fatalf("orphan remained: %v", e)
	}
}

func TestLastClientDrainsAcceptedWorkBeforeStoppingAndReattaches(t *testing.T) {
	s, backend := fixture(t)
	ctx := context.Background()
	adapter := engine.NewFake()
	c := &Controller{Pool: s.Pool, Adapter: adapter, ID: "leases", Resolve: func(context.Context, domain.BackendSpec) (string, error) { return "image@sha256:test", nil }}
	tick := func() {
		t.Helper()
		if e := c.Tick(ctx); e != nil {
			t.Fatal(e)
		}
		if e := s.Recover(ctx); e != nil {
			t.Fatal(e)
		}
		var condition string
		if e := s.Pool.QueryRow(ctx, "SELECT condition FROM linha_contexts WHERE id=$1", backend.ID).Scan(&condition); e != nil || condition != "" {
			t.Fatal(condition, e)
		}
	}
	second, e := s.Attach(ctx, "owner", backend.ID, "second", "")
	if e != nil {
		t.Fatal(e)
	}
	tick()
	tick()
	if e = s.ReleaseClient(ctx, "owner", backend.ID, backend.ClientLease.ID); e != nil {
		t.Fatal(e)
	}
	tick()
	rs, e := c.instances(ctx, backend.ID, "")
	if e != nil || len(rs) != 1 {
		t.Fatal(rs, e)
	}
	job, e := s.Submit(ctx, "owner", backend.ID, domain.SubmitRequest{LeaseID: second.ClientLease.ID, Handler: "sum", Version: 1, Payload: json.RawMessage(`{}`), IdempotencyKey: "accepted"})
	if e != nil {
		t.Fatal(e)
	}
	// Simulate abrupt client death without close.
	if _, e = s.Pool.Exec(ctx, "UPDATE linha_client_leases SET expires_at=clock_timestamp()-interval '1 second' WHERE context_id=$1", backend.ID); e != nil {
		t.Fatal(e)
	}
	tick()
	state, e := s.Context(ctx, "owner", backend.ID)
	if e != nil || state.State != "DRAINING" {
		t.Fatal(state, e)
	}
	rs, e = c.instances(ctx, backend.ID, "")
	if e != nil || len(rs) != 1 || rs[0].state == "DRAINING" {
		t.Fatal("accepted queue lost capacity", rs, e)
	}
	// A lost driver still gets replacement capacity with zero live clients.
	if e = adapter.Stop(ctx, rs[0].instance); e != nil {
		t.Fatal(e)
	}
	tick()
	tick()
	tick()
	rs, e = c.instances(ctx, backend.ID, "")
	if e != nil || len(rs) != 1 {
		t.Fatal("no replacement", rs, e)
	}
	// Terminal accepted work permits shutdown even with minDrivers=1 and STARTING instances.
	if _, e = s.Cancel(ctx, "owner", job.ID); e != nil {
		t.Fatal(e)
	}
	tick()
	tick()
	tick()
	state, e = s.Context(ctx, "owner", backend.ID)
	if e != nil || state.State != "STOPPED" {
		t.Fatal(state, e)
	}
	if j, e := s.Job(ctx, "owner", job.ID); e != nil || j.State != "CANCELLED" {
		t.Fatal(j, e)
	}
	attached, e := s.Attach(ctx, "owner", backend.ID, "third", "")
	if e != nil || attached.ID != backend.ID {
		t.Fatal(attached, e)
	}
	tick()
	tick()
	rs, e = c.instances(ctx, backend.ID, "")
	if e != nil || len(rs) != 1 {
		t.Fatal("reattach did not restore minimum", rs, e)
	}
}

type identityChangeAdapter struct {
	*engine.Fake
	changed bool
}

func (a *identityChangeAdapter) Observe(ctx context.Context, i engine.Instance) (engine.Instance, error) {
	if a.changed {
		return i, domain.Conflict("application UID changed")
	}
	out, e := a.Fake.Observe(ctx, i)
	if e != nil {
		return out, e
	}
	out.ResourceUID = "current-application"
	out.Observation = &domain.EngineObservation{Available: true, Links: []domain.EngineLink{{Name: "spark-ui", URL: "https://current.example"}}}
	return out, nil
}
func TestRejectedResourceIdentityImmediatelySuppressesLinks(t *testing.T) {
	s, backend := fixture(t)
	ctx := context.Background()
	adapter := &identityChangeAdapter{Fake: engine.NewFake()}
	c := &Controller{Pool: s.Pool, Adapter: adapter, ID: "observer", Resolve: func(context.Context, domain.BackendSpec) (string, error) { return "image@sha256:fixed", nil }}
	for n := 0; n < 3; n++ {
		if e := c.Tick(ctx); e != nil {
			t.Fatal(e)
		}
	}
	before, e := s.OperationalInstances(ctx, "owner", backend.ID, domain.OperationalFilter{})
	if e != nil || len(before.Items) != 1 || !before.Items[0].Available {
		t.Fatal(before, e)
	}
	adapter.changed = true
	if e = c.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	after, e := s.OperationalInstances(ctx, "owner", backend.ID, domain.OperationalFilter{})
	if e != nil || after.Items[0].Available || !after.Items[0].ObservedAt.Equal(*before.Items[0].ObservedAt) {
		t.Fatal(after, e)
	}
	var detail domain.EngineObservation
	if e = json.Unmarshal(after.Items[0].Detail, &detail); e != nil || len(detail.Links) != 0 || detail.Condition != "resource_identity_changed" {
		t.Fatal(detail, e)
	}
	var epoch int64
	if e = s.Pool.QueryRow(ctx, "SELECT reconcile_epoch FROM linha_contexts WHERE id=$1", backend.ID).Scan(&epoch); e != nil {
		t.Fatal(e)
	}
	stale := engine.Instance{ID: after.Items[0].ID, ContextID: backend.ID, ResourceUID: "previous-application", Incarnation: after.Items[0].Incarnation}
	if e = c.invalidateObservation(ctx, backend.ID, epoch, stale); !errors.Is(e, domain.Stale) {
		t.Fatal("stale UID invalidation", e)
	}
}
