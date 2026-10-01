package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"linha/server/internal/kube"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestNamespaceOnlyWorkerVerification(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding.EncodeToString
	var mu sync.Mutex
	podUID, saUID, issuer, serverToken := "pod-uid", "sa-uid", "https://cluster-issuer.example", "server-1"
	deleted, missing, assigned := false, false, true
	keyID := "first"
	reviews, keys := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer "+serverToken {
			t.Error("server credential absent or stale")
			http.Error(w, "unauthorized", 401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			// External jwks_uri must not receive the Kubernetes credential.
			json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "jwks_uri": "https://untrusted.invalid/keys", "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/openid/v1/jwks":
			keys++
			json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]string{"kty": "RSA", "kid": keyID, "use": "sig", "alg": "RS256", "n": enc(key.N.Bytes()), "e": enc(big.NewInt(int64(key.E)).Bytes())}}})
		case "/api/v1/namespaces/test/serviceaccounts/worker":
			json.NewEncoder(w).Encode(map[string]any{"metadata": map[string]any{"uid": saUID}})
		case "/api/v1/namespaces/test/pods/linha-instance":
			if missing {
				http.NotFound(w, r)
				return
			}
			metadata := map[string]any{"uid": podUID, "labels": map[string]string{"linha.io/context": "context", "linha.io/instance": "instance"}}
			if deleted {
				metadata["deletionTimestamp"] = time.Now().Format(time.RFC3339)
			}
			json.NewEncoder(w).Encode(map[string]any{"metadata": metadata, "spec": map[string]string{"serviceAccountName": "worker"}})
		default:
			reviews++
			http.Error(w, "cluster access forbidden", 403)
		}
	}))
	defer server.Close()
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte(serverToken), 0600); err != nil {
		t.Fatal(err)
	}
	k := &kube.Client{URL: server.URL, HTTP: server.Client(), Namespace: "test", TokenFile: tokenFile}
	verifier, err := NewWorkerVerifier(context.Background(), k)
	if err != nil {
		t.Fatal(err)
	}
	p := &Kubernetes{Kube: k, ServiceAccount: "worker", WorkerVerifier: verifier, AuthorizeInstance: func(_ context.Context, c, i, u string) error {
		mu.Lock()
		defer mu.Unlock()
		if !assigned || c != "context" || i != "instance" || u != "pod-uid" {
			return Unauthorized
		}
		return nil
	}}
	claims := func() map[string]any {
		return map[string]any{"iss": issuer, "sub": "system:serviceaccount:test:worker", "aud": []string{"linha-worker"}, "exp": time.Now().Add(time.Hour).Unix(), "nbf": time.Now().Add(-time.Minute).Unix(), "kubernetes.io": map[string]any{"namespace": "test", "pod": map[string]string{"name": "linha-instance", "uid": "pod-uid"}, "serviceaccount": map[string]string{"name": "worker", "uid": "sa-uid"}}}
	}
	sign := func(c map[string]any, signingKey *rsa.PrivateKey) string {
		header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": keyID})
		payload, _ := json.Marshal(c)
		body := enc(header) + "." + enc(payload)
		hash := sha256.Sum256([]byte(body))
		sig, err := rsa.SignPKCS1v15(rand.Reader, signingKey, crypto.SHA256, hash[:])
		if err != nil {
			t.Fatal(err)
		}
		return body + "." + enc(sig)
	}
	check := func(raw string, want bool) {
		t.Helper()
		r := httptest.NewRequest("POST", "/v1/workers/claim", nil)
		r.Header.Set("Authorization", "Bearer "+raw)
		r.Header.Set("X-Linha-Context-Id", "forged-context")
		id, err := p.Worker(context.Background(), r)
		if want && (err != nil || id.ContextID != "context" || id.ID != "pod-uid") {
			t.Fatalf("valid worker: %+v %v", id, err)
		}
		if !want && err == nil {
			t.Fatal("invalid worker accepted")
		}
	}
	good := sign(claims(), key)
	check(good, true)
	for _, change := range []func(map[string]any){
		func(c map[string]any) { c["aud"] = "other" }, func(c map[string]any) { c["iss"] = "https://other" },
		func(c map[string]any) { c["exp"] = 1 }, func(c map[string]any) { c["nbf"] = time.Now().Add(time.Hour).Unix() },
		func(c map[string]any) { c["sub"] = "system:serviceaccount:other:worker" },
		func(c map[string]any) { delete(c, "kubernetes.io") },
		func(c map[string]any) { c["kubernetes.io"].(map[string]any)["namespace"] = "other" },
		func(c map[string]any) {
			c["kubernetes.io"].(map[string]any)["pod"] = map[string]string{"name": "../other", "uid": "pod-uid"}
		},
	} {
		c := claims()
		change(c)
		check(sign(c, key), false)
	}
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	check(sign(claims(), other), false)
	for _, field := range []string{"pod", "sa", "deleted", "missing", "unassigned"} {
		mu.Lock()
		switch field {
		case "pod":
			podUID = "replacement"
		case "sa":
			saUID = "replacement"
		case "deleted":
			deleted = true
		case "missing":
			missing = true
		case "unassigned":
			assigned = false
		}
		mu.Unlock()
		check(good, false)
		mu.Lock()
		podUID = "pod-uid"
		saUID = "sa-uid"
		deleted = false
		missing = false
		assigned = true
		mu.Unlock()
	}
	// Both Kubernetes credentials and signing keys can rotate without restart.
	mu.Lock()
	serverToken = "server-2"
	key = other
	keyID = "second"
	mu.Unlock()
	if err := os.WriteFile(tokenFile, []byte("server-2"), 0600); err != nil {
		t.Fatal(err)
	}
	check(sign(claims(), other), true)
	mu.Lock()
	defer mu.Unlock()
	if reviews != 0 || keys < 2 {
		t.Fatalf("unexpected API use: %d cluster calls, %d JWKS fetches", reviews, keys)
	}
}

func TestDisabledSecurityAndAnonymousOIDC(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/workers/claim", nil)
	a := &Disabled{}
	for _, header := range []string{"", "Bearer invalid"} {
		r.Header.Set("Authorization", header)
		r.Header.Set("X-Linha-Owner", "forged")
		for _, auth := range []Authenticator{a, &Kubernetes{}} {
			owner, err := auth.Owner(context.Background(), r)
			if err != nil || owner != AnonymousOwner {
				t.Fatal("anonymous identity is not stable")
			}
		}
	}
	if _, err := a.Worker(context.Background(), r); err == nil {
		t.Fatal("missing routing identity accepted")
	}
	for k, v := range map[string]string{"X-Linha-Context-Id": "context", "X-Linha-Worker-Id": "worker", "X-Linha-Worker-Incarnation": "inc"} {
		r.Header.Set(k, v)
	}
	if id, err := a.Worker(context.Background(), r); err != nil || id.ID != "worker" {
		t.Fatal(fmt.Sprint(id, err))
	}
	if _, err := (&Kubernetes{}).Worker(context.Background(), r); err == nil {
		t.Fatal("anonymous API disabled worker authentication")
	}
}
