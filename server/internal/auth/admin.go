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
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var Forbidden = &domain.Error{Code: "FORBIDDEN", Message: "administrator role required", Status: 403}

type SubjectRule struct {
	Issuer  string `json:"issuer,omitempty"`
	Subject string `json:"subject"`
}
type ClaimRule struct {
	Issuer string   `json:"issuer,omitempty"`
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
	Scopes                                    []string
	Mappings                                  AdminMappings
}
type AdminIdentity struct {
	Actor     string     `json:"actor"`
	Role      string     `json:"role"`
	CSRF      string     `json:"csrf,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}
type Admin struct {
	Config         AdminConfig
	Repo           domain.BrowserRepository
	OAuth          oauth2.Config
	Verifier       *oidc.IDTokenVerifier
	Origin         string
	Secure         bool
	provider       *oidc.Provider
	oidcHTTPClient *http.Client
}

// DefaultBrowserOIDCScopes returns the browser scopes used when no list is configured.
func DefaultBrowserOIDCScopes() []string {
	return []string{oidc.ScopeOpenID, "profile", "email"}
}

func validateBrowserOIDCScopes(scopes []string) error {
	if scopes == nil {
		return nil // Construction without an explicit list uses the defaults.
	}
	if len(scopes) == 0 {
		return fmt.Errorf("browser OIDC scopes must be a nonempty list including openid")
	}
	seen := make(map[string]bool, len(scopes))
	for _, scope := range scopes {
		if scope == "" {
			return fmt.Errorf("browser OIDC scopes must contain nonempty scope tokens")
		}
		for _, c := range scope {
			// RFC 6749 scope-token: printable ASCII except quote and backslash.
			if c < 0x21 || c > 0x7e || c == '"' || c == '\\' {
				return fmt.Errorf("browser OIDC scopes must contain valid OAuth scope tokens")
			}
		}
		if seen[scope] {
			return fmt.Errorf("browser OIDC scopes must not contain duplicates")
		}
		seen[scope] = true
	}
	if !seen[oidc.ScopeOpenID] {
		return fmt.Errorf("browser OIDC scopes must include openid")
	}
	return nil
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
	if err := validateBrowserOIDCScopes(c.Scopes); err != nil {
		return err
	}
	for _, rules := range []RoleRules{c.Mappings.Viewer, c.Mappings.Operator} {
		for _, s := range rules.Subjects {
			if (s.Issuer != "" && s.Issuer != c.Issuer) || s.Subject == "" {
				return fmt.Errorf("admin subjects require a nonempty subject and any explicit issuer must match the configured issuer")
			}
		}
		for _, s := range rules.Claims {
			if (s.Issuer != "" && s.Issuer != c.Issuer) || len(s.Path) == 0 || len(s.Path) > 8 || len(s.Values) == 0 {
				return fmt.Errorf("admin claim rules require a bounded claim path, allowed values and any explicit issuer must match the configured issuer")
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
func NewAdmin(ctx context.Context, c AdminConfig, repo domain.BrowserRepository) (*Admin, error) {
	if c.Scopes == nil {
		c.Scopes = DefaultBrowserOIDCScopes()
	}
	if err := ValidateAdminConfig(c); err != nil {
		return nil, err
	}
	u, _ := url.Parse(c.PublicURL)
	a := &Admin{Config: c, Repo: repo, Origin: u.Scheme + "://" + u.Host, Secure: u.Scheme == "https"}
	if !c.SecurityEnabled {
		return a, nil
	}
	// Keep the provider client for callbacks, whose request contexts do not
	// carry the initialization context's transport policy.
	a.oidcHTTPClient, _ = ctx.Value(oauth2.HTTPClient).(*http.Client)
	call, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	provider, e := oidc.NewProvider(call, c.Issuer)
	if e != nil {
		return nil, e
	}
	a.provider = provider
	a.OAuth = oauth2.Config{ClientID: c.ClientID, ClientSecret: c.ClientSecret, Endpoint: provider.Endpoint(), RedirectURL: strings.TrimSuffix(c.PublicURL, "/") + "/oidc/callback", Scopes: append([]string(nil), c.Scopes...)}
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
func lookupAdminClaim(claims map[string]any, path []string) (any, bool) {
	var value any = claims
	for _, part := range path {
		object, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok = object[part]
		if !ok {
			return nil, false
		}
	}
	return value, true
}

func adminClaimValues(value any) []string {
	var values []string
	switch x := value.(type) {
	case string:
		values = append(values, x)
	case []any:
		for _, item := range x {
			if str, ok := item.(string); ok {
				values = append(values, str)
			}
		}
	}
	return values
}

func matchesAdminValues(values, allowed []string) bool {
	for _, got := range values {
		for _, want := range allowed {
			if got == want {
				return true
			}
		}
	}
	return false
}

func adminClaimKind(value any, present bool) string {
	if !present {
		return "missing"
	}
	switch value.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	case bool:
		return "boolean"
	case float64:
		return "number"
	default:
		return "unsupported"
	}
}

type adminClaimCheck struct {
	Role        string   `json:"role"`
	Path        []string `json:"path"`
	Present     bool     `json:"present"`
	Kind        string   `json:"kind"`
	StringCount int      `json:"string_count"`
	Matched     bool     `json:"matched"`
}

func (a *Admin) logMembershipDenied(ctx context.Context, claims map[string]any) {
	viewer := a.Config.Mappings.Viewer
	operator := a.Config.Mappings.Operator
	viewerCount := len(viewer.Subjects) + len(viewer.Claims)
	operatorCount := len(operator.Subjects) + len(operator.Claims)
	issuer, _ := claims["iss"].(string)
	subject, _ := claims["sub"].(string)
	checks := make([]adminClaimCheck, 0)
	present, stringsPresent := false, false
	for _, role := range []struct {
		name  string
		rules RoleRules
	}{{"operator", operator}, {"viewer", viewer}} {
		for _, rule := range role.rules.Claims {
			value, found := lookupAdminClaim(claims, rule.Path)
			values := adminClaimValues(value)
			present = present || found
			stringsPresent = stringsPresent || len(values) > 0
			matched := issuer == a.Config.Issuer && subject != "" && (rule.Issuer == "" || rule.Issuer == issuer) && matchesAdminValues(values, rule.Values)
			if len(checks) < 32 {
				checks = append(checks, adminClaimCheck{Role: role.name, Path: rule.Path, Present: found, Kind: adminClaimKind(value, found), StringCount: len(values), Matched: matched})
			}
		}
	}
	reason := "claim_values_do_not_match"
	switch {
	case issuer != a.Config.Issuer:
		reason = "issuer_mismatch"
	case subject == "":
		reason = "subject_missing"
	case viewerCount+operatorCount == 0:
		reason = "no_admin_rules"
	case len(viewer.Claims)+len(operator.Claims) == 0:
		reason = "subject_rules_do_not_match"
	case !present:
		reason = "claim_missing"
	case !stringsPresent:
		reason = "claim_type_or_values_unsupported"
	}
	slog.WarnContext(ctx, "browser administrator membership denied",
		"reason", reason, "scopes", a.OAuth.Scopes,
		"viewer_rule_count", viewerCount, "operator_rule_count", operatorCount,
		"claim_checks", checks, "checks_truncated", len(viewer.Claims)+len(operator.Claims) > len(checks))
}

func (a *Admin) role(claims map[string]any) string {
	issuer, _ := claims["iss"].(string)
	subject, _ := claims["sub"].(string)
	if issuer != a.Config.Issuer || subject == "" {
		return ""
	}
	matches := func(rules RoleRules) bool {
		for _, r := range rules.Subjects {
			if (r.Issuer == "" || issuer == r.Issuer) && subject == r.Subject {
				return true
			}
		}
		for _, r := range rules.Claims {
			if r.Issuer != "" && issuer != r.Issuer {
				continue
			}
			v, _ := lookupAdminClaim(claims, r.Path)
			if matchesAdminValues(adminClaimValues(v), r.Values) {
				return true
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
		if a.Verifier == nil {
			return id, Unauthorized
		}
		t, e := a.Verifier.Verify(ctx, token)
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
		if mutate && (r.Header.Get("Origin") != a.Origin || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Linha-CSRF")), []byte(session.CSRF)) != 1) {
			return id, &domain.Error{Code: "CSRF_REJECTED", Message: "same-origin CSRF token required", Status: 403}
		}
		session, e = a.renewBrowserSession(ctx, value, session)
		if e != nil {
			return id, e
		}
		if e = json.Unmarshal(session.Claims, &claims); e != nil {
			return id, Unauthorized
		}
		if claims["iss"] != a.Config.Issuer {
			return id, Unauthorized
		}
		if a.Config.ClientID != "" && !matchesAdminValues(adminClaimValues(claims["aud"]), []string{a.Config.ClientID}) {
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
	if a.oidcHTTPClient != nil {
		call = oidc.ClientContext(call, a.oidcHTTPClient)
	}
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
		a.logMembershipDenied(r.Context(), claims)
		return Forbidden
	}
	expiry := time.Now().Add(time.Duration(a.Config.SessionSeconds) * time.Second)
	authExpiry := authenticationExpiry(id.Expiry, token, expiry)
	if !authExpiry.After(time.Now()) {
		return Unauthorized
	}
	if id.AccessTokenHash != "" && id.VerifyAccessToken(token.AccessToken) != nil {
		return Unauthorized
	}
	if token.RefreshToken == "" {
		expiry = authExpiry
	}
	value := randomSecret()
	session := domain.BrowserSession{Hash: secretHash(value), Claims: domain.JSON(claims), CSRF: randomSecret(), ExpiresAt: expiry, AuthExpiresAt: &authExpiry}
	if token.RefreshToken != "" {
		session.RefreshCipher, e = encryptBrowserTokens(value, browserTokens{Token: *token, Nonce: l.Nonce, ClientID: a.Config.ClientID})
		if e != nil {
			return e
		}
	}
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
