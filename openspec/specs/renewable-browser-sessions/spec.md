# renewable-browser-sessions Specification

## Purpose

Keep authorized browser sessions usable across short-lived identity tokens and HA replica changes within an explicit maximum session lifetime.

## Requirements

### Requirement: Server-side bounded OIDC renewal
When a provider issues a refresh token, Linha SHALL renew credentials before authentication expires while retaining the configured absolute browser session limit. Refresh credentials SHALL remain encrypted in persistent server storage and SHALL NOT enter browser responses, cookies or logs. Rotation SHALL be serialized across replicas, and logout or revocation SHALL prevent session resurrection. Providers without refresh tokens SHALL retain bounded sign-in behavior.

#### Scenario: Access token expires while a user operates the console
- **WHEN** a valid session with renewable credentials reaches its refresh window on any healthy replica
- **THEN** a refreshed identity is validated and the browser remains usable without an interactive login until the configured session limit or provider session ends

#### Scenario: Concurrent refresh on two replicas
- **WHEN** two replicas refresh the same rotating credential concurrently
- **THEN** refresh is serialized and both requests use the resulting persisted session without consuming a stale credential twice

### Requirement: Refreshed identity and membership remain authoritative
Refresh SHALL validate issuer, audience, subject and applicable nonce/access-token bindings for any returned ID token. If the provider omits a new ID token, Linha SHALL obtain fresh UserInfo using the refreshed access token and verify its subject. Administrator mappings SHALL be reevaluated from fresh identity claims; lost membership, changed identity or invalid/revoked credentials SHALL terminate authorization. Temporary provider failure SHALL NOT extend an expired authentication or discard a still-valid rotating credential. The configured OIDC TLS transport SHALL apply to refresh and UserInfo.

#### Scenario: Membership is revoked or identity changes
- **WHEN** refreshed identity has no administrator membership, an unexpected subject, invalid signature, issuer, audience or nonce
- **THEN** access is denied and the session cannot retain its former administrator permissions

#### Scenario: Hard expiry, missing refresh token or logout
- **WHEN** the maximum session lifetime ends, nonrenewable authentication expires or the user logs out
- **THEN** further access requires a new authorized login regardless of previous refresh success
