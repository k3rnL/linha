package config

import (
	"encoding/json"
	"fmt"
	"linha/server/internal/auth"
	"strconv"
)

func ReadUI(get func(string) string, security Security) (c auth.AdminConfig, err error) {
	c.Enabled, err = Bool(get, "LINHA_UI_ENABLED", false)
	if err != nil || !c.Enabled {
		return
	}
	c.SecurityEnabled = security.Enabled
	c.Issuer = security.Issuer
	c.PublicURL = get("LINHA_UI_PUBLIC_URL")
	c.ClientID = get("LINHA_UI_OIDC_CLIENT_ID")
	c.ClientSecret = get("LINHA_UI_OIDC_CLIENT_SECRET")
	c.SessionSeconds = 3600
	if raw := get("LINHA_UI_SESSION_SECONDS"); raw != "" {
		c.SessionSeconds, err = strconv.Atoi(raw)
		if err != nil {
			return c, fmt.Errorf("invalid LINHA_UI_SESSION_SECONDS")
		}
	}
	if security.Enabled && !security.OIDCEnabled {
		return c, fmt.Errorf("enabled UI requires OIDC when security is enabled")
	}
	if raw := get("LINHA_ADMIN_MAPPINGS"); raw != "" {
		if err = json.Unmarshal([]byte(raw), &c.Mappings); err != nil {
			return c, fmt.Errorf("invalid admin mappings: %w", err)
		}
	}
	err = auth.ValidateAdminConfig(c)
	return
}
