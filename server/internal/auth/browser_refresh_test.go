package auth

import (
	"bytes"
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
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"linha/server/internal/domain"
	"linha/server/internal/postgres"
)

type renewalFixture struct {
	a, b      *Admin
	repo      domain.BrowserRepository
	cookie    *http.Cookie
	mode      atomic.Value
	refreshes atomic.Int32
}

func renewalSetup(t *testing.T, persistent bool, mode string) *renewalFixture {
	t.Helper()
	fixture := &renewalFixture{}
	fixture.mode.Store(mode)
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	enc := base64.RawURLEncoding.EncodeToString
	issuer, nonce, challenge := "", "", ""
	var rotationMu sync.Mutex
	expected := "refresh-0"
	signer := func(claims map[string]any) string {
		text := enc(domain.JSON(map[string]string{"alg": "RS256", "kid": "key"})) + "." + enc(domain.JSON(claims))
		digest := sha256.Sum256([]byte(text))
		sig, e := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
		if e != nil {
			panic(e)
		}
		return text + "." + enc(sig)
	}
	provider := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mode := fixture.mode.Load().(string)
		claims := map[string]any{"iss": issuer, "sub": "administrator", "aud": "browser", "exp": time.Now().Add(2 * time.Minute).Unix(), "nonce": nonce, "groups": []string{"operators"}}
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "jwks_uri": issuer + "/keys", "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "userinfo_endpoint": issuer + "/userinfo", "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/keys":
			json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]string{"kty": "RSA", "kid": "key", "use": "sig", "alg": "RS256", "n": enc(key.N.Bytes()), "e": enc(big.NewInt(int64(key.E)).Bytes())}}})
		case "/userinfo":
			if mode == "userinfo-busy" {
				w.WriteHeader(503)
				return
			}
			sub := "administrator"
			if mode == "userinfo-subject" {
				sub = "different"
			}
			json.NewEncoder(w).Encode(map[string]any{"sub": sub, "groups": []string{"viewers"}})
		case "/token":
			r.ParseForm()
			rotating := r.Form.Get("grant_type") == "refresh_token"
			if rotating {
				fixture.refreshes.Add(1)
				if mode == "busy" {
					w.WriteHeader(503)
					return
				}
				rotationMu.Lock()
				defer rotationMu.Unlock()
				if mode == "revoked" || r.Form.Get("refresh_token") != expected {
					w.WriteHeader(400)
					json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
					return
				}
				expected = fmt.Sprintf("refresh-%s", domain.ID())
			} else {
				digest := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
				if enc(digest[:]) != challenge {
					t.Error("PKCE mismatch")
				}
			}
			switch mode {
			case "subject":
				claims["sub"] = "different"
			case "issuer":
				claims["iss"] = "https://different.example"
			case "audience":
				claims["aud"] = "other"
			case "nonce":
				claims["nonce"] = "different"
			case "expired":
				claims["exp"] = 1
			case "access-hash":
				claims["at_hash"] = "wrong"
			case "roles":
				claims["groups"] = []string{"unrelated"}
			}
			raw := signer(claims)
			if mode == "signature" {
				parts := strings.Split(raw, ".")
				parts[2] = enc(make([]byte, key.Size()))
				raw = strings.Join(parts, ".")
			}
			result := map[string]any{"access_token": "access-secret", "token_type": "Bearer", "expires_in": 120, "id_token": raw}
			if mode != "no-refresh" {
				result["refresh_token"] = expected
			}
			if rotating && (mode == "userinfo" || mode == "userinfo-subject" || mode == "userinfo-busy") {
				delete(result, "id_token")
			}
			json.NewEncoder(w).Encode(result)
		default:
			http.NotFound(w, r)
		}
	}))
	provider.StartTLS()
	t.Cleanup(provider.Close)
	issuer = provider.URL
	var repo domain.BrowserRepository = newBrowserMemory()
	second := repo
	if persistent {
		dsn := os.Getenv("LINHA_TEST_DATABASE")
		if dsn == "" {
			t.Skip("LINHA_TEST_DATABASE required")
		}
		admin, e := pgxpool.New(t.Context(), dsn)
		if e != nil {
			t.Fatal(e)
		}
		schema := "refresh_" + domain.ID()
		if _, e = admin.Exec(t.Context(), "CREATE SCHEMA "+schema); e != nil {
			t.Fatal(e)
		}
		u, e := url.Parse(dsn)
		if e != nil {
			t.Fatal(e)
		}
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		first, e := postgres.Open(t.Context(), u.String())
		if e != nil {
			t.Fatal(e)
		}
		other, e := postgres.Open(t.Context(), u.String())
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() {
			first.Close()
			other.Close()
			admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
			admin.Close()
		})
		repo, second = first, other
	}
	config := AdminConfig{Enabled: true, SecurityEnabled: true, Issuer: issuer, PublicURL: "https://console.example/ui/", ClientID: "browser", SessionSeconds: 3600, Mappings: AdminMappings{
		Viewer: RoleRules{Claims: []ClaimRule{{Path: []string{"groups"}, Values: []string{"viewers"}}}}, Operator: RoleRules{Claims: []ClaimRule{{Path: []string{"groups"}, Values: []string{"operators"}}}},
	}}
	fixture.a, e = NewAdmin(OIDCContext(t.Context(), true), config, repo)
	if e != nil {
		t.Fatal(e)
	}
	fixture.b, e = NewAdmin(OIDCContext(t.Context(), true), config, second)
	if e != nil {
		t.Fatal(e)
	}
	fixture.repo = repo
	out := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "https://console.example/ui/oidc/login", nil)
	if e = fixture.a.Login(out, req); e != nil {
		t.Fatal(e)
	}
	redirect, e := url.Parse(out.Header().Get("Location"))
	if e != nil {
		t.Fatal(e)
	}
	nonce, challenge = redirect.Query().Get("nonce"), redirect.Query().Get("code_challenge")
	callback := httptest.NewRequest("GET", "https://console.example/ui/oidc/callback?code=code&state="+redirect.Query().Get("state"), nil)
	for _, cookie := range out.Result().Cookies() {
		callback.AddCookie(cookie)
	}
	completed := httptest.NewRecorder()
	if e = fixture.b.Callback(completed, callback); e != nil {
		t.Fatal(e)
	}
	for _, cookie := range completed.Result().Cookies() {
		if cookie.Name == "linha_session" {
			fixture.cookie = cookie
		}
	}
	if fixture.cookie == nil {
		t.Fatal("no browser session")
	}
	return fixture
}
func (f *renewalFixture) expireAuthentication(t *testing.T) domain.BrowserSession {
	t.Helper()
	value, e := f.repo.RefreshSession(t.Context(), secretHash(f.cookie.Value), func(s domain.BrowserSession) (domain.BrowserSession, error) {
		past := time.Now().Add(-time.Second)
		s.AuthExpiresAt = &past
		return s, nil
	})
	if e != nil {
		t.Fatal(e)
	}
	return value
}
func (f *renewalFixture) identity(a *Admin) (AdminIdentity, error) {
	req := httptest.NewRequest("GET", "https://console.example/v1/admin/session", nil)
	req.AddCookie(f.cookie)
	return a.Identity(req.Context(), req, false)
}
func TestBrowserRefreshAcrossIndependentReplicas(t *testing.T) {
	for _, persistent := range []bool{false, true} {
		t.Run(fmt.Sprintf("postgres=%t", persistent), func(t *testing.T) {
			f := renewalSetup(t, persistent, "")
			if f.cookie.MaxAge < 3500 {
				t.Fatal("session still capped at ID-token expiry")
			}
			original := f.expireAuthentication(t)
			if bytes.Contains(original.RefreshCipher, []byte("refresh-0")) || bytes.Contains(original.RefreshCipher, []byte("access-secret")) {
				t.Fatal("plaintext provider credential persisted")
			}
			if _, e := decryptBrowserTokens(original.Hash, original.RefreshCipher); e == nil {
				t.Fatal("database lookup hash decrypts credentials")
			}
			errors := make(chan error, 12)
			var wg sync.WaitGroup
			for i := 0; i < 12; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					a := f.a
					if i%2 == 0 {
						a = f.b
					}
					id, e := f.identity(a)
					if e == nil && id.Role != "operator" {
						e = fmt.Errorf("unexpected role %s", id.Role)
					}
					errors <- e
				}(i)
			}
			wg.Wait()
			close(errors)
			for e := range errors {
				if e != nil {
					t.Fatal(e)
				}
			}
			if f.refreshes.Load() != 1 {
				t.Fatal("rotating token consumed more than once", f.refreshes.Load())
			}
			renewed, e := f.repo.Session(t.Context(), original.Hash)
			if e != nil || !renewed.ExpiresAt.Equal(original.ExpiresAt) {
				t.Fatal("absolute lifetime extended", e)
			}
			f.expireAuthentication(t)
			if _, e = f.identity(f.b); e != nil {
				t.Fatal("rotated credential was lost", e)
			}
			req := httptest.NewRequest("POST", "https://console.example/v1/admin/logout", nil)
			req.AddCookie(f.cookie)
			req.Header.Set("Origin", f.a.Origin)
			req.Header.Set("X-Linha-CSRF", original.CSRF)
			if e = f.a.Logout(httptest.NewRecorder(), req); e != nil {
				t.Fatal(e)
			}
			if _, e = f.identity(f.b); e != Unauthorized {
				t.Fatal("logout did not revoke on other replica", e)
			}
		})
	}
}
func TestBrowserRefreshRejectsChangedIdentityAndMembership(t *testing.T) {
	for _, mode := range []string{"revoked", "subject", "issuer", "audience", "nonce", "signature", "expired", "access-hash", "roles", "userinfo-subject"} {
		t.Run(mode, func(t *testing.T) {
			f := renewalSetup(t, false, "")
			f.expireAuthentication(t)
			f.mode.Store(mode)
			if _, e := f.identity(f.b); e == nil {
				t.Fatal("invalid refreshed identity authorized")
			}
			if _, e := f.repo.Session(t.Context(), secretHash(f.cookie.Value)); e != domain.NotFound {
				t.Fatal("invalid session not revoked", e)
			}
		})
	}
}
func TestBrowserRefreshUserInfoAndTransientFailures(t *testing.T) {
	f := renewalSetup(t, false, "")
	f.expireAuthentication(t)
	f.mode.Store("busy")
	if _, e := f.identity(f.a); e == nil || e == Unauthorized {
		t.Fatal("temporary failure forced login or extended identity", e)
	}
	f.mode.Store("userinfo-busy")
	if _, e := f.identity(f.b); e == nil || e == Unauthorized {
		t.Fatal("UserInfo failure authorized expired identity", e)
	}
	f.mode.Store("userinfo")
	id, e := f.identity(f.a)
	if e != nil || id.Role != "viewer" {
		t.Fatal("fresh UserInfo membership or rotated credential lost", id, e)
	}
}
func TestNonrenewableBrowserSessionStaysBounded(t *testing.T) {
	f := renewalSetup(t, false, "no-refresh")
	if f.cookie.MaxAge > 120 {
		t.Fatal("nonrenewable cookie outlives identity")
	}
	f.expireAuthentication(t)
	if _, e := f.identity(f.a); e != Unauthorized {
		t.Fatal("expired nonrenewable identity accepted", e)
	}
	if f.refreshes.Load() != 0 {
		t.Fatal("refresh attempted without a credential")
	}
}

func TestBrowserRefreshDoesNotReuseCredentialsAfterIdentityConfigurationChanges(t *testing.T) {
	for _, field := range []string{"issuer", "client"} {
		for _, expired := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/expired=%t", field, expired), func(t *testing.T) {
				f := renewalSetup(t, false, "")
				if expired {
					f.expireAuthentication(t)
				}
				if field == "issuer" {
					f.b.Config.Issuer = "https://different.example"
				} else {
					f.b.Config.ClientID = "different-browser"
				}
				if _, err := f.identity(f.b); err != Unauthorized {
					t.Fatal("old identity configuration accepted", err)
				}
				if f.refreshes.Load() != 0 {
					t.Fatal("credential sent after issuer/client configuration changed")
				}
			})
		}
	}
}
