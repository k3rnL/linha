package postgres

import (
	"context"
	"linha/server/internal/domain"
	"linha/server/internal/engine"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTemplateDefaultsSurviveRestartAndConcurrentEnsure(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	defaults := func(value string) engine.PodTemplates {
		return engine.PodTemplates{Driver: engine.PodTemplate{"metadata": map[string]any{"labels": map[string]any{"default": value}}}}
	}
	k := &engine.Kubernetes{TemplateDefaults: defaults("original")}
	requested := domain.BackendSpec{Image: "image:1", Engine: domain.EngineSpec{Type: "spark", Version: "3.5.6", Settings: domain.JSON(map[string]any{})}}
	var err error
	requested.Engine, err = k.Normalize(requested.Engine)
	if err != nil {
		t.Fatal(err)
	}
	effective := requested
	effective.Engine, err = k.Prepare(requested.Engine)
	if err != nil {
		t.Fatal(err)
	}
	in := domain.EnsureRequest{Name: "templates", Spec: effective, RequestedSpec: &requested}
	first, err := s.Ensure(ctx, "owner", in)
	if err != nil {
		t.Fatal(err)
	}
	// New pool/repository simulates process restart against the same retained schema.
	pool, err := pgxpool.NewWithConfig(ctx, s.Pool.Config())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	restarted := &Store{Pool: pool, Lease: s.Lease}
	if err = restarted.migrate(ctx); err != nil {
		t.Fatal(err)
	}
	k.TemplateDefaults = defaults("new-default")
	changed := requested
	changed.Engine, err = k.Prepare(requested.Engine)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, e := restarted.Ensure(ctx, "owner", domain.EnsureRequest{Name: in.Name, Spec: changed, RequestedSpec: &requested})
			if e != nil || got.ID == first.ID || !strings.Contains(string(got.Spec.Engine.Settings), "new-default") {
				t.Errorf("lost effective snapshot: %s %v", got.Spec.Engine.Settings, e)
			}
		}()
	}
	wg.Wait()
	different := requested
	different.Engine.Settings = domain.JSON(map[string]any{"kubernetes": defaults("client-change")})
	if _, err = s.Ensure(ctx, "owner", domain.EnsureRequest{Name: in.Name, Spec: changed, RequestedSpec: &different}); err != nil {
		t.Fatal(err)
	}
	original, err := s.Context(ctx, "owner", first.ID)
	if err != nil || !strings.Contains(string(original.Spec.Engine.Settings), "original") {
		t.Fatal("old version changed", err)
	}
	if _, err = s.Context(ctx, "other", first.ID); err == nil {
		t.Fatal("foreign templates exposed")
	}
	var raw string
	if err = s.Pool.QueryRow(ctx, "SELECT requested_spec::text FROM linha_contexts WHERE id=$1", first.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "original") {
		t.Fatal("mutable default became client identity")
	}
}
func TestV3TemplateMigrationPreservesLegacyContext(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	legacy := backend(t, s)
	// This schema is isolated and disposable; recreate the preceding migration shape.
	if _, err := s.Pool.Exec(ctx, "ALTER TABLE linha_contexts DROP COLUMN requested_spec; ALTER TABLE linha_contexts DROP COLUMN version; ALTER TABLE linha_contexts ADD UNIQUE(owner,name); DROP TABLE linha_client_leases; ALTER TABLE linha_instances DROP COLUMN resource_kind; ALTER TABLE linha_instances DROP COLUMN resource_uid; ALTER TABLE linha_jobs DROP COLUMN context_name; ALTER TABLE linha_jobs ADD UNIQUE(owner,context_id,idempotency_key); DROP INDEX linha_jobs_active_overview; DROP INDEX linha_jobs_terminal_overview; DROP INDEX linha_attempts_finished_overview; DELETE FROM linha_schema WHERE version>=4"); err != nil {
		t.Fatal(err)
	}
	if err := s.migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	restored, err := s.Ensure(ctx, "owner", domain.EnsureRequest{Name: legacy.Name, Spec: legacy.Spec})
	if err != nil || restored.ID != legacy.ID {
		t.Fatal("legacy identity changed", err)
	}
}

func TestConcurrentDifferentTemplateRequestsCreateVersions(t *testing.T) {
	s := testStore(t)
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, label := range []string{"a", "b"} {
		go func(label string) {
			requested := domain.BackendSpec{Image: "image:1", Engine: domain.EngineSpec{Type: "spark", Version: "3.5.6", Settings: domain.JSON(map[string]any{"kubernetes": map[string]any{"driverPodTemplate": map[string]any{"metadata": map[string]any{"labels": map[string]string{"app": label}}}}})}}
			<-start
			_, err := s.Ensure(context.Background(), "owner", domain.EnsureRequest{Name: "competing-templates", Spec: requested, RequestedSpec: &requested})
			results <- err
		}(label)
	}
	close(start)
	a, b := <-results, <-results
	if a != nil || b != nil {
		t.Fatalf("expected both versions: %v %v", a, b)
	}
}
