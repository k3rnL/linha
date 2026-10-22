## Purpose

Provide renewable credentials for long-running JVM clients through OAuth 2.0 while keeping the transport extensible and the server independent of credential acquisition.

## ADDED Requirements

### Requirement: Generic credential acquisition
The SDK SHALL retain a generic token-supplier and expiring-token acquisition boundary and SHALL provide OAuth 2.0 client credentials as its sole built-in client integration. Credential helpers needed by internal workers SHALL remain separate from application deployment configuration. Providers SHALL NOT expose secrets through string rendering or error messages.

#### Scenario: Custom provider
- **WHEN** an application supplies a custom acquisition callback
- **THEN** submissions, leases, status, cancellation and result requests use its credentials without wire changes

#### Scenario: Client-secret rotation
- **WHEN** a mounted OAuth client-secret file changes
- **THEN** the next acquisition reads its current content without requiring a mounted access-token file

#### Scenario: No provider-specific acquisition
- **WHEN** a client uses the documented built-in integration
- **THEN** it obtains tokens through OAuth client credentials without provider-specific login protocols or agents

### Requirement: Expiry-aware concurrent refresh
The SDK SHALL cache expiring tokens, reacquire before expiry, serialize concurrent acquisition and bound repeated failures with a retry delay. It SHALL permit only still-valid cached credentials during refresh failure and SHALL fail closed after expiry. A recreated provider SHALL reacquire without prior process state.

#### Scenario: Concurrent ensure and heartbeats
- **WHEN** ensure, lease and retrieval requests concurrently need renewed credentials
- **THEN** one successful acquisition supplies the callers and request idempotency and lease fencing remain unchanged

#### Scenario: Provider outage and recovery
- **WHEN** acquisition fails as a cached token expires and later recovers
- **THEN** no expired token is returned and later acquisition resumes normal requests

### Requirement: OAuth client credentials protocol
The OAuth provider SHALL exchange client credentials at a configured token endpoint using HTTP Basic client authentication, optional scopes and expiry metadata. Calls SHALL have bounded deadlines and responses, forbid redirects and use isolated TLS settings with certificate verification enabled by default.

#### Scenario: Service account token
- **WHEN** a standards-compatible identity provider accepts the configured client credentials
- **THEN** the SDK uses the returned Bearer access token and renews it before its expiry

#### Scenario: Untrusted certificate
- **WHEN** the provider certificate is untrusted and no CA or bypass is configured
- **THEN** acquisition fails without weakening other clients

#### Scenario: Failed exchange
- **WHEN** an exchange fails or returns an invalid token response
- **THEN** the SDK raises a secret-safe credential error without falling back to anonymous or expired credentials
