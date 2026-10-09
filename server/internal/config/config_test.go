package config

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}
func TestDatabaseSecretCharacters(t *testing.T) {
	values := map[string]string{"LINHA_DATABASE_HOST": "::1", "LINHA_DATABASE_PORT": "5544", "LINHA_DATABASE_NAME": "linha / db", "LINHA_DATABASE_USERNAME": "user@ /'", "LINHA_DATABASE_PASSWORD": "p@ss:/?#&='\\% +$()", "LINHA_DATABASE_SSL_MODE": "disable"}
	dsn, err := DatabaseURL(env(values))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	c := parsed.ConnConfig
	if c.User != values["LINHA_DATABASE_USERNAME"] || c.Password != values["LINHA_DATABASE_PASSWORD"] || c.Database != values["LINHA_DATABASE_NAME"] || c.Host != "::1" || c.Port != 5544 || c.TLSConfig != nil {
		t.Fatal("database fields did not survive encoding")
	}
	values["LINHA_DATABASE_SSL_MODE"] = "verify-full"
	dsn, err = DatabaseURL(env(values))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err = pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.ConnConfig.TLSConfig == nil || parsed.ConnConfig.TLSConfig.InsecureSkipVerify {
		t.Fatal("TLS verification disabled")
	}
	for _, field := range []string{"HOST", "NAME", "USERNAME", "PASSWORD"} {
		key := "LINHA_DATABASE_" + field
		previous := values[key]
		values[key] = ""
		if _, err := DatabaseURL(env(values)); err == nil {
			t.Fatalf("missing %s accepted", key)
		}
		values[key] = previous
	}
	values["LINHA_DATABASE_URL"] = "postgres://standalone/db"
	if _, err := DatabaseURL(env(values)); err == nil {
		t.Fatal("ambiguous configuration accepted")
	}
	dsn, err = DatabaseURL(env(map[string]string{"LINHA_DATABASE_URL": "postgres://standalone/db"}))
	if err != nil || dsn != "postgres://standalone/db" {
		t.Fatal("standalone DSN unsupported")
	}
}
func TestSecuritySwitches(t *testing.T) {
	for _, tc := range []struct {
		name          string
		values        map[string]string
		enabled, oidc bool
		errorPart     string
	}{
		{"secure default needs OIDC", nil, false, false, "OIDC requires"},
		{"OIDC", map[string]string{"LINHA_OIDC_ISSUER": "https://issuer", "LINHA_OIDC_AUDIENCE": "linha"}, true, true, ""},
		{"anonymous API, authenticated workers", map[string]string{"LINHA_OIDC_ENABLED": "false"}, true, false, ""},
		{"all disabled", map[string]string{"LINHA_SECURITY_ENABLED": "false", "LINHA_OIDC_ENABLED": "true", "LINHA_WORKER_AUTH": "ignored"}, false, false, ""},
		{"invalid boolean", map[string]string{"LINHA_SECURITY_ENABLED": "flase"}, false, false, "boolean"},
		{"invalid worker auth", map[string]string{"LINHA_OIDC_ENABLED": "false", "LINHA_WORKER_AUTH": "none"}, false, false, "LINHA_WORKER_AUTH"},
		{"legacy mode", map[string]string{"LINHA_MODE": "development"}, false, false, "was removed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ReadSecurity(env(tc.values))
			if tc.errorPart != "" {
				if err == nil || !strings.Contains(err.Error(), tc.errorPart) {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err != nil || got.Enabled != tc.enabled || got.OIDCEnabled != tc.oidc {
				t.Fatalf("%+v %v", got, err)
			}
			if got.Enabled && got.WorkerAuth != "projected-token" {
				t.Fatal("namespace-safe verification must be default")
			}
		})
	}
}

func TestOIDCTLSSettings(t *testing.T) {
	for _, tc := range []struct {
		name, value          string
		security, oidc, want bool
		wantError            bool
	}{
		{"default", "", true, true, false, false},
		{"strict", "false", true, true, false, false},
		{"insecure", "true", true, true, true, false},
		{"invalid", "typo", true, true, false, true},
		{"security disabled ignores invalid", "typo", false, true, false, false},
		{"OIDC disabled ignores invalid", "typo", true, false, false, false},
		{"security disabled ignores opt-in", "true", false, true, false, false},
		{"OIDC disabled ignores opt-in", "true", true, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]string{
				"LINHA_SECURITY_ENABLED": "true", "LINHA_OIDC_ENABLED": "true",
				"LINHA_OIDC_ISSUER": "https://issuer.example", "LINHA_OIDC_AUDIENCE": "linha",
				"LINHA_OIDC_TLS_INSECURE_SKIP_VERIFY": tc.value,
			}
			if !tc.security {
				values["LINHA_SECURITY_ENABLED"] = "false"
			}
			if !tc.oidc {
				values["LINHA_OIDC_ENABLED"] = "false"
			}
			got, err := ReadSecurity(env(values))
			if tc.wantError {
				if err == nil || !strings.Contains(err.Error(), "LINHA_OIDC_TLS_INSECURE_SKIP_VERIFY") {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err != nil || got.OIDCInsecureSkipVerify != tc.want {
				t.Fatalf("got %+v, %v; want bypass=%v", got, err, tc.want)
			}
		})
	}
}
