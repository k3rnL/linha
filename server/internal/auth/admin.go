package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"linha/server/internal/domain"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var Forbidden = &domain.Error{Code: "FORBIDDEN", Message: "administrator role required", Status: 403}

type SubjectRule struct {
	Issuer  string `json:"issuer"`
	Subject string `json:"subject"`
}
type ClaimRule struct {
	Issuer string   `json:"issuer"`
	Path   []string `json:"path"`
	Values []string `json:"values"`
}
type RoleRules struct {
	Subjects []SubjectRule `json:"subjects"`
	Claims   []ClaimRule   `json:"claims"`
}
type AdminMappings struct {
	Viewer   RoleRules `json:"viewer"`
	Operator RoleRules `json:"operator"`
}
type AdminConfig struct {
	Enabled, SecurityEnabled                  bool
	Issuer, PublicURL, ClientID, ClientSecret string
	SessionSeconds                            int
	Mappings                                  AdminMappings
}
type AdminIdentity struct {
	Actor     string     `json:"actor"`
	Role      string     `json:"role"`
	CSRF      string     `json:"csrf,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}
type Admin struct {
	Config                AdminConfig
	Repo                  domain.BrowserRepository
	OAuth                 oauth2.Config
	Verifier, APIVerifier *oidc.IDTokenVerifier
	Origin                string
	Secure                bool
}

func ValidateAdminConfig(c AdminConfig) error {
	if !c.Enabled {
		return nil
	}
	u, e := url.Parse(c.PublicURL)
	if e != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || u.Fragment != "" || u.Path != "/ui/" {
		return fmt.Errorf("UI public URL must be an absolute http(s) URL ending in /ui/")
	}
	if c.SessionSeconds < 60 || c.SessionSeconds > 86400 {
		return fmt.Errorf("UI session seconds must be 60-86400")
	}
	if !c.SecurityEnabled {
		return nil
	}
	if c.Issuer == "" || c.ClientID == "" {
		return fmt.Errorf("enabled UI requires OIDC issuer and browser client ID when security is enabled")
	}
	for _, rules := range []RoleRules{c.Mappings.Viewer, c.Mappings.Operator} {
		for _, s := range rules.Subjects {
			if s.Issuer != c.Issuer || s.Subject == "" {
				return fmt.Errorf("admin subjects must bind the configured issuer and a nonempty subject")
			}
		}
		for _, s := range rules.Claims {
			if s.Issuer != c.Issuer || len(s.Path) == 0 || len(s.Path) > 8 || len(s.Values) == 0 {
				return fmt.Errorf("admin claim rules require the trusted issuer, a bounded claim path and allowed values")
			}
			for _, p := range s.Path {
				if p == "" || len(p) > 128 || p == "__proto__" {
					return fmt.Errorf("invalid admin claim path")
				}
			}
		}
	}
	return nil
}
func NewAdmin(ctx context.Context, c AdminConfig, repo domain.BrowserRepository, apiVerifier *oidc.IDTokenVerifier) (*Admin, error) {
	if err := ValidateAdminConfig(c); err != nil {
		return nil, err
	}
	u, _ := url.Parse(c.PublicURL)
	a := &Admin{Config: c, Repo: repo, Origin: u.Scheme + "://" + u.Host, Secure: u.Scheme == "https", APIVerifier: apiVerifier}
	if !c.SecurityEnabled {
		return a, nil
	}
	call, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	provider, e := oidc.NewProvider(call, c.Issuer)
	if e != nil {
		return nil, e
	}
	a.OAuth = oauth2.Config{ClientID: c.ClientID, ClientSecret: c.ClientSecret, Endpoint: provider.Endpoint(), RedirectURL: strings.TrimSuffix(c.PublicURL, "/") + "/oidc/callback", Scopes: []string{oidc.ScopeOpenID, "profile", "email"}}
	a.Verifier = provider.Verifier(&oidc.Config{ClientID: c.ClientID})
	return a, nil
}
func randomSecret() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func secretHash(v string) string { b := sha256.Sum256([]byte(v)); return hex.EncodeToString(b[:]) }
func cookie(r *http.Request, name string) string {
	c, e := r.Cookie(name)
	if e != nil {
		return ""
	}
	return c.Value
}
func (a *Admin) setCookie(w http.ResponseWriter, name, value string, max int) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, Secure: a.Secure, SameSite: http.SameSiteLaxMode, MaxAge: max})
}
func (a *Admin) role(claims map[string]any) string {
	issuer, _ := claims["iss"].(string)
	subject, _ := claims["sub"].(string)
	if issuer != a.Config.Issuer || subject == "" {
		return ""
	}
	matches := func(rules RoleRules) bool {
		for _, r := range rules.Subjects {
			if issuer == r.Issuer && subject == r.Subject {
				return true
			}
		}
		for _, r := range rules.Claims {
			if issuer != r.Issuer {
				continue
			}
			var v any = claims
			for _, p := range r.Path {
				m, ok := v.(map[string]any)
				if !ok {
					v = nil
					break
				}
				v = m[p]
			}
			values := []string{}
			switch x := v.(type) {
			case string:
				values = append(values, x)
			case []any:
				for _, el := range x {
					if str, ok := el.(string); ok {
						values = append(values, str)
					}
				}
			}
			for _, got := range values {
				for _, want := range r.Values {
					if got == want {
						return true
					}
				}
			}
		}
		return false
	}
	if matches(a.Config.Mappings.Operator) {
		return "operator"
	}
	if matches(a.Config.Mappings.Viewer) {
		return "viewer"
	}
	return ""
}
func (a *Admin) Identity(ctx context.Context, r *http.Request, mutate bool) (id AdminIdentity, err error) {
	if !a.Config.SecurityEnabled {
		return AdminIdentity{Actor: "anonymous", Role: "operator"}, nil
	}
	var claims map[string]any
	if token := Bearer(r); token != "" {
		if a.APIVerifier == nil {
			return id, Unauthorized
		}
		t, e := a.APIVerifier.Verify(ctx, token)
		if e != nil {
			return id, Unauthorized
		}
		if e = t.Claims(&claims); e != nil {
			return id, Unauthorized
		}
		id.ExpiresAt = &t.Expiry
	} else {
		value := cookie(r, "linha_session")
		if len(value) != 43 {
			return id, Unauthorized
		}
		session, e := a.Repo.Session(ctx, secretHash(value))
		if e != nil {
			if e == domain.NotFound {
				return id, Unauthorized
			}
			return id, e
		}
		if e = json.Unmarshal(session.Claims, &claims); e != nil {
			return id, Unauthorized
		}
		id.CSRF = session.CSRF
		id.ExpiresAt = &session.ExpiresAt
		if mutate && (r.Header.Get("Origin") != a.Origin || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Linha-CSRF")), []byte(session.CSRF)) != 1) {
			return id, &domain.Error{Code: "CSRF_REJECTED", Message: "same-origin CSRF token required", Status: 403}
		}
	}
	id.Role = a.role(claims)
	if id.Role == "" {
		return id, Forbidden
	}
	id.Actor = fmt.Sprintf("%s#%s", claims["iss"], claims["sub"])

	return id, nil
}
func (a *Admin) Login(w http.ResponseWriter, r *http.Request) error {
	if !a.Config.SecurityEnabled {
		http.Redirect(w, r, "/ui/", http.StatusSeeOther)
		return nil
	}
	state, nonce, verifier := randomSecret(), randomSecret(), oauth2.GenerateVerifier()
	l := domain.LoginTransaction{Hash: secretHash(state), Nonce: nonce, Verifier: verifier, ExpiresAt: time.Now().Add(5 * time.Minute)}
	if e := a.Repo.SaveLogin(r.Context(), l); e != nil {
		return e
	}
	a.setCookie(w, "linha_login", state, 300)
	http.Redirect(w, r, a.OAuth.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oidc.Nonce(nonce)), http.StatusSeeOther)
	return nil
}
func (a *Admin) Callback(w http.ResponseWriter, r *http.Request) error {
	if !a.Config.SecurityEnabled {
		return Forbidden
	}
	state := r.URL.Query().Get("state")
	if len(state) != 43 || subtle.ConstantTimeCompare([]byte(state), []byte(cookie(r, "linha_login"))) != 1 {
		return Unauthorized
	}
	l, e := a.Repo.TakeLogin(r.Context(), secretHash(state))
	if e != nil {
		if e == domain.NotFound {
			return Unauthorized
		}
		return e
	}
	a.setCookie(w, "linha_login", "", -1)
	call, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	token, e := a.OAuth.Exchange(call, r.URL.Query().Get("code"), oauth2.VerifierOption(l.Verifier))
	if e != nil {
		return Unauthorized
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		return Unauthorized
	}
	id, e := a.Verifier.Verify(call, raw)
	if e != nil || id.Nonce != l.Nonce || id.Subject == "" {
		return Unauthorized
	}
	var claims map[string]any
	if e = id.Claims(&claims); e != nil {
		return Unauthorized
	}
	if a.role(claims) == "" {
		return Forbidden
	}
	expiry := time.Now().Add(time.Duration(a.Config.SessionSeconds) * time.Second)
	if id.Expiry.Before(expiry) {
		expiry = id.Expiry
	}
	if !expiry.After(time.Now()) {
		return Unauthorized
	}
	value := randomSecret()
	session := domain.BrowserSession{Hash: secretHash(value), Claims: domain.JSON(claims), CSRF: randomSecret(), ExpiresAt: expiry}
	if e = a.Repo.SaveSession(call, session); e != nil {
		return e
	}
	a.setCookie(w, "linha_session", value, int(time.Until(expiry).Seconds()))
	http.Redirect(w, r, "/ui/", http.StatusSeeOther)
	return nil
}
func (a *Admin) Logout(w http.ResponseWriter, r *http.Request) error {
	if _, e := a.Identity(r.Context(), r, true); e != nil {
		return e
	}
	if value := cookie(r, "linha_session"); value != "" {
		if e := a.Repo.DeleteSession(r.Context(), secretHash(value)); e != nil {
			return e
		}
	}
	a.setCookie(w, "linha_session", "", -1)
	w.WriteHeader(204)
	return nil
}
