package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"linha/server/internal/auth"
	"linha/server/internal/domain"
	"linha/server/internal/engine"
	"linha/server/internal/postgres"
	"linha/server/internal/storage"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

type principals struct{}

func (principals) Owner(_ context.Context, r *http.Request) (string, error) {
	switch auth.Bearer(r) {
	case "A", "B":
		return auth.Bearer(r), nil
	}
	return "", auth.Unauthorized
}
func (principals) Worker(context.Context, *http.Request) (auth.WorkerIdentity, error) {
	return auth.WorkerIdentity{}, auth.Unauthorized
}
func TestPublicRoutesEnforceOwnershipBeforeResourceLookup(t *testing.T) {
	dsn := os.Getenv("LINHA_TEST_DATABASE")
	if dsn == "" {
		t.Skip("LINHA_TEST_DATABASE required")
	}
	ctx := context.Background()
	admin, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer admin.Close()
	schema := "http_" + domain.ID()
	if _, e = admin.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	parsed, e := url.Parse(dsn)
	if e != nil {
		t.Fatal(e)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	repo, e := postgres.Open(ctx, parsed.String())
	if e != nil {
		t.Fatal(e)
	}
	defer repo.Close()
	root := t.TempDir()
	local, e := storage.NewLocal(root)
	if e != nil {
		t.Fatal(e)
	}
	defer local.Close()
	server := httptest.NewServer((&Server{Repo: repo, Auth: principals{}, Storage: storage.Registry{Local: local}, Engines: engine.Registry{"fake": engine.NewFake()}}).Handler())
	defer server.Close()
	leases := map[string]string{}
	call := func(token, method, path string, body any, want int) map[string]any {
		t.Helper()
		var data []byte
		if body != nil {
			data = domain.JSON(body)
		}
		r, e := http.NewRequest(method, server.URL+path, bytes.NewReader(data))
		if e != nil {
			t.Fatal(e)
		}
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Linha-Client-Lease", leases[token])
		response, e := server.Client().Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(response.Body).Decode(&out)
		if response.StatusCode != want {
			t.Fatalf("%s %s: %d want %d: %+v", method, path, response.StatusCode, want, out)
		}
		if lease, ok := out["clientLease"].(map[string]any); ok {
			leases[token] = lease["id"].(string)
		}
		return out
	}
	spec := map[string]any{"name": "example", "spec": map[string]any{"image": "example:test", "engine": map[string]any{"type": "fake", "version": "1", "settings": map[string]any{}}, "results": map[string]any{"type": "local", "root": root}}}
	c := call("A", "POST", "/v1/contexts:ensure", spec, 200)["id"].(string)
	other := call("B", "POST", "/v1/contexts:ensure", spec, 200)["id"].(string)
	if c == other {
		t.Fatal("context ownership collapsed")
	}
	input := map[string]any{"handler": "handler", "version": 1, "payload": map[string]any{}, "idempotencyKey": "key"}
	j := call("A", "POST", "/v1/contexts/"+c+"/jobs", input, 202)["id"].(string)
	if again := call("A", "POST", "/v1/contexts/"+c+"/jobs", input, 202)["id"]; again != j {
		t.Fatal("response replay changed ID")
	}
	for _, path := range []string{"/v1/contexts/" + c, "/v1/jobs/" + j, "/v1/jobs/" + j + "/result", "/v1/jobs/" + j + "/files/unknown", "/v1/jobs/" + j + "/attempts", "/v1/jobs/" + j + "/parts", "/v1/jobs/" + j + "/part?path=part.parquet"} {
		call("B", "GET", path, nil, 404)
		call("", "GET", path, nil, 401)
	}
	call("B", "POST", "/v1/jobs/"+j+":cancel", nil, 404)
	call("B", "POST", "/v1/contexts/"+c+"/jobs", input, 404)
	page := call("B", "GET", "/v1/jobs?contextId="+c, nil, 200)
	if len(page["items"].([]any)) != 0 {
		t.Fatal("cross-owner listing")
	}
	forged := map[string]any{"owner": "B", "handler": "handler", "version": 1, "payload": map[string]any{}, "idempotencyKey": "forged"}
	call("A", "POST", "/v1/contexts/"+c+"/jobs", forged, 400)
	call("A", "POST", "/v1/workers/register", map[string]any{}, 401)
	response, e := server.Client().Get(server.URL + "/metrics")
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("metrics query failed")
	}
}
