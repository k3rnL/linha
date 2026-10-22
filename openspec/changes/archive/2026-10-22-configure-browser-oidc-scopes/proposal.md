## Why

Browser login currently requests only `openid profile email`, so identity provider groups published through a separate scope never reach Linha's administrator role mapping. Deployments need to select the scopes their identity provider supports without changing server code or moving group claims into unrelated scope templates.

## What Changes

- Add `ui.oidc.scopes` in Helm and JSON-array `LINHA_UI_OIDC_SCOPES` for the server.
- Keep `openid profile email` when configuration is omitted; explicit lists replace the defaults and must include `openid`.
- Validate active browser scope configuration and use it in authorization-code login while retaining PKCE, nonce, token validation, group-role checks and HA sessions.
- Document identity provider group scope templates, provider scopes and client group assignments.
- Add structured callback membership-denial diagnostics without exposing authentication credentials or claim values.
- Reject misplaced administrator rules under `security.oidc.admin` so they cannot silently render as empty role mappings.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `admin-web-ui`: Configure the browser's requested OIDC scopes, including group claims needed for administrator membership.

## Impact

Go browser authentication/configuration, Helm values/schema/Deployment, related tests and administrator setup documentation. No database migration, public job protocol change, SDK change or engine behavior change. The server and chart must both include the implementation before configuring custom scopes.
