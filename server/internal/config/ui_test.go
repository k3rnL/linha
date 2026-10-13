package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestUISecurityMatrix(t *testing.T) {
	for _, tt := range []struct {
		name      string
		security  Security
		env       map[string]string
		wantError bool
	}{
		{"disabled preserves anonymous public API", Security{Enabled: true, OIDCEnabled: false}, map[string]string{}, false},
		{"anonymous operator explicit", Security{}, map[string]string{"LINHA_UI_ENABLED": "true", "LINHA_UI_PUBLIC_URL": "http://localhost:8080/ui/"}, false},
		{"no implicit anonymous admin", Security{Enabled: true}, map[string]string{"LINHA_UI_ENABLED": "true", "LINHA_UI_PUBLIC_URL": "http://localhost:8080/ui/"}, true},
		{"OIDC viewer/operator defaults deny", Security{Enabled: true, OIDCEnabled: true, Issuer: "https://issuer.example"}, map[string]string{"LINHA_UI_ENABLED": "true", "LINHA_UI_PUBLIC_URL": "https://console.example/ui/", "LINHA_UI_OIDC_CLIENT_ID": "browser"}, false},
		{"missing browser audience", Security{Enabled: true, OIDCEnabled: true, Issuer: "https://issuer.example"}, map[string]string{"LINHA_UI_ENABLED": "true", "LINHA_UI_PUBLIC_URL": "https://console.example/ui/"}, true},
		{"unsafe UI origin", Security{}, map[string]string{"LINHA_UI_ENABLED": "true", "LINHA_UI_PUBLIC_URL": "javascript:alert(1)"}, true},
		{"issuer binding", Security{Enabled: true, OIDCEnabled: true, Issuer: "https://issuer.example"}, map[string]string{"LINHA_UI_ENABLED": "true", "LINHA_UI_PUBLIC_URL": "https://console.example/ui/", "LINHA_UI_OIDC_CLIENT_ID": "browser", "LINHA_ADMIN_MAPPINGS": `{"operator":{"subjects":[{"issuer":"https://attacker.example","subject":"a"}]}}`}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, e := ReadUI(func(key string) string { return tt.env[key] }, tt.security)
			if (e != nil) != tt.wantError {
				t.Fatalf("error=%v wantError=%v", e, tt.wantError)
			}
		})
	}
}

func TestUIInheritedMappingIssuers(t *testing.T) {
	for _, raw := range []string{
		`{"viewer":{"subjects":[{"subject":"viewer"}]},"operator":{"claims":[{"path":["roles"],"values":["operator"]}]}}`,
		`{"viewer":{"claims":[{"path":["roles"],"values":["viewer"]}]},"operator":{"subjects":[{"subject":"operator"}]}}`,
	} {
		values := map[string]string{
			"LINHA_UI_ENABLED": "true", "LINHA_UI_PUBLIC_URL": "https://linha.example/ui/",
			"LINHA_UI_OIDC_CLIENT_ID": "browser", "LINHA_ADMIN_MAPPINGS": raw,
		}
		if _, err := ReadUI(env(values), Security{Enabled: true, OIDCEnabled: true, Issuer: "https://issuer.example"}); err != nil {
			t.Fatalf("inherited mappings rejected: %v", err)
		}
	}
}

func TestUIOIDCScopes(t *testing.T) {
	security := Security{Enabled: true, OIDCEnabled: true, Issuer: "https://issuer.example"}
	for _, tt := range []struct {
		name, raw string
		want      []string
		invalid   bool
	}{
		{"default", "", []string{"openid", "profile", "email"}, false},
		{"groups", `["openid","profile","email","groups"]`, []string{"openid", "profile", "email", "groups"}, false},
		{"replace defaults", `["openid","groups"]`, []string{"openid", "groups"}, false},
		{"openid only", `["openid"]`, []string{"openid"}, false},
		{"order and URI scope", `["api://linha/read","openid","groups"]`, []string{"api://linha/read", "openid", "groups"}, false},
		{"malformed", `["openid"`, nil, true},
		{"plain string", "openid groups", nil, true},
		{"JSON string", `"openid groups"`, nil, true},
		{"object", `{}`, nil, true},
		{"null", `null`, nil, true},
		{"empty list", `[]`, nil, true},
		{"missing openid", `["groups"]`, nil, true},
		{"wrong type", `["openid",123]`, nil, true},
		{"null item", `["openid",null]`, nil, true},
		{"empty token", `["openid",""]`, nil, true},
		{"whitespace", `["openid","group name"]`, nil, true},
		{"newline", `["openid","group\nname"]`, nil, true},
		{"quote", `["openid","group\"name"]`, nil, true},
		{"backslash", `["openid","group\\name"]`, nil, true},
		{"non-ASCII", `["openid","grüppe"]`, nil, true},
		{"duplicate", `["openid","groups","groups"]`, nil, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			values := map[string]string{"LINHA_UI_ENABLED": "true", "LINHA_UI_PUBLIC_URL": "https://linha.example/ui/", "LINHA_UI_OIDC_CLIENT_ID": "browser", "LINHA_UI_OIDC_SCOPES": tt.raw}
			config, err := ReadUI(env(values), security)
			if tt.invalid {
				if err == nil || !strings.Contains(strings.ToLower(err.Error()), "scope") {
					t.Fatalf("expected scope error, got %v", err)
				}
			} else if err != nil || !reflect.DeepEqual(config.Scopes, tt.want) {
				t.Fatalf("scopes=%v error=%v want=%v", config.Scopes, err, tt.want)
			}
		})
	}
}

func TestUnusedBrowserScopesAreIgnored(t *testing.T) {
	for _, raw := range []string{`broken`, `null`, `[]`, `["groups"]`} {
		for _, uiEnabled := range []bool{false, true} {
			values := map[string]string{"LINHA_UI_PUBLIC_URL": "http://linha.example/ui/", "LINHA_UI_OIDC_SCOPES": raw}
			if uiEnabled {
				values["LINHA_UI_ENABLED"] = "true"
			}
			security := Security{Enabled: !uiEnabled}
			if _, err := ReadUI(env(values), security); err != nil {
				t.Fatalf("unused scope config %q UI enabled=%v: %v", raw, uiEnabled, err)
			}
		}
	}
}
