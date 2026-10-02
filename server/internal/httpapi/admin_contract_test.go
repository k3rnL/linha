package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"linha/server/internal/auth"
	"linha/server/internal/domain"
	"linha/server/internal/engine"
	"linha/server/internal/storage"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestAdminOpenAPIResponseFixtures(t *testing.T) {
	repo := adminTestStore(t)
	ctx := context.Background()
	root := t.TempDir()
	local, e := storage.NewLocal(root)
	if e != nil {
		t.Fatal(e)
	}
	defer local.Close()
	spec := domain.BackendSpec{Image: "test", Engine: domain.EngineSpec{Type: "fake", Version: "1", Settings: json.RawMessage(`{}`)}, Results: domain.ResultPolicy{Type: "local", Root: root, Path: domain.PathTemplate{Version: 1, Template: "jobs/{requestId}/{attemptId}/{file}"}, MaxBytes: 1024, RetentionSeconds: 3600}}
	c, e := repo.Ensure(ctx, "anonymous", domain.EnsureRequest{Name: "contract", Spec: spec, Hostname: "contract-api-pod"})
	if e != nil {
		t.Fatal(e)
	}
	admin := &auth.Admin{Repo: repo, Config: auth.AdminConfig{Enabled: true, SecurityEnabled: false}}
	handler := (&Server{Repo: repo, Auth: &auth.Disabled{}, Admin: admin, UI: http.NotFoundHandler(), Storage: storage.Registry{Local: local}, Engines: engine.Registry{"fake": engine.NewFake()}}).Handler()
	fixtures := []map[string]any{}
	call := func(method, path, contract string, body any, status int) map[string]any {
		t.Helper()
		r := httptest.NewRequest(method, path, bytes.NewReader(domain.JSON(body)))
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, r)
		if out.Code != status {
			t.Fatalf("%s %s: %d %s", method, path, out.Code, out.Body)
		}
		var value map[string]any
		if e := json.Unmarshal(out.Body.Bytes(), &value); e != nil {
			t.Fatal(e)
		}
		fixtures = append(fixtures, map[string]any{"method": method, "path": contract, "status": status, "body": value})
		return value
	}
	for _, path := range []string{"contexts", "servers", "jobs", "overview"} {
		call("GET", "/v1/admin/"+path, "/v1/admin/"+path, nil, 200)
	}
	call("GET", "/v1/admin/session", "/v1/admin/session", nil, 200)
	call("GET", "/ui/config", "/ui/config", nil, 200)
	call("GET", "/v1/admin/contexts/"+c.ID, "/v1/admin/contexts/{context}", nil, 200)
	for _, path := range []string{"clients", "instances", "workers"} {
		call("GET", "/v1/admin/contexts/"+c.ID+"/"+path, "/v1/admin/contexts/{context}/"+path, nil, 200)
	}
	accepted := call("POST", "/v1/admin/contexts/"+c.ID+"/jobs", "/v1/admin/contexts/{context}/jobs", map[string]any{"handler": "sum", "version": 1, "payload": map[string]int{"n": 2}, "idempotencyKey": "fixture"}, 202)
	job := accepted["id"].(string)
	w := domain.Worker{ID: "contract-worker", ContextID: c.ID, Incarnation: domain.ID(), Capacity: 1, Capabilities: []domain.Capability{{Handler: "sum", Version: 1, Result: domain.ResultDescriptor{Kind: "json", Schema: "sum.result", Version: 1}}}}
	if e = repo.Register(ctx, w); e != nil {
		t.Fatal(e)
	}
	a, e := repo.Claim(ctx, w.ID, w.Incarnation)
	if e != nil || a == nil {
		t.Fatal(a, e)
	}
	u := domain.AttemptUpdate{WorkerID: w.ID, Incarnation: w.Incarnation, AttemptID: a.AttemptID, Fence: a.Fence}
	allocation, e := repo.Allocate(ctx, job, domain.OutputRequest{AttemptUpdate: u, Name: "result.json", Kind: "json", ContentType: "application/json"})
	if e != nil {
		t.Fatal(e)
	}
	f, e := local.Put(ctx, allocation, bytes.NewBufferString(`{"value":2}`))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = repo.Complete(ctx, job, domain.Completion{AttemptUpdate: u, Descriptor: a.ResultDescriptor, Files: []domain.File{f}}); e != nil {
		t.Fatal(e)
	}
	call("GET", "/v1/admin/jobs/"+job, "/v1/admin/jobs/{job}", nil, 200)
	for _, path := range []string{"attempts", "audit", "result", "preview"} {
		call("GET", "/v1/admin/jobs/"+job+"/"+path, "/v1/admin/jobs/{job}/"+path, nil, 200)
	}
	call("POST", "/v1/admin/jobs/"+job+"/cancel", "/v1/admin/jobs/{job}/cancel", map[string]any{}, 200)
	if path := os.Getenv("LINHA_ADMIN_FIXTURE_OUTPUT"); path != "" {
		b, e := json.MarshalIndent(fixtures, "", "  ")
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(path, append(b, '\n'), 0600); e != nil {
			t.Fatal(e)
		}
	}
}
