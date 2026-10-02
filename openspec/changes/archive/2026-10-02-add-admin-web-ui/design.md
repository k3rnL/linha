## Context

See [proposal.md](proposal.md) for scope. The existing API is owner-scoped and has no browser login, administrative role, context list, or engine-detail endpoint. Submission requires a live SDK lease. Jobs, attempts, result metadata, immutable context versions, and client leases already reside in PostgreSQL. Cancellation already transitions running jobs through CANCELLING without stopping a shared SparkContext.

The companion [observability change](../2026-10-02-add-platform-observability/design.md) owns engine observations, server heartbeat records, and the internal operational read model. This change owns administrator authorization, HTTP contracts, request controls, and presentation. Neither the UI nor the request lifecycle depends on Prometheus or Grafana availability.

## Goals / Non-Goals

**Goals:** make administration consistent across HA replicas, preserve durable request semantics during manual operations, and connect historical requests to explicitly timestamped live observations.

**Non-Goals:** browser-created backend configurations, arbitrary SQL or code execution, a Spark UI proxy, ingress provisioning, Spark History Server installation, a full dataset viewer, distributed tracing, or live log aggregation.

## Decisions

### 1. Embedded frontend and navigation

Use React with TypeScript, Vite, shadcn/ui, Tailwind CSS, and TanStack Query. Keep source in `web/`, lock frontend dependencies, and embed production assets in the Go binary. Node is a build/development dependency. Reuse the existing server image and HTTP service. This fits interactive JSON editing and refreshable tables while avoiding a separately operated frontend service. A Go-template UI would reduce build dependencies but require more bespoke client-state handling for this scope.

Serve the application under `/ui/`, with deep-link fallback confined to that prefix; unknown API paths must remain JSON errors. Cache content-hashed assets immutably and revalidate the entry document. Serve runtime configuration separately so one build works with different issuers and public URLs. Set `ui.enabled: false` by default; disabling it disables its session and administrative HTTP routes as well.

Routes:

- `/ui/contexts`: active contexts by default; owner, logical name, engine, state, and include-stopped filters.
- `/ui/contexts/{contextId}`: version overview, clients, instances, requests, and configuration tabs. A logical-context group lists versions and their demand independently.
- `/ui/requests`: global filters by owner, context/name/version, state, and submission time; stable cursor pagination.
- `/ui/requests/{jobId}`: immutable input, progress, attempt history, diagnostics, provenance, cancellation, and results.
- `/ui/requests/new?sourceJobId=...`: shared create/replay form; source is optional.
- `/ui/servers`: server heartbeat, readiness, background-loop status, and dependency observations from the same internal read model.

Use a compact navigation sidebar, tables, explicit empty/error/stale states, and keyboard-accessible controls. Poll visible active views every five seconds; stop polling when hidden or unauthenticated, back off on failures, and invalidate affected queries after mutations. Historical terminal details do not need continuous polling. Show last observation time; a refresh failure must not look like an empty healthy system.

### 2. Explicit administrator access

Introduce a separate `/v1/admin/` authorization boundary. Ordinary SDK endpoints retain their current owner scope even when the caller also has an admin role. The admin API obtains target ownership from the selected persisted context/job, never from a trusted-looking client-supplied owner string.

Support `viewer` (cross-owner inspection, result access) and `operator` (viewer plus create/replay/cancel). Both are administrator roles; this is not a tenant-user portal. Configure mappings under `security.admin` using issuer-bound subject allowlists and/or an explicit validated OIDC claim path with allowed values, including nested group/role claims. Default mappings grant nothing. Re-evaluate local mappings on every request. Record the actor independently from the target's existing owner.

Browser login uses the existing trusted issuer, Authorization Code with PKCE, state and nonce validation, and a server-side callback. `ui.oidc.clientId` is the browser application's own audience, distinct from the SDK API audience. Validate ID-token issuer, signature, expiry, audience, and nonce before using its claims. A confidential client can supply credentials through `ui.oidc.secretRef` and `clientSecretKey`. The UI's public URL and callback are explicitly configured, not derived from untrusted forwarding headers.

Use opaque, random, HttpOnly session cookies and hashed session identifiers in PostgreSQL; server replicas share sessions and short-lived login transactions. Cookies use SameSite=Lax and Secure for HTTPS. Expire sessions no later than the validated identity token, with an additional configured maximum. Do not retain provider refresh tokens; reauthentication uses the provider's session. Require same-origin/CSRF validation on cookie-authenticated mutations and logout. Avoid browser-local token storage. A SPA public-client flow was considered, but shared server sessions keep credentials and claim validation in the Go authorization boundary.

Respect the master switch: with `security.enabled=false`, an explicitly enabled UI has anonymous operator access and actor attribution `anonymous`. With security enabled but public OIDC disabled, enabling the admin UI is a configuration error until OIDC is configured; anonymous public SDK access must not silently become administrative access. Existing installations with the UI disabled continue to support all current security modes.

### 3. Read APIs and engine detail

Add paginated `/v1/admin/contexts`, context details, `/clients`, `/instances`, `/v1/admin/servers`, global jobs, job details, attempts, result metadata, bounded previews, and paginated parts. Publish the exact contracts in OpenAPI before implementing the frontend against them. Pagination defaults to 50 and caps at 200; cursors bind their ordering and filters. Use `(submittedAt,id)` or `(createdAt,id)` ordering rather than random-ID chronology. Requests include owner and logical-context identity for administration.

Delegate live observation to the companion read model. DTOs distinguish desired configuration, durable state, observed engine state, and observation freshness. Include connected-client counts and last-seen/expiry details without turning a page view into a lease. Explain lifetime using active clients, queued/running/retrying accepted work, and scaling/drain conditions.

Spark detail includes one instance row per driver, application/pod identity, worker registration/readiness, executor counts/states, configured CPU/heap/overhead/allocation settings, and observed pod resource requests/limits when available. Observed usage requires an external metrics source and is not invented from limits. Other engines receive generic instance detail and only their supported extensions.

Use the validated `spark-ui` link supplied by the observation model from the current SparkApplication's `status.driverInfo.webUIIngressAddress`. Open it in a separate tab. No ingress means “No published Spark UI URL” plus available service/port details. Missing/dead/replaced drivers never inherit an old incarnation's live link. Linha does not proxy the UI or apply its session authentication to the external ingress; that ingress keeps its own access policy.

### 4. Create and edit/replay share one acceptance path

The create form selects an existing context/version, then a handler and contract version, JSON payload, bounded retry policy, and optional new deadline. Advertised worker capabilities offer suggestions but do not masquerade as a business-parameter schema. Server validation remains authoritative.

Replay prefills this form from a durable source job. The default target is its exact context version. The operator can select another version of the same owner's logical context and edit parameters; a replay cannot silently move data to a different owner's context. Fresh creation can select any context authorized to the administrator. The target context determines owner and result policy. Old deadlines and idempotency keys are not copied. A new deadline must be in the future. Changed engine/result configuration is shown before submission.

Add `POST /v1/admin/contexts/{contextId}/jobs` with the ordinary JSON request fields plus optional `replayedFrom` and explicit `activateIfStopped`. Each deliberate submission obtains one new idempotency key, retained for retries/double-clicks of that submission. The server returns 202 only after committing the job, policy snapshot, provenance, and accepted audit entry. If the response is lost, retrying the same envelope/key returns the same ID; conflicting edits under that key return conflict. Starting a separate replay creates a fresh key and ID. The source job and result references never change, and new results use the new job/attempt allocation paths.

Administrative creation is an explicit authorized alternative to SDK lease-based submission. Under the existing context lock, commit the accepted job and context wake-up/reconcile request together. A stopped or draining version requires `activateIfStopped=true`; otherwise return an actionable conflict. Accepted queued/running/retrying work itself keeps the version alive, including replacement capacity, until terminal resolution. No synthetic browser lease is needed. A version with no remaining client demand drains after accepted work finishes, regardless of minimum drivers. A browser closing, administrator logout, or accepting-server crash does not abandon acknowledged work.

This approach reuses accepted-work lifetime rather than adding indefinite dashboard leases. Automatic retry remains opt-in and bounded; a manual replay intentionally creates another execution and does not promise exactly-once business side effects.

### 5. Cancellation and audit

Add an administrative cancel endpoint using the same transaction/ordering as ordinary cancellation. QUEUED/RETRYING work can become CANCELLED immediately; RUNNING transitions to CANCELLING until worker termination or lease recovery. Show that state accurately. Never kill a shared driver to cancel one request. A completion committed first remains successful; cancellation committed first fences success. Duplicate cancellation is harmless, and stale attempts cannot publish results.

Persist audit records for accepted administrative create/replay/cancel operations with actor, action, target owner/context/job, source job where applicable, timestamp, and outcome. Write the accepted audit/provenance record in the same transaction as the mutation; rollbacks leave no claimed acceptance. Retried idempotent submissions do not duplicate accepted audit entries. Failed authorization is logged without payloads or credentials. Audit history survives application restarts and is visible on request details; all mutations remain useful without a log aggregation service.

### 6. Primitive result access

Return published locations resolved from committed allocations and provider configuration: absolute shared-filesystem paths for local results and `s3://bucket/key` URIs for S3. Never expose delegated credentials, signed write URLs, staging locations, or incomplete attempt outputs. A URI is metadata, not proof that a user's workstation can open it.

JSON results get an explicit bounded preview (default 64 KiB, hard maximum 1 MiB), with truncation/invalid-content state. Files show location, name, size, content type, checksum, and retention status. Datasets show format, manifest, totals, and paginated published parts. Download actions reuse authorized result streaming; escaped text previews cannot execute result-supplied HTML. Metadata remains useful after context shutdown and clearly marks expired/missing payloads.

### 7. Coordination and delivery boundaries

Implement observability's operational read model and fenced engine snapshots first. UI work can then use those repository interfaces while metrics/dashboard/Helm monitoring proceed separately. Observability owns generic snapshot migrations; this change owns session/audit/provenance migrations, admin routes, and frontend assets. Coordinate sequential schema versions when both changes land; do not independently assign the same next migration number.

The React/Vite approach and polling behavior are supported by the [React build guidance](https://react.dev/learn/build-a-react-app-from-scratch), [shadcn Vite setup](https://ui.shadcn.com/docs/installation/vite), and [TanStack Query polling documentation](https://tanstack.com/query/latest/docs/framework/react/guides/polling). Exact compatible dependency versions are pinned during implementation.

## Risks / Trade-offs

- Cross-owner operations are more powerful than SDK access → isolate admin routes, validate roles server-side, retain original ownership, and test forged/non-admin credentials.
- Replaying on a different version can change behavior → show the selected version/result policy, keep provenance, and require explicit activation of stopped versions.
- OIDC session persistence adds schema and operational configuration → reuse PostgreSQL, bound session/login lifetimes, and test callbacks/requests through different replicas.
- Live Spark metadata can lag or disappear → surface timestamps/unavailable states and retain durable attempts independently.
- Large histories/results can overload the UI → server-side filtering, indexed cursor pagination, capped previews, and paginated dataset parts.

## Migration Plan

1. Land the companion read-model contracts and its schema migrations; extend them additively for admin metadata.
2. Add coordinated session/audit/provenance migrations without rewriting existing ownership, versions, or result paths. Existing requests have no replay provenance and display “SDK/legacy”.
3. Ship the image with UI disabled by default; document OIDC client, admin mappings, public URL, and Helm examples before enabling it.
4. Verify a two-replica rollout, cross-replica login, interrupted acceptance, replay/cancel races, and all-server restart while jobs run.
5. Roll back exposure by disabling the UI. Do not automatically downgrade the database or run a binary that rejects the newer schema.
