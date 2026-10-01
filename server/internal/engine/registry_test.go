package engine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"linha/server/internal/domain"
	"linha/server/internal/kube"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
)

func TestRegistrySecretResolutionAndRotation(t *testing.T) {
	var password atomic.Value
	password.Store("first-password")
	var requests atomic.Int32
	var deny atomic.Bool
	digest := "sha256:" + strings.Repeat("a", 64)
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if deny.Load() || !ok || user != "worker" || pass != password.Load().(string) {
			w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
			w.WriteHeader(401)
			w.Write([]byte("secret-content-never-logged"))
			return
		}
		if r.URL.Path == "/v2/" {
			w.WriteHeader(200)
			return
		}
		if r.Method != "HEAD" {
			t.Errorf("unexpected registry method: %s", r.Method)
		}
		w.Header().Set("Docker-Content-Digest", digest)
		w.Header().Set("Content-Type", "application/vnd.docker.distribution.manifest.v2+json")
		w.Header().Set("Content-Length", "100")
	}))
	defer registry.Close()
	host := strings.TrimPrefix(registry.URL, "http://")
	var invalid atomic.Bool
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/api/v1/namespaces/test/secrets/pull" {
			t.Errorf("unexpected Secret access: %s", r.URL.Path)
		}
		encoded, _ := json.Marshal(map[string]any{"auths": map[string]any{host: map[string]string{"auth": base64.StdEncoding.EncodeToString([]byte("worker:" + password.Load().(string)))}}})
		if invalid.Load() {
			encoded = []byte(`{"auths":"secret-content-never-logged"`)
		}
		json.NewEncoder(w).Encode(map[string]any{"type": "kubernetes.io/dockerconfigjson", "data": map[string][]byte{".dockerconfigjson": encoded}})
	}))
	defer api.Close()
	k := &Kubernetes{Client: &kube.Client{URL: api.URL, HTTP: api.Client(), Namespace: "test"}, AllowedImages: []string{host + "/apps"}, RegistrySecrets: []string{"pull"}}
	spec := domain.BackendSpec{Image: host + "/apps/worker:1", Engine: domain.EngineSpec{Type: "spark", Version: "3.5.6", Settings: domain.JSON(map[string]any{"kubernetes": PodTemplates{Driver: template(t, `{"spec":{"imagePullSecrets":[{"name":"pull"}]}}`)}})}}
	before := string(domain.JSON(spec))
	for _, pass := range []string{"first-password", "rotated-password"} {
		password.Store(pass)
		got, err := k.Resolve(context.Background(), spec)
		if err != nil || got != host+"/apps/worker@"+digest {
			t.Fatalf("resolution: %s %v", got, err)
		}
	}
	if requests.Load() != 2 {
		t.Fatal("credentials were not reloaded on resolution")
	}
	if string(domain.JSON(spec)) != before {
		t.Fatal("credential material persisted in spec")
	}
	count := requests.Load()
	for _, refs := range [][]string{{"pull", "forbidden"}, {"../pull"}} {
		if _, err := k.registryKeychain(context.Background(), refs); err == nil {
			t.Fatal("invalid Secret selection accepted")
		}
	}
	if requests.Load() != count {
		t.Fatal("Secret read happened before whole-set validation")
	}
	deny.Store(true)
	if _, err := k.Resolve(context.Background(), spec); err == nil || strings.Contains(err.Error(), "secret-content") || strings.Contains(err.Error(), "rotated-password") {
		t.Fatalf("unsafe registry error: %v", err)
	}
	deny.Store(false)
	invalid.Store(true)
	if _, err := k.Resolve(context.Background(), spec); err == nil || strings.Contains(err.Error(), "secret-content") {
		t.Fatalf("unsafe error: %v", err)
	}
}

func TestRegistryScopesDoNotLeakCredentials(t *testing.T) {
	k := registryKeychain{{scope: "registry.example/team", auth: authn.AuthConfig{Username: "scoped", Password: "secret"}}}
	for _, tc := range []struct{ image, user string }{{"registry.example/team/worker:1", "scoped"}, {"registry.example/team-other/worker:1", ""}, {"other.example/team/worker:1", ""}} {
		ref, err := name.ParseReference(tc.image)
		if err != nil {
			t.Fatal(err)
		}
		auth, err := k.Resolve(ref.Context())
		if err != nil {
			t.Fatal(err)
		}
		config, err := auth.Authorization()
		if err != nil {
			t.Fatal(err)
		}
		if config.Username != tc.user {
			t.Fatalf("scope %s received %q", tc.image, config.Username)
		}
	}
	if registryScope("https://index.docker.io/v1/") != "index.docker.io" {
		t.Fatal("Docker Hub alias not recognized")
	}
}
