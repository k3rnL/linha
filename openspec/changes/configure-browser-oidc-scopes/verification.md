# Verification: configure-browser-oidc-scopes

Reviewed on 2026-10-12. Implementation and all seven tasks are complete; no release was published for this change.

## Results

- `make check` passed with a disposable PostgreSQL 17 database: all Go package tests and vet passed.
- `go test -race -count=1 ./server/internal/auth ./server/internal/config` passed with the same database, including independent pools and recreated callback services.
- All 18 Helm configuration tests passed on Helm 3.18.4 and Helm 4.3.0. Tests cover default/custom/replacement scope lists, JSON environment rendering, absent variables when UI is disabled, and rejection of invalid explicit lists.
- Data formatting, Python Black checks, Go formatting and `git diff --check` passed. Strict OpenSpec validation passed for all 25 items.
- `make build VERSION=dev-oidc-scopes REVISION=6512168-dirty` rebuilt the embedded console and Go server. Docker image `linha-server:oidc-scopes` reports `Linha dev-oidc-scopes` and rejects malformed/null/empty/missing-openid/duplicate active scope configuration before database/provider initialization.

## Scenario review

| Contract | Evidence |
| --- | --- |
| Default authorization scopes and compatible direct construction | Existing signed browser flows assert `openid profile email` |
| Explicit replacement including custom groups | Signed group flows request exactly `openid groups`; URI/order configuration cases pass |
| Invalid active configuration, before discovery | Parser cases cover JSON shapes, null entries, token characters, duplicates and missing openid; direct-construction fixture records zero discovery requests |
| Inactive environment configuration ignored | UI-disabled and master-security-disabled parser cases |
| Group viewer/operator login across replicas and recreation | Signed provider emits group claims only for a requested groups scope; independent PostgreSQL pools retain login/session state after callback service recreation |
| Non-member and missing-claim denial | Group flows require `FORBIDDEN` and emit no session cookie |
| Existing callback validation | Group flows reject nonce, audience, expiry, issuer, empty subject, signature, replay and missing browser state; TLS flows retain explicit trust behavior |
| Helm deployment contract | Default/custom list round-trips and invalid-list schema tests on Helm 3 and 4 |

The provider integration uses a signed local OIDC fixture, not a live identity provider deployment. Documentation explains provider scope templates, supported scopes, identity-group assignments, ID-token claims and fresh login. Existing released binaries with the fixed scope list need an updated server build as well as the new chart.

No database migration, SDK or public job protocol change. No critical issues or unresolved implementation tasks.

## Follow-up: administrator mapping placement

The user's supplied values placed group rules under `security.oidc.admin`.
Before the guard, Helm rendered both viewer/operator claims and subjects as empty
arrays in `LINHA_ADMIN_MAPPINGS`, which explains the reported administrator 403.
The schema now rejects unknown `security.oidc` properties. A regression test
rejects the misplaced block and confirms both group rules with value `linha-group`
are rendered from `security.admin`. All 18 chart tests pass on both Helm versions;
formatting and strict OpenSpec checks pass. This follow-up changes only the chart,
documentation and planning artifacts; the earlier Go/image checks remain valid.

## Follow-up: missing provider scope and safe denial diagnostics

The running pod was confirmed as `Linha dev-oidc-scopes`. identity provider's provider read
returned `scopes_supported: []` and the groups scope read returned 404, identifying
the missing custom-scope configuration. Documentation now includes the provider
update and explicit namespace guidance. The source confirms that updating only
`scopes_supported` preserves issuer/allowed-client settings.

Verified callback denials now log rule counts, requested scopes and bounded
claim-path presence/type/string-count/match metadata. Shared claim helpers keep
matching and diagnostics consistent. Tests capture warnings from signed group
callbacks, reject membership diagnostics for unverified tokens, and ensure raw
values, identifiers and credentials are absent. External 403/session-denial
behavior is unchanged. Full Go tests/vet and focused auth/config race checks were
rerun with PostgreSQL and passed. Formatting and strict OpenSpec validation passed.
The separate rebuilt `linha-server:oidc-diagnostics` image reports
`Linha dev-oidc-diagnostics` and passes active-scope validation smoke checks.
The user's existing scope image tag was not replaced and no registry push or live
identity provider mutation was performed. Correcting the provider scope configuration can
be tested with the existing deployed scope image.
