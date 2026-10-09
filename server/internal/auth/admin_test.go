package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"linha/server/internal/domain"
	"linha/server/internal/postgres"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

type browserMemory struct {
	mu       sync.Mutex
	logins   map[string]domain.LoginTransaction
	sessions map[string]domain.BrowserSession
}

func newBrowserMemory() *browserMemory {
	return &browserMemory{logins: map[string]domain.LoginTransaction{}, sessions: map[string]domain.BrowserSession{}}
}
func (b *browserMemory) SaveLogin(_ context.Context, l domain.LoginTransaction) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.logins[l.Hash] = l
	return nil
}
func (b *browserMemory) TakeLogin(_ context.Context, h string) (domain.LoginTransaction, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	l, ok := b.logins[h]
	delete(b.logins, h)
	if !ok || !l.ExpiresAt.After(time.Now()) {
		return l, domain.NotFound
	}
	return l, nil
}
func (b *browserMemory) SaveSession(_ context.Context, s domain.BrowserSession) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sessions[s.Hash] = s
	return nil
}
func (b *browserMemory) Session(_ context.Context, h string) (domain.BrowserSession, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s, ok := b.sessions[h]
	if !ok || !s.ExpiresAt.After(time.Now()) {
		return s, domain.NotFound
	}
	return s, nil
}
func (b *browserMemory) DeleteSession(_ context.Context, h string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.sessions, h)
	return nil
}
func (b *browserMemory) ExpireBrowserRecords(context.Context) error { return nil }
func TestAdminIssuerBoundNestedMappingsAndCookies(t *testing.T) {
	repo := newBrowserMemory()
	a := &Admin{Repo: repo, Origin: "https://linha.example", Config: AdminConfig{SecurityEnabled: true, Issuer: "https://issuer.example", Mappings: AdminMappings{Viewer: RoleRules{Claims: []ClaimRule{{Issuer: "https://issuer.example", Path: []string{"realm_access", "roles"}, Values: []string{"linha-viewer"}}}}, Operator: RoleRules{Subjects: []SubjectRule{{Issuer: "https://issuer.example", Subject: "operator"}}}}}}
	claims := map[string]any{"iss": "https://issuer.example", "sub": "viewer", "realm_access": map[string]any{"roles": []any{"linha-viewer"}}}
	if got := a.role(claims); got != "viewer" {
		t.Fatal(got)
	}
	claims["iss"] = "https://untrusted"
	if a.role(claims) != "" {
		t.Fatal("cross-issuer role accepted")
	}
	claims["iss"] = "https://issuer.example"
	token := randomSecret()
	session := domain.BrowserSession{Hash: secretHash(token), Claims: domain.JSON(claims), CSRF: "csrf", ExpiresAt: time.Now().Add(time.Minute)}
	repo.SaveSession(context.Background(), session)
	request := func(mutate bool, origin, csrf string) (AdminIdentity, error) {
		r := httptest.NewRequest("GET", "https://linha.example/v1/admin/session", nil)
		r.AddCookie(&http.Cookie{Name: "linha_session", Value: token})
		r.Header.Set("Origin", origin)
		r.Header.Set("X-Linha-CSRF", csrf)
		r.Header.Set("X-Linha-Admin-Role", "operator")
		return a.Identity(r.Context(), r, mutate)
	}
	if id, e := request(false, "", ""); e != nil || id.Role != "viewer" {
		t.Fatalf("%+v %v", id, e)
	}
	for _, v := range [][2]string{{"https://attacker.example", "csrf"}, {"https://linha.example", "wrong"}, {"", ""}} {
		if _, e := request(true, v[0], v[1]); e == nil {
			t.Fatal("CSRF bypass")
		}
	}
	if _, e := request(true, "https://linha.example", "csrf"); e != nil {
		t.Fatal(e)
	} // viewer may log out; mutation endpoints separately require operator.
	a.Config.Mappings = AdminMappings{}
	if _, e := request(false, "", ""); e != Forbidden {
		t.Fatal("mapping not reevaluated")
	}
	session.ExpiresAt = time.Now().Add(-time.Second)
	repo.SaveSession(context.Background(), session)
	if _, e := request(false, "", ""); e != Unauthorized {
		t.Fatal("expired session accepted")
	}
}
func TestOIDCPKCECallbackAcrossReplicasAndTokenValidation(t *testing.T) {
	oidcReplicaFlow(t, false, false)
}
func TestOIDCPKCECallbackWithIndependentDatabasePools(t *testing.T) { oidcReplicaFlow(t, true, false) }
func TestOIDCTLSPKCECallbackAcrossReplicas(t *testing.T)            { oidcReplicaFlow(t, false, true) }
func TestOIDCTLSPKCECallbackWithIndependentDatabasePools(t *testing.T) {
	oidcReplicaFlow(t, true, true)
}
func oidcReplicaFlow(t *testing.T, persistent, useTLS bool) {
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	enc := base64.RawURLEncoding.EncodeToString
	issuer := ""
	nonce, challenge := "", ""
	invalid := ""
	sign := func(claims map[string]any) string {
		header := enc(domain.JSON(map[string]string{"alg": "RS256", "kid": "test"}))
		payload := header + "." + enc(domain.JSON(claims))
		hash := sha256.Sum256([]byte(payload))
		signature, e := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
		if e != nil {
			t.Fatal(e)
		}
		return payload + "." + enc(signature)
	}
	provider := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "jwks_uri": issuer + "/keys", "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/keys":
			json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]string{"kty": "RSA", "kid": "test", "use": "sig", "alg": "RS256", "n": enc(key.N.Bytes()), "e": enc(big.NewInt(int64(key.E)).Bytes())}}})
		case "/token":
			r.ParseForm()
			hash := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if enc(hash[:]) != challenge {
				t.Error("PKCE verifier mismatch")
			}
			claims := map[string]any{"iss": issuer, "sub": "admin", "aud": "browser", "exp": time.Now().Add(time.Minute).Unix(), "nonce": nonce}
			if invalid == "nonce" {
				claims["nonce"] = "wrong"
			}
			if invalid == "audience" {
				claims["aud"] = "sdk"
			}
			if invalid == "expiry" {
				claims["exp"] = 1
			}
			if invalid == "issuer" {
				claims["iss"] = "https://untrusted"
			}
			if invalid == "subject" {
				claims["sub"] = ""
			}
			if invalid == "role" {
				claims["sub"] = "ordinary-user"
			}
			raw := sign(claims)
			if invalid == "signature" {
				parts := strings.Split(raw, ".")
				parts[2] = enc(make([]byte, key.Size()))
				raw = strings.Join(parts, ".")
			}
			json.NewEncoder(w).Encode(map[string]any{"access_token": "never-in-browser", "token_type": "Bearer", "id_token": raw})
		default:
			http.NotFound(w, r)
		}
	}))
	if useTLS {
		provider.StartTLS()
	} else {
		provider.Start()
	}
	defer provider.Close()
	issuer = provider.URL
	var repo domain.BrowserRepository = newBrowserMemory()
	var second domain.BrowserRepository = repo
	if persistent {
		dsn := os.Getenv("LINHA_TEST_DATABASE")
		if dsn == "" {
			t.Skip("LINHA_TEST_DATABASE required")
		}
		ctx := context.Background()
		p, e := pgxpool.New(ctx, dsn)
		if e != nil {
			t.Fatal(e)
		}
		schema := "oidc_" + domain.ID()
		if _, e = p.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
			t.Fatal(e)
		}
		u, e := url.Parse(dsn)
		if e != nil {
			t.Fatal(e)
		}
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		first, e := postgres.Open(ctx, u.String())
		if e != nil {
			t.Fatal(e)
		}
		other, e := postgres.Open(ctx, u.String())
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { first.Close(); other.Close(); p.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); p.Close() })
		repo = first
		second = other
	}
	membership := SubjectRule{Subject: "admin"}
	if !useTLS {
		membership.Issuer = issuer
	} // Retain the legacy explicit-issuer flow too.
	config := AdminConfig{Enabled: true, SecurityEnabled: true, Issuer: issuer, PublicURL: "https://linha.example/ui/", ClientID: "browser", SessionSeconds: 3600, Mappings: AdminMappings{Operator: RoleRules{Subjects: []SubjectRule{membership}}}}
	newReplica := func(repository domain.BrowserRepository) *Admin {
		initialization, cancel := context.WithCancel(context.Background())
		defer cancel()
		a, err := NewAdmin(OIDCContext(initialization, useTLS), config, repository, nil)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	if useTLS {
		if _, err := NewAdmin(context.Background(), config, repo, nil); err == nil {
			t.Fatal("browser provider accepted untrusted certificate without opt-in")
		}
	}
	a := newReplica(repo)
	b := newReplica(second)
	login := func() (*http.Request, *httptest.ResponseRecorder) {
		out := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "https://linha.example/ui/oidc/login", nil)
		if e := a.Login(out, r); e != nil {
			t.Fatal(e)
		}
		u, e := url.Parse(out.Header().Get("Location"))
		if e != nil {
			t.Fatal(e)
		}
		nonce = u.Query().Get("nonce")
		challenge = u.Query().Get("code_challenge")
		if u.Query().Get("code_challenge_method") != "S256" {
			t.Fatal("PKCE method missing")
		}
		callback := httptest.NewRequest("GET", "https://linha.example/ui/oidc/callback?state="+u.Query().Get("state")+"&code=test", nil)
		for _, c := range out.Result().Cookies() {
			callback.AddCookie(c)
		}
		return callback, httptest.NewRecorder()
	}
	callback, out := login()
	if e := b.Callback(out, callback); e != nil {
		t.Fatal(e)
	}
	if out.Header().Get("Location") != "/ui/" {
		t.Fatal("login did not land on overview", out.Header())
	}
	if out.Code != 303 {
		t.Fatal(out.Code)
	}
	sessionCookie := out.Result().Cookies()[1]
	if !sessionCookie.HttpOnly || !sessionCookie.Secure || sessionCookie.SameSite != http.SameSiteLaxMode {
		t.Fatal("insecure session cookie")
	}
	if _, e := repo.Session(context.Background(), sessionCookie.Value); e == nil {
		t.Fatal("raw session identifier stored")
	}
	request := httptest.NewRequest("GET", "https://linha.example/v1/admin/session", nil)
	request.AddCookie(sessionCookie)
	id, e := a.Identity(request.Context(), request, false)
	if e != nil || id.Role != "operator" || time.Until(*id.ExpiresAt) > time.Minute {
		t.Fatalf("cross replica session: %+v %v", id, e)
	}
	if e = b.Callback(httptest.NewRecorder(), callback); e == nil {
		t.Fatal("callback replay accepted")
	}
	for _, mode := range []string{"nonce", "audience", "expiry", "issuer", "subject", "signature", "role"} {
		invalid = mode
		callback, out = login()
		if e = b.Callback(out, callback); e == nil {
			t.Fatal("invalid token accepted:", mode)
		}
	}
	invalid = ""
	callback, out = login()
	config.Mappings.Operator.Subjects[0].Issuer = "" // Upgrade legacy rules without invalidating retained identity.
	b = newReplica(second)                           // Recreate the callback service with durable login/session state.
	if e = b.Callback(out, callback); e != nil {
		t.Fatalf("callback after restart: %v", e)
	}
	if _, e = b.Identity(request.Context(), request, false); e != nil {
		t.Fatalf("session after restart: %v", e)
	}
	callback, _ = login()
	callback.Header.Del("Cookie")
	if e = b.Callback(httptest.NewRecorder(), callback); e == nil {
		t.Fatal("callback without browser state accepted")
	}
}

func TestAdminMappingsInheritConfiguredIssuer(t *testing.T) {
	for _, role := range []string{"viewer", "operator"} {
		for _, kind := range []string{"subject", "claim"} {
			for _, ruleIssuer := range []string{"", "https://issuer.example", "https://untrusted.example"} {
				t.Run(role+"/"+kind+"/"+ruleIssuer, func(t *testing.T) {
					config := AdminConfig{
						Enabled: true, SecurityEnabled: true, Issuer: "https://issuer.example",
						PublicURL: "https://linha.example/ui/", ClientID: "browser", SessionSeconds: 3600,
					}
					rules := RoleRules{}
					if kind == "subject" {
						rules.Subjects = []SubjectRule{{Issuer: ruleIssuer, Subject: "admin"}}
					} else {
						rules.Claims = []ClaimRule{{Issuer: ruleIssuer, Path: []string{"realm_access", "roles"}, Values: []string{"linha-admin"}}}
					}
					if role == "operator" {
						config.Mappings.Operator = rules
					} else {
						config.Mappings.Viewer = rules
					}
					err := ValidateAdminConfig(config)
					if ruleIssuer == "https://untrusted.example" {
						if err == nil {
							t.Fatal("conflicting explicit issuer accepted")
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					a := &Admin{Config: config}
					claims := map[string]any{"iss": config.Issuer, "sub": "admin", "realm_access": map[string]any{"roles": []any{"linha-admin"}}}
					if got := a.role(claims); got != role {
						t.Fatalf("role=%q want %q", got, role)
					}
					claims["iss"] = "https://untrusted.example"
					if a.role(claims) != "" {
						t.Fatal("inherited rule accepted another issuer")
					}
					claims["iss"] = config.Issuer
					claims["sub"] = ""
					if a.role(claims) != "" {
						t.Fatal("inherited rule accepted an empty subject")
					}
					claims["sub"] = "ordinary-user"
					claims["realm_access"] = map[string]any{"roles": []any{"unrelated"}}
					if a.role(claims) != "" {
						t.Fatal("non-member received an admin role")
					}
					a.Config.Mappings = AdminMappings{}
					if a.role(claims) != "" {
						t.Fatal("missing membership rules granted access")
					}
				})
			}
		}
	}
}
