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
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSeparateAPIAndBrowserIdentities(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding.EncodeToString
	var origin string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/keys" {
			json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]string{"kty": "RSA", "kid": "test", "alg": "RS256", "use": "sig", "n": enc(key.N.Bytes()), "e": enc(big.NewInt(int64(key.E)).Bytes())}}})
			return
		}
		issuer := origin + "/api"
		if r.URL.Path == "/browser/.well-known/openid-configuration" {
			issuer = origin + "/browser"
		}
		json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "jwks_uri": origin + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}})
	}))
	defer server.Close()
	origin = server.URL
	api, err := NewOIDCVerifier(context.Background(), origin+"/api", "linha")
	if err != nil {
		t.Fatal(err)
	}
	browser, err := NewOIDCVerifier(context.Background(), origin+"/browser", "browser-client")
	if err != nil {
		t.Fatal(err)
	}
	public := &Kubernetes{Verifier: api}
	admin := &Admin{Verifier: browser, Config: AdminConfig{SecurityEnabled: true, Issuer: origin + "/browser", Mappings: AdminMappings{Operator: RoleRules{Claims: []ClaimRule{{Path: []string{"groups"}, Values: []string{"operators"}}}}}}}
	token := func(issuer, audience, subject string) string {
		header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "test"})
		claims, _ := json.Marshal(map[string]any{"iss": issuer, "aud": audience, "sub": subject, "exp": time.Now().Add(time.Hour).Unix(), "groups": []string{"operators"}})
		body := enc(header) + "." + enc(claims)
		hash := sha256.Sum256([]byte(body))
		sig, e := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
		if e != nil {
			t.Fatal(e)
		}
		return body + "." + enc(sig)
	}
	request := func(value string) *http.Request {
		r := httptest.NewRequest("GET", "/v1/admin/contexts", nil)
		r.Header.Set("Authorization", "Bearer "+value)
		return r
	}
	owners := map[string]bool{}
	for i := range 10 {
		r := request(token(origin+"/api", "linha", fmt.Sprintf("application-%d", i)))
		owner, e := public.Owner(context.Background(), r)
		if e != nil || owners[owner] {
			t.Fatalf("application isolation failed: %s %v", owner, e)
		}
		owners[owner] = true
		if _, e := admin.Identity(context.Background(), r, false); e != Unauthorized {
			t.Fatalf("API JWT became administrator: %v", e)
		}
	}
	r := request(token(origin+"/browser", "browser-client", "administrator"))
	if _, e := public.Owner(context.Background(), r); e != Unauthorized {
		t.Fatalf("browser JWT accepted by SDK API: %v", e)
	}
	id, e := admin.Identity(context.Background(), r, false)
	if e != nil || id.Role != "operator" {
		t.Fatalf("browser administrator rejected: %+v %v", id, e)
	}
}
