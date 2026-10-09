package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"linha/server/internal/kube"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestOIDCValidationAndOwnerIsolation(t *testing.T) {
	oidcValidationAndOwnerIsolation(t, false)
}
func TestOIDCTLSValidationAndOwnerIsolation(t *testing.T) {
	oidcValidationAndOwnerIsolation(t, true)
}
func oidcValidationAndOwnerIsolation(t *testing.T, useTLS bool) {
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	enc := base64.RawURLEncoding.EncodeToString
	var issuer string
	var signingKey atomic.Pointer[rsa.PrivateKey]
	signingKey.Store(key)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/.well-known/openid-configuration" {
			json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "jwks_uri": issuer + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}})
			return
		}
		current := signingKey.Load()
		json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]string{"kty": "RSA", "kid": enc(current.N.Bytes()), "use": "sig", "alg": "RS256", "n": enc(current.N.Bytes()), "e": enc(big.NewInt(int64(current.E)).Bytes())}}})
	}))
	if useTLS {
		server.StartTLS()
	} else {
		server.Start()
	}
	defer server.Close()
	issuer = server.URL
	if useTLS {
		for _, strict := range []context.Context{context.Background(), OIDCContext(context.Background(), false)} {
			if _, err := NewOIDCVerifier(strict, issuer, "linha"); err == nil {
				t.Fatal("untrusted certificate accepted without opt-in")
			}
		}
		if _, err := NewWorkerVerifier(OIDCContext(context.Background(), true), &kube.Client{URL: issuer, HTTP: http.DefaultClient}); err == nil {
			t.Fatal("OIDC TLS opt-in weakened worker signing-key discovery")
		}
	}
	initialization, cancel := context.WithCancel(context.Background())
	verifier, e := NewOIDCVerifier(OIDCContext(initialization, useTLS), issuer, "linha")
	cancel() // Later requests must still fetch keys after initialization ends.
	p := &Kubernetes{Verifier: verifier}
	if e != nil {
		t.Fatal(e)
	}
	tokenForIssuer := func(subject, audience, tokenIssuer string, expiry int64) string {
		header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": enc(key.N.Bytes())})
		claims, _ := json.Marshal(map[string]any{"iss": tokenIssuer, "sub": subject, "aud": audience, "exp": expiry})
		body := enc(header) + "." + enc(claims)
		hash := sha256.Sum256([]byte(body))
		signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
		if err != nil {
			t.Fatal(err)
		}
		return body + "." + enc(signature)
	}
	token := func(subject, audience string, expiry int64) string {
		return tokenForIssuer(subject, audience, issuer, expiry)
	}
	owner := func(token string) (string, error) {
		r := httptest.NewRequest("GET", "/v1/jobs", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		return p.Owner(context.Background(), r)
	}
	expiry := time.Now().Add(time.Hour).Unix()
	a, e := owner(token("a", "linha", expiry))
	if e != nil {
		t.Fatal(e)
	}
	b, e := owner(token("b", "linha", expiry))
	if e != nil || a == b {
		t.Fatal("subjects share ownership")
	}
	same, e := owner(token("a", "linha", expiry+1))
	if e != nil || same != a {
		t.Fatal("owner changed on credential renewal")
	}
	forged := strings.Split(token("a", "linha", expiry), ".")
	forged[2] = enc(make([]byte, key.Size()))
	for _, invalid := range []string{token("a", "other", expiry), token("a", "linha", 1), token("", "linha", expiry), tokenForIssuer("a", "linha", "https://untrusted.example", expiry), strings.Join(forged, "."), "invalid"} {
		if _, e = owner(invalid); e == nil {
			t.Fatal("invalid token accepted")
		}
	}
	if useTLS {
		// A new key ID forces a refresh through the same scoped TLS client.
		key, e = rsa.GenerateKey(rand.Reader, 2048)
		if e != nil {
			t.Fatal(e)
		}
		signingKey.Store(key)
		if renewed, err := owner(token("a", "linha", expiry)); err != nil || renewed != a {
			t.Fatalf("key rotation changed owner or failed: %s %v", renewed, err)
		}
		response, err := http.DefaultClient.Get(issuer)
		if err == nil {
			response.Body.Close()
			t.Fatal("OIDC option weakened the default HTTP client")
		}
	}
}
func TestProjectedWorkerPodBinding(t *testing.T) {
	valid := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/apis/authentication.k8s.io/v1/tokenreviews" {
			var in map[string]any
			json.NewDecoder(r.Body).Decode(&in)
			if in["spec"].(map[string]any)["audiences"].([]any)[0] != "linha-worker" {
				t.Error("incorrect requested audience")
			}
			json.NewEncoder(w).Encode(map[string]any{"status": map[string]any{"authenticated": valid, "audiences": []string{"linha-worker"}, "user": map[string]any{"username": "system:serviceaccount:test:worker", "extra": map[string][]string{"authentication.kubernetes.io/pod-name": {"linha-instance"}, "authentication.kubernetes.io/pod-uid": {"uid"}}}}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"metadata": map[string]any{"uid": "uid", "labels": map[string]string{"linha.io/context": "context", "linha.io/instance": "instance"}}, "spec": map[string]string{"serviceAccountName": "worker"}})
	}))
	defer server.Close()
	called := false
	p := &Kubernetes{TokenReview: true, Kube: &kube.Client{URL: server.URL, Namespace: "test", HTTP: server.Client()}, ServiceAccount: "worker", AuthorizeInstance: func(_ context.Context, c, i, u string) error {
		called = true
		if c != "context" || i != "instance" || u != "uid" {
			t.Error("scope mismatch")
		}
		return nil
	}}
	r := httptest.NewRequest("POST", "/v1/workers/claim", nil)
	r.Header.Set("Authorization", "Bearer test-projected-token")
	id, e := p.Worker(context.Background(), r)
	if e != nil || !called || id.ID != "uid" {
		t.Fatalf("valid worker: %+v %v", id, e)
	}
	valid = false
	if _, e = p.Worker(context.Background(), r); e == nil {
		t.Fatal("revoked identity accepted")
	}
	p.AuthorizeInstance = func(context.Context, string, string, string) error { return Unauthorized }
	valid = true
	if _, e = p.Worker(context.Background(), r); e == nil {
		t.Fatal("unassigned incarnation accepted")
	}
}

func TestOperatorDriverRequiresApplicationOwner(t *testing.T) {
	owned := true
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owners := []map[string]string{}
		if owned {
			owners = append(owners, map[string]string{"apiVersion": "sparkoperator.k8s.io/v1beta2", "kind": "SparkApplication", "name": "linha-instance"})
		}
		json.NewEncoder(w).Encode(map[string]any{"metadata": map[string]any{"uid": "uid", "labels": map[string]string{"linha.io/context": "context", "linha.io/instance": "instance", "spark-role": "driver"}, "ownerReferences": owners}, "spec": map[string]string{"serviceAccountName": "worker"}})
	}))
	defer api.Close()
	auth := &Kubernetes{Kube: &kube.Client{URL: api.URL, Namespace: "test", HTTP: api.Client()}, ServiceAccount: "worker", AuthorizeInstance: func(context.Context, string, string, string) error { return nil }}
	if _, err := auth.authorizePod(context.Background(), "linha-instance-driver", "uid"); err != nil {
		t.Fatal(err)
	}
	owned = false
	if _, err := auth.authorizePod(context.Background(), "linha-instance-driver", "uid"); err == nil {
		t.Fatal("operator driver without ownership accepted")
	}
}
