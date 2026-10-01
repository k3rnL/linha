## Why

METOC needs to accept processing requests quickly and preserve their IDs, ownership,
status, and outputs independently of Spark sessions or service restarts. Linha will
provide a reusable job platform with durable request management and managed engines,
starting with Spark, without coupling its request API to a particular compute engine.

## What Changes

- Establish three logical modules: Scala/JVM client SDK, Go job server, and Scala/JVM worker SDK.
- Expose a language-neutral JSON request/result protocol as the base API; build Scala/JVM
  serialization, typed entrypoints, and job handles as wrappers over that protocol.
- Provide a `LinhaContext` obtained by ensuring a managed backend with a caller-specified
  image, engine settings, and result-storage policy. Scala entrypoints contain their
  parameters and an execute method receiving a job context with engine access and progress.
- Persist requests before acknowledging stable IDs; support owner-scoped listing,
  individual status, cancellation, and typed result retrieval.
- Separate durable request scheduling from engine-specific provisioning and lifecycle control.
- Embed the worker runtime in user images to support manual JSON dispatch or registered
  typed entrypoints, report progress/errors, renew leases, and publish durable results.
- Store result payloads in caller-managed local filesystems or S3, selected during ensure;
  retain request state and result metadata in PostgreSQL. Support typed JSON values,
  managed file/stream writers, and distributed Spark dataset output.
- Support serializable S3 path templates using request/attempt identity, submission time,
  and artifact names, with reproducible resolution and collision-safe attempt locations.
- Preserve result visibility, ownership, limits, retention, and retrieval across restarts;
  document persistent shared mount requirements for local storage.
- Manage Spark driver minimum/maximum counts, concurrent request capacity,
  queue-based scaling, replacement of disappeared drivers, and optional native
  executor dynamic allocation.
- Deploy the job server redundantly with a Helm chart, durable database coordination,
  least-privilege worker credentials, migrations, and recovery observability.

## Capabilities

### New Capabilities

- `typed-client-sdk`: Raw JSON submission and typed Scala/JVM wrappers, durable handles, and `LinhaContext` interaction.
- `durable-request-management`: Owner-scoped request acceptance, queueing, state, attempts, idempotency, and recovery.
- `managed-backend-contexts`: Idempotent backend ensure, image/configuration identity, reconciliation, and engine adapter contracts.
- `worker-runtime`: Raw dispatch, typed entrypoint execution, job context, result helpers, capacity, leases, and completion protocol.
- `durable-result-storage`: Local/S3 providers, path templates, values/files/datasets, atomic visibility, retention, and restart retrieval.
- `managed-spark-engine`: Concurrent Spark execution, driver lifecycle/scaling, and executor allocation settings.
- `ha-deployment`: Multi-replica deployment, database/controller coordination, Helm, migrations, and operational checks.

### Modified Capabilities

None; Linha is a new project.

## Impact

The new repository will contain the Go server and JVM SDK build structures, versioned
HTTP/JSON contracts, PostgreSQL migrations, a Kubernetes Spark adapter, local/S3 result providers,
and a Helm chart. The first SDK bindings target Scala/JVM; Go SDKs and additional
engine adapters are deferred. User processing code is packaged in trusted worker
images and adapted to the SDK interfaces. Existing METOC code and deployments are
not migrated by this change. The proposed foundation replaces no existing Linha APIs.
