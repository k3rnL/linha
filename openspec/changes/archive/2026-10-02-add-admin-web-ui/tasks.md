## 1. Shared contracts and persistence

- [x] 1.1 Consume the operational-read-model contracts from `add-platform-observability` milestone 1; agree DTO ownership and sequential migration versions before implementation.
- [x] 1.2 Define admin OpenAPI contracts and fixtures for context/version/client/instance/server inspection, requests/attempts, primitive results, creation/replay, and cancellation; include timestamp/freshness and metric-context filters.
- [x] 1.3 Add coordinated PostgreSQL migrations for browser login transactions/sessions, accepted administrative audit, and request actor/replay provenance; verify existing records/SDKs remain readable.
- [x] 1.4 Implement bounded admin list/detail queries and indexes with stable cursor ordering, owner-derived lookup, timestamped read-model integration, and immutable audit/provenance retrieval.

## 2. Administrator authentication and authorization

- [x] 2.1 Implement issuer-bound viewer/operator mappings from configured subjects/claim paths with deny-by-default authorization on every admin endpoint; preserve ordinary SDK owner checks.
- [x] 2.2 Implement server-side OIDC code/PKCE login, nonce/state/audience validation, cross-replica session/login storage, bounded expiry, secure cookies, CSRF/origin checks, and logout.
- [x] 2.3 Add UI public URL/client/Secret-reference configuration and validate the security matrix: UI disabled, OIDC admin enabled, master security disabled, and rejected anonymous-admin configuration with security enabled.
- [x] 2.4 Test non-admin/cross-owner calls, forged claims/headers, viewer mutations, expired sessions, invalid callbacks, CSRF rejection, and callback/session handling through different replicas.

## 3. Administrative request operations and results

- [x] 3.1 Implement transactional admin creation with context-derived ownership, existing envelope validation, fresh idempotency semantics, audit/provenance, and committed acceptance before 202.
- [x] 3.2 Implement replay-source validation, explicit same-logical-context version selection, editable input, fresh deadline/key handling, and conflict-safe idempotent retries without changing source jobs/results.
- [x] 3.3 Implement explicit activation of stopped/draining versions and atomic accepted-work reconciliation without browser leases; test retirement/lease-expiry races and eventual drain after work.
- [x] 3.4 Implement admin cancellation through the existing cancellation/attempt-fencing transaction with audit and stable completion/cancellation ordering.
- [x] 3.5 Implement published local/S3 location metadata, capped escaped JSON preview, retention/unavailability state, file downloads, and paginated dataset parts with no staging/credential exposure.
- [x] 3.6 Add PostgreSQL integration tests for concurrent lost-response retries, provenance conflicts, audit rollback, stale completions, original-owner retrieval, and cancellation racing success.

## 4. Frontend and embedded serving

- [x] 4.1 Create `web/` with pinned React/TypeScript/Vite/shadcn/Tailwind/TanStack Query dependencies, formatting/type checks, typed API access, and browser test tooling.
- [x] 4.2 Embed the production assets in Go, scope SPA fallback to `/ui/`, add runtime configuration/session bootstrap, and verify cache/deep-link behavior in the existing container image.
- [x] 4.3 Build administrator login/logout and viewer/operator navigation with accessible controls and explicit unauthorized/unavailable states.
- [x] 4.4 Build the context landing page, owner/name/version grouping, filters/pagination, client/configuration/instance/request tabs, lifecycle explanations, and read-only polling.
- [x] 4.5 Build Spark instance detail and existing ingress links from the shared observations; cover multiple drivers, no ingress, stale/replaced instances, and non-Spark adapters.
- [x] 4.6 Build global request filters and details for input/progress/attempts/stack traces/audit/results, including capped previews and large paginated datasets.
- [x] 4.7 Build the shared new/replay form with editable JSON and policies, explicit context version/activation, pending-submission key reuse, validation/conflict messages, and new-job navigation.
- [x] 4.8 Build request cancellation controls that show CANCELLING until terminal resolution and preserve unrelated shared-engine work.
- [x] 4.9 Build server health/freshness views and Grafana drill-through filters; pause hidden/unauthenticated polling and prevent read-only views from acquiring leases.

## 5. Deployment, validation, and documentation

- [x] 5.1 Add Helm UI/security values/schema/env/Secret references and examples; keep UI disabled by default and preserve namespace-only RBAC and existing security modes when disabled.
- [x] 5.2 Add CI for frontend formatting/types/build, meaningful browser flows, admin API/OpenAPI conformance, Go/DB integration, and Helm security/render combinations.
- [x] 5.3 Verify on two server replicas: cross-replica login, rollout, accepting-server failure after commit, full restart during queued/running/cancelling work, and retained result/audit retrieval.
- [x] 5.4 Verify real Spark: open existing ingress links, distinguish driver versus registered-worker readiness, replay an edited request, cancel one of two shared-driver jobs, and drain an explicitly reactivated version after browser closure.
- [x] 5.5 Document OIDC/admin setup, external Spark ingress ownership, creation/replay side effects, result-location meaning, UI/observability dependency boundary, migrations, and measured validation limits; validate the completed OpenSpec change before marking tasks done.
