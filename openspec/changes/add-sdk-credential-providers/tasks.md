## 1. JVM OAuth credentials

- [x] 1.1 Implement redacted credential helpers and expiry-aware concurrent refresh with failure recovery tests.
- [x] 1.2 Keep bounded HTTP/TLS and OAuth client credentials; remove provider-specific client integrations and verify protocol, rotation, timeout and redaction behavior.
- [x] 1.3 Configure example clients with OAuth and validate real client credentials issuance and renewal using a disposable identity provider.

## 2. Independent server authentication

- [x] 2.1 Split API JWT and browser OIDC configuration/verifiers/TLS; verify multi-subject ownership and admin isolation with signed tokens.
- [x] 2.2 Update Linha Helm validation/rendering and tests for independent settings and rejected shared audience.

## 3. Documentation and verification

- [x] 3.1 Document OAuth client setup, client-secret rotation and independent API/browser settings with standalone examples.
- [x] 3.2 Run Java 17 SDK builds, Go/vet with disposable PostgreSQL and auth race tests, Helm checks, restart smoke tests, formatting and strict OpenSpec validation. Record actual evidence and deployment limits.
