// Package config reads independent database, authentication, and engine settings.
package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
)

type Security struct {
	Enabled                bool
	APIEnabled             bool
	APIInsecureSkipVerify  bool
	APIIssuer, APIAudience string
	OIDCEnabled            bool
	OIDCInsecureSkipVerify bool
	Issuer, WorkerAuth     string
}

func Bool(get func(string) string, key string, fallback bool) (bool, error) {
	if value := get(key); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return false, fmt.Errorf("%s must be a boolean", key)
		}
		return parsed, nil
	}
	return fallback, nil
}

func ReadSecurity(get func(string) string) (s Security, err error) {
	if get("LINHA_MODE") != "" {
		return s, fmt.Errorf("LINHA_MODE was removed; configure LINHA_SECURITY_ENABLED and LINHA_OIDC_ENABLED")
	}
	s.Enabled, err = Bool(get, "LINHA_SECURITY_ENABLED", true)
	if err != nil {
		return
	}
	// The master switch overrides all identity-provider configuration.
	if !s.Enabled {
		return
	}
	if get("LINHA_OIDC_AUDIENCE") != "" {
		return s, fmt.Errorf("LINHA_OIDC_AUDIENCE was removed; configure LINHA_API_JWT_ISSUER and LINHA_API_JWT_AUDIENCE independently of browser OIDC")
	}
	s.APIEnabled, err = Bool(get, "LINHA_API_AUTH_ENABLED", true)
	if err != nil {
		return
	}
	if s.APIEnabled {
		s.APIIssuer, s.APIAudience = get("LINHA_API_JWT_ISSUER"), get("LINHA_API_JWT_AUDIENCE")
		if s.APIAudience == "" {
			s.APIAudience = "linha"
		}
		if s.APIIssuer == "" {
			return s, fmt.Errorf("API JWT authentication requires LINHA_API_JWT_ISSUER")
		}
		s.APIInsecureSkipVerify, err = Bool(get, "LINHA_API_JWT_TLS_INSECURE_SKIP_VERIFY", false)
		if err != nil {
			return
		}
	}
	s.OIDCEnabled, err = Bool(get, "LINHA_OIDC_ENABLED", true)
	if err != nil {
		return
	}
	s.Issuer = get("LINHA_OIDC_ISSUER")
	uiEnabled, err := Bool(get, "LINHA_UI_ENABLED", false)
	if err != nil {
		return s, err
	}
	if s.OIDCEnabled && uiEnabled {
		s.OIDCInsecureSkipVerify, err = Bool(get, "LINHA_OIDC_TLS_INSECURE_SKIP_VERIFY", false)
		if err != nil {
			return s, err
		}
	}
	s.WorkerAuth = get("LINHA_WORKER_AUTH")
	if s.WorkerAuth == "" {
		s.WorkerAuth = "projected-token"
	}
	if s.WorkerAuth != "projected-token" && s.WorkerAuth != "token-review" {
		return s, fmt.Errorf("LINHA_WORKER_AUTH must be projected-token or token-review")
	}
	return
}

// DatabaseURL accepts a standalone DSN or individually supplied connection fields.
// URL encoding keeps arbitrary secret values out of URI/query syntax.
func DatabaseURL(get func(string) string) (string, error) {
	keys := []string{"HOST", "PORT", "NAME", "USERNAME", "PASSWORD", "SSL_MODE", "SSL_ROOT_CERT"}
	if dsn := get("LINHA_DATABASE_URL"); dsn != "" {
		for _, key := range keys {
			if get("LINHA_DATABASE_"+key) != "" {
				return "", fmt.Errorf("use LINHA_DATABASE_URL or separate database fields, not both")
			}
		}
		return dsn, nil
	}
	host, name := get("LINHA_DATABASE_HOST"), get("LINHA_DATABASE_NAME")
	username, password := get("LINHA_DATABASE_USERNAME"), get("LINHA_DATABASE_PASSWORD")
	if host == "" || name == "" || username == "" || password == "" {
		return "", fmt.Errorf("database host, name, username and password are required")
	}
	port := get("LINHA_DATABASE_PORT")
	if port == "" {
		port = "5432"
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", fmt.Errorf("invalid LINHA_DATABASE_PORT")
	}
	ssl := get("LINHA_DATABASE_SSL_MODE")
	if ssl == "" {
		ssl = "verify-full"
	}
	switch ssl {
	case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
	default:
		return "", fmt.Errorf("invalid LINHA_DATABASE_SSL_MODE")
	}
	query := url.Values{"sslmode": {ssl}}
	if cert := get("LINHA_DATABASE_SSL_ROOT_CERT"); cert != "" {
		query.Set("sslrootcert", cert)
	}
	u := url.URL{Scheme: "postgres", Host: net.JoinHostPort(host, port), User: url.UserPassword(username, password), Path: "/" + name, RawQuery: query.Encode()}
	return u.String(), nil
}
