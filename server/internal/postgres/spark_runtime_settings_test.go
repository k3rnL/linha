package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"linha/server/internal/domain"
	"linha/server/internal/engine"
	"linha/server/internal/kube"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSparkRuntimeSettingsPersistAcrossConcurrentEnsureAndRestart(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	k := &engine.Kubernetes{}
	prepare := func(timeout string) domain.BackendSpec {
		t.Helper()
		spec, err := k.Prepare(domain.EngineSpec{Type: "spark", Version: "3.5.6", Settings: json.RawMessage(fmt.Sprintf(`{"application":{"mainClass":"example.Main","mainApplicationFile":"local:///opt/job.jar"},"ui":{"enabled":true,"port":4050},"executors":{"dynamicAllocation":true,"executorIdleTimeout":"90s","cachedExecutorIdleTimeout":%q,"shuffleTrackingTimeout":%q}}`, timeout, timeout))})
		if err != nil {
			t.Fatal(err)
		}
		return domain.BackendSpec{Image: "image@sha256:test", Engine: spec}
	}
	first, err := s.Ensure(ctx, "owner", domain.EnsureRequest{Name: "spark-settings", Spec: prepare("2m"), ClientID: "first"})
	if err != nil {
		t.Fatal(err)
	}
	// A second repository/connection pool models another replica or server restart.
	pool, err := pgxpool.NewWithConfig(ctx, s.Pool.Config())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	restarted := &Store{Pool: pool, Lease: s.Lease}
	type result struct {
		c   domain.BackendContext
		err error
	}
	results := make(chan result, 8)
	for i := 0; i < 8; i++ {
		duration := "2m"
		if i%2 == 0 {
			duration = "120s"
		}
		spec := prepare(duration)
		go func() {
			c, e := restarted.Ensure(ctx, "owner", domain.EnsureRequest{Name: first.Name, Spec: spec, ClientID: domain.ID()})
			results <- result{c, e}
		}()
	}
	for i := 0; i < 8; i++ {
		r := <-results
		if r.err != nil || r.c.ID != first.ID || r.c.Version != first.Version {
			t.Fatal(r)
		}
	}
	restored, err := restarted.Context(ctx, "owner", first.ID)
	if err != nil {
		t.Fatal(err)
	}
	var submitted map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&submitted); err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer api.Close()
	// A replacement gets the persisted effective spec, with no client resubmission.
	replacement := &engine.Kubernetes{Client: &kube.Client{URL: api.URL, HTTP: api.Client(), Namespace: "jobs"}, ServerURL: "http://linha", ServiceAccount: "worker"}
	if err = replacement.Ensure(ctx, restored, engine.Instance{ID: "replacement", Image: restored.Spec.Image}); err != nil {
		t.Fatal(err)
	}
	conf := submitted["spec"].(map[string]any)["sparkConf"].(map[string]any)
	for key, want := range map[string]string{"spark.ui.enabled": "true", "spark.ui.port": "4050", "spark.dynamicAllocation.executorIdleTimeout": "90s", "spark.dynamicAllocation.cachedExecutorIdleTimeout": "120s", "spark.dynamicAllocation.shuffleTracking.timeout": "120s"} {
		if conf[key] != want {
			t.Errorf("restored %s = %v, want %s", key, conf[key], want)
		}
	}
	changed, err := restarted.Ensure(ctx, "owner", domain.EnsureRequest{Name: first.Name, Spec: prepare("3m"), ClientID: "changed"})
	if err != nil || changed.ID == first.ID || changed.Version == first.Version {
		t.Fatal(changed, err)
	}
	if _, err = restarted.Context(ctx, "other", first.ID); !errors.Is(err, domain.NotFound) {
		t.Fatal("foreign context exposed", err)
	}
}
