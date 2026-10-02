package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
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
	"time"
)

func adminTestStore(t *testing.T) *postgres.Store {
	t.Helper()
	dsn := os.Getenv("LINHA_TEST_DATABASE")
	if dsn == "" {
		t.Skip("LINHA_TEST_DATABASE required")
	}
	ctx := context.Background()
	p, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	schema := "admin_http_" + domain.ID()
	if _, e = p.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	config, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		t.Fatal(e)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
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
	t.Cleanup(func() { s.Close(); p.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); p.Close() })
	return s
}
func TestAdminHTTPAuthorizationOwnerIsolationAndReplay(t *testing.T) {
	repo := adminTestStore(t)
	root := t.TempDir()
	local, e := storage.NewLocal(root)
	if e != nil {
		t.Fatal(e)
	}
	defer local.Close()
	ctx := context.Background()
	spec := domain.BackendSpec{Image: "test", Engine: domain.EngineSpec{Type: "fake", Version: "1", Settings: json.RawMessage(`{}`)}, Results: domain.ResultPolicy{Type: "local", Root: root, Path: domain.PathTemplate{Version: 1, Template: "jobs/{requestId}/{attemptId}/{file}"}, MaxBytes: 1024, RetentionSeconds: 3600}}
	c, e := repo.Ensure(ctx, "B", domain.EnsureRequest{Name: "private", Spec: spec})
	if e != nil {
		t.Fatal(e)
	}
	issuer := "https://issuer.example"
	administrator := &auth.Admin{Repo: repo, Origin: "https://console.example", Config: auth.AdminConfig{SecurityEnabled: true, Issuer: issuer, Mappings: auth.AdminMappings{Viewer: auth.RoleRules{Subjects: []auth.SubjectRule{{Issuer: issuer, Subject: "viewer"}}}, Operator: auth.RoleRules{Subjects: []auth.SubjectRule{{Issuer: issuer, Subject: "operator"}}}}}}
	cookies := map[string]string{}
	for _, subject := range []string{"viewer", "operator", "ordinary"} {
		value := base64.RawURLEncoding.EncodeToString([]byte(domain.ID()))
		hash := sha256.Sum256([]byte(value))
		cookies[subject] = value
		if e = repo.SaveSession(ctx, domain.BrowserSession{Hash: hex.EncodeToString(hash[:]), CSRF: "csrf", Claims: domain.JSON(map[string]any{"iss": issuer, "sub": subject}), ExpiresAt: time.Now().Add(time.Minute)}); e != nil {
			t.Fatal(e)
		}
	}
	handler := (&Server{Repo: repo, Auth: principals{}, Admin: administrator, UI: http.NotFoundHandler(), Storage: storage.Registry{Local: local}, Engines: engine.Registry{"fake": engine.NewFake()}}).Handler()
	call := func(subject, method, path string, body any, origin, csrf string, want int) map[string]any {
		t.Helper()
		r := httptest.NewRequest(method, path, bytes.NewReader(domain.JSON(body)))
		r.AddCookie(&http.Cookie{Name: "linha_session", Value: cookies[subject]})
		r.Header.Set("Origin", origin)
		r.Header.Set("X-Linha-CSRF", csrf)
		r.Header.Set("X-Linha-Admin-Role", "operator")
		r.Header.Set("Authorization", "Bearer A")
		if subject != "sdk" {
			r.Header.Del("Authorization")
		}
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, r)
		if out.Code != want {
			t.Fatalf("%s %s: %d want %d: %s", method, path, out.Code, want, out.Body)
		}
		var result map[string]any
		json.NewDecoder(out.Body).Decode(&result)
		return result
	}
	call("ordinary", "GET", "/v1/admin/overview", nil, "", "", 403)
	call("viewer", "GET", "/v1/admin/overview", nil, "", "", 200)
	call("ordinary", "GET", "/v1/admin/contexts", nil, "", "", 403)
	call("viewer", "GET", "/v1/admin/contexts", nil, "", "", 200)
	input := map[string]any{"handler": "sum", "version": 1, "payload": map[string]any{"n": 20}, "idempotencyKey": "admin-http"}
	path := "/v1/admin/contexts/" + c.ID + "/jobs"
	call("viewer", "POST", path, input, "https://console.example", "csrf", 403)
	call("operator", "POST", path, input, "https://other.example", "csrf", 403)
	call("operator", "POST", path, input, "https://console.example", "bad", 403)
	job := call("operator", "POST", path, input, "https://console.example", "csrf", 202)
	id := job["id"].(string)
	same := call("operator", "POST", path, input, "https://console.example", "csrf", 202)
	if same["id"] != id {
		t.Fatal("lost-response retry changed identity")
	}
	call("sdk", "GET", "/v1/jobs/"+id, nil, "", "", 404)
	call("viewer", "GET", "/v1/admin/jobs/"+id, nil, "", "", 200)
	call("viewer", "POST", "/v1/admin/jobs/"+id+"/cancel", nil, "https://console.example", "csrf", 403)
	call("operator", "POST", "/v1/admin/jobs/"+id+"/cancel", nil, "https://console.example", "csrf", 200)
	audit := call("viewer", "GET", "/v1/admin/jobs/"+id+"/audit", nil, "", "", 200)
	if len(audit["items"].([]any)) != 2 {
		t.Fatal("accepted audit missing")
	}
	call("viewer", "POST", "/v1/admin/logout", nil, "https://console.example", "csrf", 204)
	call("viewer", "GET", "/v1/admin/session", nil, "", "", 401)
	disabled := (&Server{Repo: repo, Auth: principals{}, Engines: engine.Registry{}}).Handler()
	out := httptest.NewRecorder()
	disabled.ServeHTTP(out, httptest.NewRequest("GET", "/v1/admin/contexts", nil))
	if out.Code != 404 || !json.Valid(out.Body.Bytes()) {
		t.Fatal("disabled admin route is exposed or non-JSON")
	}
}
