## Context

See proposal.md for motivation. API OIDC verification and browser OIDC discovery use the Go default HTTP client. The OIDC library retains the discovery client for future JWKS retrieval, but browser OAuth exchange runs with each callback request's context. Worker JWT signing keys use a separate authenticated Kubernetes client.

## Goals / Non-Goals

**Goals:** Keep one consistent transport policy for both OIDC consumers and all their outbound requests; keep the existing default trust behavior; allow explicit deployments to bypass verification.

**Non-Goals:** No SDK TLS flags, configurable JVM trust, global transport bypass, issuer-mismatch exceptions, migrations, or changes to database and storage TLS policies.

## Decisions

- Parse the new boolean only with active OIDC, following the existing master-switch override. Helm schema validates its type and emits the matching server environment variable.
- Construct a separate OIDC context carrying a cloned HTTP transport when the option is enabled. Pass this context only to API and browser provider initialization. Do not replace the root server context or mutate default clients/transports. Reuse existing constructors and the OIDC/OAuth shared HTTP-client context key instead of adding unrelated constructor options.
- Preserve the browser initialization client on the administrator service and inject it into the callback's bounded request context for token exchange. Retain the provider verifier's key cache and normal JWT validation. Do not retain initialization deadlines or cancellation for future callbacks.
- Keep the default path unchanged, including system CA support. Document a mounted CA using the existing extraVolumes/extraVolumeMounts/extraEnv interfaces; a new CA-specific Helm API is unnecessary for this request.
- Job acknowledgement remains after PostgreSQL acceptance; owner hashes, retry policy, durable leases, attempt fencing, result publication, and recovery after restart are unchanged. No schema or SDK changes are necessary.
- Validate with self-signed HTTPS providers, later JWKS fetches, the existing cross-replica PKCE browser flow, forged/invalid tokens, and an unrelated strict client. Run the existing independent-database browser flow to retain HA/restart invariants, with a disposable PostgreSQL database if available.

## Risks / Trade-offs

- [Provider impersonation when verification is skipped] → Default false; explicit opt-in, startup warning, and documented provider CA alternative. JWT validation remains active, although a transport attacker can impersonate the provider's key endpoint.
- [Browser trust is independent] → Explain that this server option cannot make browsers trust the provider or UI certificate.
- [Discovery-only fix would leave login broken] → Test actual OAuth exchange and delayed signing-key retrieval after constructor cancellation, including callbacks on another administrator instance.

## Migration Plan

Deploy a server image built with this change and upgrade the chart with the option explicitly set only where needed. No persistence migration is required. Restore certificate verification by installing the provider CA, setting the option false, and rolling server replicas. Existing published release 0.1.0 does not gain this feature automatically.
