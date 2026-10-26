## ADDED Requirements

### Requirement: Configurable browser OIDC scopes
Linha SHALL allow operators to configure the browser OIDC scope list through `ui.oidc.scopes` in Helm or the JSON-array `LINHA_UI_OIDC_SCOPES` environment variable. An omitted setting SHALL request `openid`, `profile`, and `email`. An explicit list SHALL replace those defaults, preserve the supplied order, and contain `openid` and unique nonempty valid OAuth scope tokens. Active browser OIDC SHALL reject malformed, non-array, null, empty, duplicate, invalid-token, or missing-openid configuration before requesting provider discovery. When the UI or master security is disabled, the server SHALL ignore the unused scope environment variable.

Custom scope claims SHALL be subject to the existing verified-token administrator membership rules. Configuring scopes SHALL NOT grant roles, change SDK token audiences or public job ownership, weaken PKCE/state/nonce or token validation, or require sticky sessions.

#### Scenario: Existing deployment omits scopes
- **WHEN** browser OIDC is enabled without explicit scopes
- **THEN** the authorization request includes `openid profile email` and retains existing login validation

#### Scenario: Provider-specific scope selection
- **WHEN** scopes are configured as `[openid, groups]`
- **THEN** browser login requests exactly `openid groups`, without adding optional default scopes

#### Scenario: Invalid active configuration
- **WHEN** active browser scope configuration is malformed, non-array, null, empty, duplicated, lacks openid, or contains invalid scope tokens
- **THEN** the server refuses startup with a scope configuration error before discovery

#### Scenario: Unused scope configuration
- **WHEN** the UI or master security is disabled and the scope environment variable is malformed
- **THEN** the unused setting does not prevent startup or change the effective anonymous/security behavior

#### Scenario: Group membership across replicas
- **WHEN** a browser requests the configured groups scope and the signed ID token contains a group matching a viewer or operator rule
- **THEN** login can begin on one replica, complete on another, and retain the mapped role after service recreation using shared persisted session state

#### Scenario: Scope does not grant administrator membership
- **WHEN** a valid token lacks a configured group, omits its group claim, or belongs to an unrelated group despite the requested scope
- **THEN** login is denied without creating a session or exposing another owner's administration data

#### Scenario: Custom-scope token is invalid or callback is replayed
- **WHEN** a custom-scope callback has invalid state, nonce, issuer, audience, expiry, subject or signature, or reuses a consumed login transaction
- **THEN** it is rejected and creates no session

#### Scenario: Helm renders scope configuration
- **WHEN** a deployment configures the Helm scope list
- **THEN** the server Deployment receives the corresponding JSON-array environment value and invalid explicit lists fail chart validation

### Requirement: Validated administrator mapping placement
The Helm chart SHALL reject unknown properties under `security.oidc`, including administrator mappings placed under `security.oidc.admin`. Administrator mappings configured under `security.admin` SHALL be rendered into the server's role configuration without silently dropping group-claim rules.

#### Scenario: Administrator mappings are nested under OIDC
- **WHEN** chart values configure `security.oidc.admin`
- **THEN** chart validation rejects the misplaced property before deployment instead of rendering empty role mappings

#### Scenario: Group mappings use the canonical location
- **WHEN** chart values configure viewer and operator group-claim mappings under `security.admin`
- **THEN** the Deployment contains those exact configured claim paths and group values

### Requirement: Diagnosable browser administrator membership rejection
A browser callback whose verified identity fails administrator membership SHALL emit a structured server warning that distinguishes missing membership rules, an unavailable or unsupported configured claim, and a nonmatching claim value. The warning SHALL identify requested scopes and configured claim paths without exposing tokens, authorization codes, state, nonce, cookies, client secrets, subject identifiers or arbitrary claim values. The external failure SHALL remain the existing generic administrator 403.

#### Scenario: Verified login has no group claim
- **WHEN** a verified callback fails a configured group rule because the claim is absent
- **THEN** server diagnostics identify that path as absent while the browser receives the generic 403 and no session is created

#### Scenario: Rules are empty or claim values do not match
- **WHEN** verified callback membership fails with no rules or with a present but nonmatching claim
- **THEN** diagnostics distinguish those failures using rule counts and path/match metadata without logging claim values or credentials
