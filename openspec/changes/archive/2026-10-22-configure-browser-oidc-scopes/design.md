## Context

See proposal.md for motivation. Browser login uses a provider-specific browser client, authorization code with PKCE, and verified ID-token claims for administrator membership. Login transactions and sessions are stored in PostgreSQL and support replica replacement. The OAuth configuration currently hardcodes three scopes.

## Goals / Non-Goals

**Goals:** expose the requested browser scopes through server/Helm configuration, preserve omitted-setting compatibility, and prove group-only membership works across replicas and service recreation.

**Non-Goals:** change SDK token audiences, fetch UserInfo, infer provider group claims, modify identity provider assignments remotely, or change job ownership, durable acknowledgement, retry/attempt fencing, results or engine management. No database changes are needed.

## Decisions

- Use `ui.oidc.scopes` as a replacement list, with defaults `[openid, profile, email]`. A full list permits removing unsupported optional scopes; an additional-scopes-only setting would not allow that.
- Serialize the Helm list as JSON into `LINHA_UI_OIDC_SCOPES`. JSON preserves scope boundaries and types and follows existing structured environment configuration.
- When browser OIDC is active, reject malformed/non-array/null/empty scope configuration, duplicate scopes, invalid OAuth scope-token characters, or omission of `openid`. Validate before discovery rather than allowing an unusable login redirect. Retain order and exact names. Unset configuration uses fresh default slices; construction without a custom list remains compatible.
- Ignore the unused scope environment variable when the UI or master security is disabled. Helm continues to validate explicit scope values structurally.
- Reject unknown properties of `security.oidc`, including misplaced `admin` rules. Canonical administrator mappings are under `security.admin`; silently ignoring the nested form renders empty mappings and causes a misleading runtime 403. Valid existing OIDC fields retain their behavior.
- Pass the validated list into the browser OAuth configuration. Retain all issuer/signature/audience/expiry, state/nonce/PKCE and membership checks. Requesting a scope does not grant a role.
- On a verified callback membership failure, log a bounded warning with reason, requested scopes, rule counts and configured claim-path presence/type/match status. Keep the external 403 generic. Do not log raw claims, claim values, subject identifiers, tokens, codes, state, nonce, cookies or client secrets. Reuse the same claim matching helpers for authorization and diagnostics to avoid divergent logic.
- Extend the signed provider fixture so it emits group claims only after the groups scope was requested. Exercise viewer and operator membership, non-member/missing-claim denial, forged-token rejection, callback replay, independent database pools and service recreation.

## Risks / Trade-offs

- A requested scope may not be supported or populated by the provider → document supported scopes, templates, identity-group membership and client group assignments; retain fail-closed token/membership checks.
- Changing requested scopes does not add claims to already-issued tokens or stored sessions → require a fresh login after changing the setting.
- An older server ignores the new setting → document that v0.1.1 lacks support and both the new server implementation and updated chart are required.

## Migration Plan

Deploy the new server/chart with the defaults for unchanged behavior. Add a provider-supported `groups` scope and the configured group-claim rules, then log in again. Removing the setting restores the previous requested scope list. Queue state, attempts and retained results are unaffected; rollback does not require a database migration.
