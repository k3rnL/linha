## Context

Every client transport call obtains a bearer credential through CredentialProvider. Submission, context heartbeats and result downloads must share a provider instance. The server validates JWTs independently of how applications obtain them. See proposal.md for the scope.

## Goals / Non-Goals

Goals: one standard built-in OAuth flow, reusable token refresh, common API audience, independent browser authentication and clear deployment examples. Worker identity authentication remains separate.

Non-goals: identity-provider provisioning, token brokerage by Linha, old client configuration fallback, ownership migration, background refresh threads or job semantics changes.

## Decisions

1. Keep CredentialProvider as the extensible transport boundary. Serialize refresh through a shared cache, refresh before expiry and back off failed acquisition. Only unexpired cached tokens may be returned during an outage. Context heartbeat traffic triggers renewal; no background scheduler is required. Static/file helpers remain available for internal workers and custom integrations, not as an application deployment mode.
2. Make OAuth 2.0 client credentials the only built-in client acquisition protocol. Configure a token endpoint, client ID, secret supplier and optional scopes. Use HTTP Basic client authentication and the returned Bearer token and expires_in. A mounted file contains a client secret, never a pre-generated client JWT. Re-read the secret on each acquisition so Secret rotation is supported.
3. Keep Java 17 HTTP clients bounded by connection/request deadlines and response sizes, forbid redirects and require HTTPS unless explicitly enabled for local tests. CA trust and TLS bypass are isolated to the provider client. Error messages and rendering never contain credentials or response bodies. Start token expiry at exchange initiation to account for network time.
4. Separate security.api.jwt issuer/audience/TLS from browser security.oidc issuer/TLS and ui.oidc.clientId. One API resource audience defaults to linha regardless of application count; each application retains a distinct issuer/subject identity. Reject old shared-audience settings. Browser tokens do not automatically grant API or administrator access.
5. Fail clearly for incomplete active OAuth configuration. Explicitly disabled client authentication bypasses acquisition. Applications share one provider across their transports. Token renewal never retries mutations on 401 or replaces request idempotency keys.
6. Preserve durable acknowledgement, ownership checks, scoped idempotency, retry policies, worker leases, attempt fencing and result publication. After restart, a provider reacquires credentials; a stable issuer/subject restores access to retained request IDs.

## Risks / Trade-offs

- Acquisition blocks callers: bound latency and cache successful tokens.
- Provider outage beyond token expiry prevents calls: fail closed and recover on a later acquisition.
- Secret rotation takes effect at the next acquisition; configure overlapping identity-provider credentials during rollout.
- A realm URL is an issuer, not a token endpoint: documentation identifies discovery token_endpoint and supplies a generic Keycloak example.
- Changing the issuer or service-account subject changes durable job ownership; this release does not migrate owners.

## Migration Plan

Configure the API issuer and resource audience separately from browser login. Deploy matching SDK and application builds with OAuth client credentials and remove obsolete client JWT Secrets and provider-specific settings. Internal worker projected tokens are unchanged. No legacy fallback or owner migration is provided.
