package config

import "testing"

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
