package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"linha/server/internal/auth"
	"linha/server/internal/domain"
	"linha/server/internal/engine"
	"linha/server/internal/postgres"
	"linha/server/internal/storage"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestApplicationGatewayS3IsolationRangeAndServerRestart(t *testing.T) {
	dsn, endpoint, bucket := os.Getenv("LINHA_TEST_DATABASE"), os.Getenv("LINHA_TEST_S3_ENDPOINT"), os.Getenv("LINHA_TEST_S3_BUCKET")
	if dsn == "" || endpoint == "" || bucket == "" {
		t.Skip("requires disposable PostgreSQL and S3 fixtures")
	}
	ctx := context.Background()
	admin, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer admin.Close()
	schema := "gateway_" + domain.ID()
	if _, e = admin.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	uri, _ := url.Parse(dsn)
	query := uri.Query()
	query.Set("search_path", schema)
	uri.RawQuery = query.Encode()
	repo, e := postgres.Open(ctx, uri.String())
	if e != nil {
		t.Fatal(e)
	}
	defer repo.Close()
	s3, e := storage.NewS3(ctx, "data", storage.S3Config{Bucket: bucket, Prefix: domain.ID(), Endpoint: endpoint, Region: "us-east-1", PathStyle: true, AllowWorkloadAccess: true})
	if e != nil {
		t.Fatal(e)
	}
	app := &Server{Repo: repo, Auth: &auth.Disabled{}, Storage: storage.Registry{Destinations: map[string]storage.Backend{"data": s3}}, Engines: engine.Registry{"fake": engine.NewFake()}}
	server := httptest.NewServer(app.Handler())
	defer func() { server.Close() }()
	call := func(method, path, token string, body []byte, want int, headers map[string]string) []byte {
		t.Helper()
		request, e := http.NewRequest(method, server.URL+path, bytes.NewReader(body))
		if e != nil {
			t.Fatal(e)
		}
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		for k, v := range headers {
			request.Header.Set(k, v)
		}
		response, e := server.Client().Do(request)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		raw, _ := io.ReadAll(response.Body)
		if response.StatusCode != want {
			t.Fatalf("%s %s: status %d: %s", method, path, response.StatusCode, raw)
		}
		return raw
	}
	var c domain.BackendContext
	raw := call("POST", "/v1/contexts:ensure", "", domain.JSON(map[string]any{"name": "files", "spec": map[string]any{"image": "test", "engine": map[string]any{"type": "fake", "version": "1", "settings": map[string]any{}}, "results": map[string]any{"type": "s3", "destination": "data"}, "storage": map[string]any{"destinations": []string{"data"}}}}), 200, nil)
	if e = json.Unmarshal(raw, &c); e != nil {
		t.Fatal(e)
	}
	var a domain.StorageSession
	raw = call("POST", "/v1/contexts/"+c.ID+"/storage-sessions", "", domain.JSON(domain.StorageSessionInput{Destination: "data", ClientLease: c.ClientLease.ID}), 201, nil)
	if e = json.Unmarshal(raw, &a); e != nil {
		t.Fatal(e)
	}
	base := "/v1/storage/sessions/" + a.ID + "/"
	call("PUT", base+"data?path=dir/file.nc", a.Token, []byte("0123456789"), 200, nil)
	call("GET", base+"data?path=dir/file.nc", "invalid", nil, 401, nil)
	if got := call("GET", base+"data?path=dir/file.nc", a.Token, nil, 206, map[string]string{"Range": "bytes=3-5"}); string(got) != "345" {
		t.Fatal("range", string(got))
	}
	call("GET", base+"stat?path=dir", a.Token, nil, 200, nil)
	call("GET", base+"list?path=dir", a.Token, nil, 200, nil)
	for _, path := range []string{"../outside", "dir/../outside", "/absolute", "dir//file"} {
		call("PUT", base+"data?path="+url.QueryEscape(path), a.Token, []byte("x"), 400, nil)
	}
	server.Close()
	server = httptest.NewServer(app.Handler())
	if got := call("GET", base+"data?path=dir/file.nc", a.Token, nil, 200, nil); string(got) != "0123456789" {
		t.Fatal("restart lost session")
	}
	call("POST", base+"lock?path=dir", a.Token, []byte("{}"), 200, nil)
	guard := map[string]string{"X-Linha-Storage-Lock": "dir"}
	call("POST", base+"rename?path=dir/file.nc&target=dir/moved.nc", a.Token, []byte("{}"), 200, guard)
	call("PUT", base+"data?path=outside", a.Token, []byte("x"), 400, guard)
	call("DELETE", base+"lock?path=dir", a.Token, nil, 200, nil)
	call("PUT", base+"data?path=dir/moved.nc", a.Token, []byte("x"), 409, guard)
	call("DELETE", base+"delete?path=dir&recursive=false", a.Token, nil, 409, nil)
	call("DELETE", base+"delete?path=dir&recursive=true", a.Token, nil, 200, nil)
	call("POST", "/v1/contexts/"+c.ID+"/clients/"+c.ClientLease.ID+"/renew", "", []byte("{}"), 200, nil)
	call("DELETE", "/v1/contexts/"+c.ID+"/clients/"+c.ClientLease.ID, "", nil, 204, nil)
	raw = call("GET", base+"stat?path=", a.Token, nil, 409, nil)
	if !strings.Contains(string(raw), "STALE_ATTEMPT") {
		t.Fatal("unfenced client")
	}
}
