## 1. Repository and build foundation

- [x] 1.1 Create the Linha repository, Go module, three logical module directories, OpenSpec configuration, and buildable version-command scaffold; verify Go formatting, compilation, and vet.
- [x] 1.2 Establish supported Go and JVM toolchain baselines and pin a tested Spark/Scala/JDK compatibility matrix, recording METOC compatibility constraints.
- [x] 1.3 Add the JVM SDK build structure, shared protocol types/fixtures, and independent client/worker artifact builds.
- [x] 1.4 Add CI checks for Go, JVM SDKs, protocol conformance, and strict OpenSpec validation without requiring a production cluster.

## 2. Transport and domain contracts

- [x] 2.1 Define OpenAPI base contracts for ensure, raw handler/version/JSON submission, list/status/cancel, raw values and artifact/manifest access; specify stable errors, pagination, schemas, local/S3 policies, and template configuration.
- [x] 2.2 Define authenticated worker registration, raw/typed handler capabilities, claims, heartbeat/renewal, drain, progress/failure, output allocation/authorization, and idempotent manifest completion contracts.
- [x] 2.3 Implement engine-independent Go models and ports for contexts, requests, attempts, ownership, engine capabilities, repositories, and result stores.
- [x] 2.4 Implement a fake engine adapter and conformance fixtures demonstrating that the request models contain no required Spark-specific fields.
- [x] 2.5 Define canonical request/spec hashing and shared fixtures for raw/typed equivalence, idempotency, image identity, schema compatibility, and persisted result policy across Go and JVM encoders.

- [x] 2.6 Define version-1 path-template grammar, supported placeholders/date formats, required identity segments, artifact naming, and wire fixtures independent of executable lambdas.

## 3. Persistent request management and acceptance

- [x] 3.1 Add PostgreSQL connections and exclusively coordinated migrations for contexts, desired instances, workers, JSON jobs, submission timestamps, attempts/progress, output allocations, and result metadata/manifests.
- [x] 3.2 Implement transactional request acceptance, policy snapshots, stable public IDs, scoped idempotency uniqueness, and changed-payload conflicts.
- [x] 3.3 Implement authenticated-principal handling, ownership filters, cursor-based listing, and individual status retrieval against persisted records.
- [x] 3.4 Implement exclusive capacity-aware claims with durable attempts, fencing tokens, database-time leases, and compatible-handler routing.
- [x] 3.5 Implement the authoritative request/attempt state machine, cancellation intent, terminal-state ordering, and repeated-message handling.
- [x] 3.6 Verify acceptance response loss, concurrent duplicate submissions, competing claims, cross-owner access, and persistence-unavailable rejection using PostgreSQL integration tests.

## 4. JVM SDKs and the first complete request

- [x] 4.1 Implement the base JSON client and Scala executable-entrypoint wrapper, result kind/schema descriptors, derived/explicit JSON codecs, LinhaContext references, and typed JobHandle restoration.
- [x] 4.2 Implement asynchronous ensure, raw/typed submit, polling/listing, cancellation, and retrieval with refreshable credentials and stable idempotency keys across transport retries.
- [x] 4.3 Implement the worker JVM entrypoint, manual JSON dispatch, registered typed reconstruction/execute wrapper, capability advertisement, initialization/readiness, and bounded slots.
- [x] 4.4 Implement generic LinhaJobContext with identity, progress, cancellation and result access; add long-poll claims, persisted progress/failure reporting, independent lease renewal, drain, and cooperative cancellation.
- [x] 4.5 Implement local-provider JSON encoding, persistent output allocation, containment checks, write completion, and atomic result-reference/success publication with client schema validation.
- [x] 4.6 Add raw JSON and typed Scala entrypoint examples through a non-Spark test backend; verify both use the same durable lifecycle, idempotency behavior, and result protocol.
- [x] 4.7 Restart client/server/worker processes after the local-result example completes with storage retained; verify status/progress and original result retrieval by public ID.

- [x] 4.8 Implement managed local-path and output-stream helpers, FileResult/DatasetResult contracts, unique artifact allocation, resource cleanup, and staging quotas; verify callback failure never publishes partial success.

## 5. Failure recovery and result storage

- [x] 5.1 Implement lease/deadline recovery, durable interruption reasons, opt-in bounded retry/backoff, and attempt history with unchanged public IDs.
- [x] 5.2 Enforce stale-incarnation/fencing checks on progress, renewal, failure, and completion, including cancellation versus completion races.
- [x] 5.3 Add authorized S3 destination settings, attempt-scoped uploads, integrity validation, immutable references/manifests, conditional publication, and provider errors without database-payload fallback.
- [x] 5.4 Implement provider-independent typed-value, file-stream, and incremental dataset-manifest/part retrieval, fresh authorized download access, limits, and storage-outage errors.
- [x] 5.5 Implement result retention, RESULT_EXPIRED metadata, abandoned file/object/multipart cleanup with active-allocation and committed-manifest checks, and job-tombstone retention.
- [x] 5.6 Test incomplete local/S3 writes, lost completion acknowledgements, stale successful workers, retry exhaustion, size/staging-limit failures, expiry, and restart retrieval for both providers.

- [x] 5.7 Implement S3 template validation and deterministic UTC expansion in Go/JVM, authorization/containment and provider-length checks, and persisted allocations before writes.
- [x] 5.8 Verify template fixtures across timezone/day boundaries, unchanged submission time on retry, lost allocation responses, concurrent/stale attempts, duplicate names, traversal/encoded traversal, reserved names, invalid formats and missing identity segments.

## 6. Context identity and managed lifecycle

- [x] 6.1 Implement owned idempotent context ensure and canonical immutable-spec conflicts including local/S3 destinations and template configuration; expose readiness independently.
- [x] 6.2 Resolve and retain image digests using authorized registry configuration; validate image/resource policy and adapter-specific configuration before launch.
- [x] 6.3 Implement persisted desired instance records and per-context controller coordination with ownership epochs, idempotent resource identities, and reconciliation after ambiguous side effects.
- [x] 6.4 Add backend conditions, startup retry/backoff, readiness gating including required result access, worker-incarnation tracking, and drain/stop through the generic adapter.
- [x] 6.5 Verify concurrent ensure calls, controller failure after resource creation, client disconnect, immutable image replacement, and automatic replacement using the fake adapter.

## 7. Spark worker and Kubernetes adapter

- [x] 7.1 Build a documented example image with compatible Spark/JVM libraries, the Linha runtime, raw/typed entrypoints, storage connectors, and user dependencies.
- [x] 7.2 Implement Kubernetes driver Pod/Service provisioning with distinct instance/incarnation identity, routable driver networking, executor ownership, and resource UID preconditions.
- [x] 7.3 Initialize one classic SparkContext per driver and expose request-scoped SparkSession capabilities through LinhaJobContext with bounded concurrent execution.
- [x] 7.4 Implement FAIR request pools, per-attempt Spark job groups, cancellation, execution-thread property/session cleanup, and request-owned resource cleanup helpers.
- [x] 7.5 Implement driver observation and reconciliation for disappeared/failed Pods, startup failures, readiness, and cleanup of obsolete managed resources.
- [x] 7.6 Run two concurrent sample requests on one real driver; cancel one and verify that the other can complete with its own durable result.
- [x] 7.7 Delete a required driver while clients are disconnected; verify automatic replacement and independent failed/retried request outcomes according to policy.

- [x] 7.8 Implement managed distributed Parquet output and dataset manifests over assigned local/S3 locations, with compatible committers and validated driver/executor access without global credential rebinding.
- [x] 7.9 Test partitioned multi-file output on both providers, executor failures before publication, missing shared mounts, manifest streaming, and distinct paths under simultaneous requests/retries.

## 8. Spark scaling and resource policy

- [x] 8.1 Implement validation and translation of driver min/max, per-driver concurrency, queue/wait thresholds, cooldown, resource ceilings, and drain policy.
- [x] 8.2 Implement queue/capacity-driven desired-driver scaling, including scale from zero, pending-startup accounting, maximum enforcement, and constrained-cluster conditions.
- [x] 8.3 Implement idle excess-driver drain/scale-down while preserving minimum capacity and avoiding termination of active work.
- [x] 8.4 Implement static executor settings and optional Spark dynamic allocation with validated min/initial/max values, shuffle tracking, and applicable quotas.
- [x] 8.5 Validate driver scale-up/down and executor growth/shrink independently under controlled load; measure queue wait, request concurrency, and retained cache/broadcast resources.

## 9. Identity, HA, and Helm

- [x] 9.1 Implement configurable OIDC validation and stable owner/tenant mapping, reject caller-supplied ownership, and exercise all public endpoints against cross-owner requests.
- [x] 9.2 Implement projected Kubernetes worker-identity verification, Pod/context-incarnation authorization, credential renewal, and restricted result-upload permissions.
- [x] 9.3 Implement server graceful shutdown, dependency-aware readiness, independent liveness, and continued worker communication through another replica.
- [x] 9.4 Add correlated structured logs and metrics for queue age/depth, attempts/leases, result publication, backend conditions, worker capacity, and provisioning failures.
- [x] 9.5 Create the Helm chart with multiple replicas, Service/probes, migration coordination, RBAC, Secrets, disruption/spread controls, and references to caller-managed local volumes/mounts for servers and relevant workers/executors.
- [x] 9.6 Lint/render chart configurations and install in a disposable cluster with persistent PostgreSQL, S3-compatible storage, and a shared local-result volume; test replica replacement and missing-mount reporting.
- [x] 9.7 Document database/store HA prerequisites, caller-managed mount requirements, scoped destination/credential policy, path templates, retention, and compatible upgrade/rollback.

## 10. Acceptance and release readiness

- [x] 10.1 Exercise full server/controller/worker process restarts with completed, queued, running, retrying, and cancelling jobs; verify stable IDs, outcomes, and restored desired capacity.
- [x] 10.2 Inject database outages and worker partitions beyond lease expiry; verify no false acceptance, stale completion rejection, and policy-based recovery.
- [x] 10.3 Exercise concurrent controllers and worker claims across multiple replicas; verify no duplicate logical backend instances and no multiple current attempts per job.
- [x] 10.4 Exercise owner-scoped retrieval after URL expiry and driver replacement; verify local/S3 results and resolved paths survive process restarts and different client/worker timezones.
- [x] 10.5 Publish a walkthrough covering manual JSON dispatch, Livy-style typed execute(ctx), value/file/stream/Spark output, local/S3 ensure settings, custom paths, polling, restart retrieval, and driver replacement.
- [x] 10.6 Record measured defaults/limits and complete the capability-scenario acceptance checklist before describing Linha as operational or proposing METOC migration.

## 11. Deployment configuration revision

- [x] 11.1 Replace the shared existingSecret Helm value with PostgreSQL connection settings and configurable credential Secret keys; verify passwords with special characters and custom key rendering.
- [x] 11.2 Remove deployment mode in favor of independent security/OIDC switches; verify credential-free client/worker execution and restart retrieval, and retain authenticated owner isolation.
- [x] 11.3 Default to namespace-only RBAC with signed projected-token validation and live identity checks; retain optional TokenReview and verify no cluster-scoped grants/calls in namespace-only operation.
- [x] 11.4 Update deployment documentation, raw protocol, SDK examples and OpenSpec; validate Helm configuration combinations, Go checks, JVM tests and the restart smoke test.

Completion evidence: [acceptance matrix](../../../../docs/acceptance.md), schema v6, final image checks and all scenario mappings (2026-09-30).
