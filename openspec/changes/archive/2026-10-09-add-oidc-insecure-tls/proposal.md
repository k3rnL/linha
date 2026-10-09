## Why

Linha currently cannot initialize OIDC against an identity provider whose HTTPS certificate is not trusted by the server. Operators need an explicit opt-in to skip certificate verification when installing the provider CA is impractical.

## What Changes

- Add `security.oidc.tls.insecureSkipVerify`, defaulting to false, and the equivalent server environment variable `LINHA_OIDC_TLS_INSECURE_SKIP_VERIFY`.
- Apply the setting consistently to OIDC discovery, signing-key retrieval, and browser OAuth token exchange.
- Preserve JWT signature, issuer, audience, expiry, browser nonce, and administrator membership checks.
- Isolate the setting from Kubernetes worker authentication, storage, database, and other HTTP clients.
- Document the option and the alternative of mounting the provider CA.

## Capabilities

### New Capabilities

- `oidc-tls-configuration`: Explicit, scoped TLS certificate verification settings for the public API and administrator browser authentication.

### Modified Capabilities

None.

## Impact

Go server security configuration and authentication initialization, Helm values/schema/Deployment, authentication and chart tests, and deployment documentation. No SDK protocol, PostgreSQL schema, job lifecycle, acknowledgement, ownership, retry, fencing, or result publication changes. No new dependencies.
