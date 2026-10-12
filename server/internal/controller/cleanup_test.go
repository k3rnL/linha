package controller

import (
	"context"
	"encoding/json"
	"errors"
	"linha/server/internal/domain"
	"linha/server/internal/engine"
	"linha/server/internal/kube"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetiredHistoryDoesNotPollAbsentKubernetesResources(t *testing.T) {
	s, backend := fixture(t)
	ctx := context.Background()
	// Match the production case: all contexts are stopped but their retained
	// intents have Pod and SparkApplication identities from previous versions.
	if _, err := s.Pool.Exec(ctx, `INSERT INTO linha_instances(id,context_id,state,pod_uid,resource_kind,resource_uid)
SELECT md5(n::text),$1,'DEAD','former-pod',CASE WHEN n%2=0 THEN 'pod' ELSE 'sparkapplication' END,'former-resource' FROM generate_series(1,5000) n`, backend.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, "UPDATE linha_contexts SET state='STOPPED' WHERE id=$1", backend.ID); err != nil {
		t.Fatal(err)
	}
	var lists atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || strings.Contains(r.URL.Path, "/linha-") || r.URL.Query().Get("labelSelector") == "" {
			t.Errorf("historical name was polled: %s %s", r.Method, r.URL)
		}
		lists.Add(1)
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer server.Close()
	adapter := &engine.Kubernetes{Client: &kube.Client{URL: server.URL, Namespace: "test", HTTP: server.Client()}}
	c := &Controller{Pool: s.Pool, Adapter: adapter, ID: "replica"}
	for n := 0; n < 100; n++ {
		if err := c.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if lists.Load() != 3 {
		t.Fatalf("5000 retired intents generated %d requests, want 3 collection lists", lists.Load())
	}
	c.nextCleanup = time.Time{}
	if err := c.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if lists.Load() != 6 {
		t.Fatalf("next sweep made %d requests, want 6 total", lists.Load())
	}
	var count int
	if err := s.Pool.QueryRow(ctx, "SELECT count(*) FROM linha_instances WHERE context_id=$1 AND state='DEAD'", backend.ID).Scan(&count); err != nil || count != 5000 {
		t.Fatalf("history changed: %d, %v", count, err)
	}
}

// controlledInventory allows independent listed resource identity, database
// identity, and deletion failures without changing active-instance operations.
type controlledInventory struct {
	*engine.Fake
	resources          []engine.ManagedResource
	listErr, deleteErr error
	listed, deleted    atomic.Int32
	mu                 sync.Mutex
	removed            []engine.ManagedResource
}

func (a *controlledInventory) ListResources(context.Context) ([]engine.ManagedResource, error) {
	a.listed.Add(1)
	return a.resources, a.listErr
}
func (a *controlledInventory) DeleteResource(_ context.Context, r engine.ManagedResource) error {
	a.deleted.Add(1)
	if a.deleteErr != nil {
		return a.deleteErr
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.removed = append(a.removed, r)
	return nil
}

func TestCleanupClaimsOnlyRetiredMatchingIntents(t *testing.T) {
	s, backend := fixture(t)
	ctx := context.Background()
	deadID, activeID := domain.ID(), domain.ID()
	if _, err := s.Pool.Exec(ctx, `INSERT INTO linha_instances(id,context_id,state,resource_kind) VALUES($1,$3,'DEAD','pod'),($2,$3,'STARTING','sparkapplication')`, deadID, activeID, backend.ID); err != nil {
		t.Fatal(err)
	}
	resource := engine.ManagedResource{InstanceID: deadID, ContextID: backend.ID, InstanceKind: "pod", Kind: "services", UID: "late-service"}
	adapter := &controlledInventory{Fake: engine.NewFake(), resources: []engine.ManagedResource{
		resource,
		{InstanceID: activeID, ContextID: backend.ID, InstanceKind: "sparkapplication", Kind: "sparkapplications", UID: "active-uid"},
		{InstanceID: deadID, ContextID: domain.ID(), InstanceKind: "pod", Kind: "pods", UID: "foreign-context"},
		{InstanceID: deadID, ContextID: backend.ID, InstanceKind: "sparkapplication", Kind: "sparkapplications", UID: "wrong-kind"},
		{InstanceID: domain.ID(), ContextID: backend.ID, InstanceKind: "pod", Kind: "pods", UID: "unknown-intent"},
	}}
	first := &Controller{Pool: s.Pool, Adapter: adapter, ID: "first"}
	second := &Controller{Pool: s.Pool, Adapter: adapter, ID: "second"}
	// Competing replicas discover the same orphan. The losing replica cannot
	// delete it; active/unknown/cross-context identities never acquire ownership.
	var wg sync.WaitGroup
	for _, c := range []*Controller{first, second} {
		wg.Add(1)
		go func(c *Controller) {
			defer wg.Done()
			if err := c.cleanup(ctx); err != nil {
				t.Error(err)
			}
		}(c)
	}
	wg.Wait()
	if adapter.deleted.Load() != 1 || len(adapter.removed) != 1 || adapter.removed[0] != resource {
		t.Fatalf("unsafe or duplicate deletion: %+v, calls=%d", adapter.removed, adapter.deleted.Load())
	}
	var owner string
	if err := s.Pool.QueryRow(ctx, "SELECT reconcile_owner FROM linha_contexts WHERE id=$1", backend.ID).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	current, next := first, second
	if owner == "second" {
		current, next = second, first
	}
	if _, err := s.Pool.Exec(ctx, "UPDATE linha_contexts SET reconcile_until=clock_timestamp()-interval '1 second' WHERE id=$1", backend.ID); err != nil {
		t.Fatal(err)
	}
	next.nextCleanup = time.Time{}
	if err := next.cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	current.nextCleanup = time.Time{}
	if err := current.cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	if adapter.deleted.Load() != 2 {
		t.Fatalf("stale controller deleted after ownership moved: %d", adapter.deleted.Load())
	}
}

func TestStoppedContextLateCreateRecoveredAfterControllerRestart(t *testing.T) {
	s, backend := fixture(t)
	ctx := context.Background()
	adapter := engine.NewFake()
	id := domain.ID()
	if _, err := s.Pool.Exec(ctx, "INSERT INTO linha_instances(id,context_id,state,resource_kind) VALUES($1,$2,'DEAD','sparkapplication')", id, backend.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, "UPDATE linha_contexts SET state='STOPPED' WHERE id=$1", backend.ID); err != nil {
		t.Fatal(err)
	}
	first := &Controller{Pool: s.Pool, Adapter: adapter, ID: "before-restart"}
	if err := first.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	late := engine.Instance{ID: id, ContextID: backend.ID, Kind: "sparkapplication"}
	if err := adapter.Ensure(ctx, backend, late); err != nil {
		t.Fatal(err)
	}
	// A fresh controller has no cleanup cache or active-context reconciliation.
	restarted := &Controller{Pool: s.Pool, Adapter: adapter, ID: "after-restart"}
	if err := restarted.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Observe(ctx, late); !errors.Is(err, domain.NotFound) {
		t.Fatalf("late create leaked: %v", err)
	}
	var state string
	if err := s.Pool.QueryRow(ctx, "SELECT state FROM linha_contexts WHERE id=$1", backend.ID).Scan(&state); err != nil || state != "STOPPED" {
		t.Fatalf("history reactivated: %s, %v", state, err)
	}
}

func TestCleanupFailureIsBoundedAndDoesNotBlockCapacity(t *testing.T) {
	s, backend := fixture(t)
	ctx := context.Background()
	adapter := &controlledInventory{Fake: engine.NewFake(), listErr: errors.New("inventory unavailable")}
	c := &Controller{Pool: s.Pool, Adapter: adapter, ID: "replica", Resolve: func(context.Context, domain.BackendSpec) (string, error) { return "image@sha256:fixed", nil }}
	if err := c.Tick(ctx); err == nil {
		t.Fatal("inventory failure was hidden")
	}
	for n := 0; n < 3; n++ {
		if err := c.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	instances, err := c.instances(ctx, backend.ID, "")
	if err != nil || len(instances) != 1 {
		t.Fatalf("capacity failed after inventory error: %+v, %v", instances, err)
	}
	if _, err := adapter.Observe(ctx, instances[0].instance); err != nil {
		t.Fatal("driver never started", err)
	}
	var condition string
	if err := s.Pool.QueryRow(ctx, "SELECT condition FROM linha_contexts WHERE id=$1", backend.ID).Scan(&condition); err != nil || condition != "" {
		t.Fatalf("cleanup contaminated provisioning: %s, %v", condition, err)
	}
	if adapter.listed.Load() != 1 {
		t.Fatalf("inventory failure caused retry storm: %d", adapter.listed.Load())
	}
	id := domain.ID()
	if _, err := s.Pool.Exec(ctx, "INSERT INTO linha_instances(id,context_id,state,resource_kind) VALUES($1,$2,'DEAD','pod')", id, backend.ID); err != nil {
		t.Fatal(err)
	}
	adapter.resources = []engine.ManagedResource{{InstanceID: id, ContextID: backend.ID, InstanceKind: "pod", Kind: "services", UID: "service"}}
	adapter.listErr = nil
	adapter.deleteErr = errors.New("delete unavailable")
	c.nextCleanup = time.Time{}
	if err := c.Tick(ctx); err == nil {
		t.Fatal("deletion failure was hidden")
	}
	if err := c.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if adapter.listed.Load() != 2 || adapter.deleted.Load() != 1 {
		t.Fatal("deletion failure caused retry storm")
	}
	adapter.deleteErr = nil
	c.nextCleanup = time.Time{}
	if err := c.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if len(adapter.removed) != 1 {
		t.Fatal("cleanup did not recover")
	}
}

func TestCleanupDeletesLegacyServiceWithoutAbsentPodProbe(t *testing.T) {
	s, backend := fixture(t)
	ctx := context.Background()
	id := domain.ID()
	if _, err := s.Pool.Exec(ctx, "INSERT INTO linha_instances(id,context_id,state,resource_kind) VALUES($1,$2,'DEAD','pod')", id, backend.ID); err != nil {
		t.Fatal(err)
	}
	var deleted atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v1/namespaces/test/services/linha-"+id {
			var body struct{ Preconditions struct{ UID string } }
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Preconditions.UID != "service-only" {
				t.Errorf("UID missing: %+v %v", body, err)
			}
			deleted.Add(1)
			return
		}
		if r.Method != http.MethodGet || strings.Contains(r.URL.Path, "/linha-") {
			t.Errorf("unexpected absent-resource probe: %s %s", r.Method, r.URL)
		}
		if strings.HasSuffix(r.URL.Path, "/services") {
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{map[string]any{"metadata": map[string]any{
				"name": "linha-" + id, "namespace": "test", "uid": "service-only", "labels": map[string]string{"app.kubernetes.io/managed-by": "linha", "linha.io/context": backend.ID, "linha.io/instance": id},
			}}}})
			return
		}
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer server.Close()
	adapter := &engine.Kubernetes{Client: &kube.Client{URL: server.URL, Namespace: "test", HTTP: server.Client()}}
	c := &Controller{Pool: s.Pool, Adapter: adapter, ID: "replica"}
	if err := c.cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	if deleted.Load() != 1 {
		t.Fatal("Service leaked when Pod was absent")
	}
}
