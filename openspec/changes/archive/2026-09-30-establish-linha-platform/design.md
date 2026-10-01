## Context

See [proposal.md](proposal.md) for motivation and capability scope. The repository
currently contains a buildable Go command scaffold. The user selected a Go server
and Scala/JVM SDKs. Existing METOC handlers use classic Spark APIs, filesystem I/O,
NetCDF libraries, broadcasts, and cached DataFrames. Executable handler code belongs
in a user-supplied image; the server must remain independent of those libraries.

## Goals / Non-Goals

**Goals:**
- Give clients stable, owned job records independent of process and engine lifetimes.
- Make backend ensure safe under concurrent callers and controller failures.
- Support typed entrypoints across a language-neutral protocol.
- Reuse Spark drivers with bounded request concurrency and managed resource bounds.
- Make acceptance, result publication, retries, and restart recovery testable.

**Non-Goals:**
- Arbitrary JVM object/closure serialization or downloading executable code per request.
- Exactly-once execution of user side effects or resuming a dead Spark driver's memory.
- Go SDK bindings, additional production engines, multi-cluster scheduling, billing,
  and transparent worker-image upgrades in the first release.
- Migrating METOC as part of this foundation change.

## Decisions

### 1. Three logical modules and explicit adapter boundaries

The client SDK owns `LinhaContext`, typed entrypoints, `JobHandle[Result]`, polling,
and authorized result access. The Go job server owns the request state machine,
queue, attempts, results, and backend reconciliation. The worker SDK owns the JVM
process entrypoint, handler registry, work protocol, and engine execution integration.

Inside the server, request management depends on repository, result-store, and
backend-capability interfaces. It does not import Spark-specific job types. An
engine adapter validates engine configuration, describes capabilities, reconciles
instances, observes readiness, and drains/stops instances. The worker-side engine
integration provides the execution environment and cancellation/resource cleanup.
These are internal plugin interfaces in v1, not dynamically loaded arbitrary Go code.
A fake adapter must exercise the complete request lifecycle in tests.

Alternative: one generic adapter method accepting arbitrary engine JSON everywhere.
Rejected because it loses validation and leaks engine behavior into request management.
Engine settings instead have a registered, versioned schema and typed adapter models.

Repository layout: `server/`, `sdk/client-jvm/`, `sdk/worker-jvm/`, shared transport
contracts under `api/`, and the chart under `deploy/helm/linha/`.

### 2. A public JSON protocol with optional typed entrypoint wrappers

Versioned HTTP/JSON is the base contract for client and worker operations, described
by OpenAPI. A user can submit an envelope without Scala classes or generated codecs:

```json
{
  "handler": "analytics.count-rows",
  "version": 1,
  "payload": {"input": "s3a://analytics-input/events/"},
  "idempotencyKey": "request-123"
}
```

The Go server validates the envelope and persists the payload without decoding an
application-specific JVM type. A base worker callback receives that envelope and a
job context; user code matches the handler/version and calls its business logic.
Raw workers advertise supported handler/version pairs and result descriptors too,
so compatibility routing does not depend on the typed SDK. The base result protocol
supports JSON values and file/dataset descriptors with the same publication rules.

The Scala layer wraps these base operations. Keep the METOC Livy client's useful
shape: a case-class entrypoint contains parameters and `execute(context)` behavior.
The client serializes only constructor data with a registered JSON codec; the worker
reconstructs an allowlisted entrypoint already installed in its image and invokes it.
Stable handler IDs are independent of Scala class names. No source code, closures,
classloaders, or Kryo/Java object graphs cross the wire. Definitions may be shared
between application and worker artifacts; Spark implementation dependencies remain
provided/runtime dependencies where appropriate. A separate handler implementation
is optional for users wanting to separate contracts and implementation further.

Illustrative SDK shape; these APIs are planned, not implemented:

```scala
case class RowsCount(value: Long)

case class CountRows(input: String) extends SparkEntrypoint[RowsCount] {
  override def execute(ctx: LinhaJobContext[Spark]): RowsCount = {
    ctx.progress(0.1, "Reading dataset")
    val count = ctx.spark.read.parquet(input).count()
    ctx.progress(0.9, "Counting complete")
    RowsCount(count)
  }
}

object CountRows {
  implicit val codec: EntrypointCodec[CountRows, RowsCount] =
    EntrypointCodec.derived(id = "analytics.count-rows", version = 1)
}

// Worker bootstrap: registered codecs supply the raw dispatch wrapper.
LinhaWorker.spark(args).register(CountRows.codec).run()

// Client: asynchronous transport, as in the METOC Livy client.
val accepted: Future[JobHandle[RowsCount]] = for {
  context <- client.ensureBackend("analytics-v1", backendSpec)
  job <- context.submit(CountRows("s3a://analytics-input/events/"),
                        idempotencyKey = "request-123")
} yield job

// A fresh client can restore a handle by ID and expected descriptor.
val job = client.job(savedJobId, CountRows.codec)
val status: Future[JobStatus] = job.status()
val result: Future[RowsCount] = job.result() // Once successful.
```

The entrypoint descriptor defines request/result schema identities and versions,
result kind, and codecs, with derivation for ordinary case classes and explicit
codecs for custom types. The concise version argument defaults both schema versions;
independent schema versions are representable in the descriptor. Typed submission,
restoration, polling, and retrieval delegate to the raw API with identical ownership,
idempotency, and errors. Retrieval checks schema compatibility before decoding.
The raw API also supports polling, listing, cancellation, and result download.
A pending result lookup returns a not-ready outcome; it does not imply synchronous
execution. An explicit waiting helper can be layered on polling later.

`LinhaJobContext[E]` carries job ID, attempt ID, cancellation, progress, and managed
result helpers. `SparkEntrypoint[R]` specializes the generic entrypoint contract for
the Spark engine, and the Spark SDK exposes `ctx.spark` as its request-scoped session.
Other engines specialize the same context contract without Spark dependencies in the
base protocol. SDK helpers register request-owned caches/broadcasts for cleanup and
keep scheduling/cancellation identity on the actual Spark submission threads. A handler
must not stop the shared SparkContext. Progress is advisory, scoped to an attempt,
and persisted by the server; reporting 100 percent never commits job success.

Only compatible ready workers acquire requests. Unsupported handler/schema on a ready
backend produces a durable compatibility error; an unready backend keeps work queued
until readiness or deadline. HTTP/JSON is chosen over gRPC for interoperability and
inspection. A single base protocol prevents typed wrappers acquiring different queue,
result, or recovery semantics. Polling is sufficient for v1.

### 3. PostgreSQL is the source of truth and initial durable queue

Persist contexts, desired backend state, worker incarnations, requests, attempts,
progress, and result metadata in PostgreSQL. Result payloads use local or S3 storage. Request rows are the initial
queue. Transactional claims use row locking with skip-locked semantics and enforce
both context admission policy and worker capacity. This avoids a database/broker
split-write during acceptance. A broker can be added later as a notification channel.

Suggested records:

| Record | Durable identity and essential fields |
| --- | --- |
| Context | ID, tenant/owner, logical key, canonical spec hash, resolved image, lifecycle state, desired instance count, last condition |
| Worker | Context, instance slot, incarnation, readiness, handler manifest, capacity, last heartbeat, drain state |
| Job | Public ID, tenant/owner, context ID, entrypoint/schema, input, idempotency key/hash, state, policy snapshot, timestamps/deadline |
| Attempt | Job ID, attempt number, worker incarnation, fencing token, lease expiry, error/progress, engine references |
| Result | Job ID, committed attempt, result kind/schema, destination identity, resolved paths/manifest, checksums, sizes, retention |
| Output allocation | Job/attempt, logical artifact name, provider/template version, resolved path or dataset prefix, publication state |

The public ID is generated by the server and committed with the request before
acknowledgement. An idempotency key is unique per principal and context. Repeating
an identical logical request returns its original ID; changed content returns a
conflict. The hash includes the effective result-policy snapshot and retry settings. The SDK reuses a
key for retries of one call; applications provide a stable key across their own restarts.
No accepted request is represented only in a server process or a notification queue.

Proposed public endpoints:
- `POST /v1/contexts:ensure` and `GET /v1/contexts/{id}`.
- `POST /v1/contexts/{id}/jobs`, returning `202` and a public job ID.
- `GET /v1/jobs` with owner-scoped cursor pagination and context/state filters.
- `GET /v1/jobs/{id}`, `GET /v1/jobs/{id}/result`, and `POST /v1/jobs/{id}:cancel`.

No endpoint needs a live engine to read persisted completed job status or results.
Acceptance and worker claims fail closed while persistence is unavailable.

### 4. Managed contexts are desired state, not a client-owned process

Ensure includes engine type/version, an image reference, handler compatibility
expectations, validated engine configuration, concurrency/scaling policy, and default
result/retry/retention policies. Result settings include the local root or S3 destination,
versioned path template, limits, and required mounts/credentials. The server authenticates the caller, checks resource
and image policy, and atomically ensures a context keyed by tenant, owner, and logical
name. Identical specifications return the same context even when it is STARTING.
A conflicting spec under the same key returns an explicit conflict, including a changed
result destination or path template. A template is persisted as data, never evaluated
against the current client clock during ensure.

Context specifications are immutable in v1. Code/configuration changes use a new
logical context version; autoscaled instance counts are mutable controller state,
not a change to the client's specification. Images are resolved and recorded by
digest before workers are launched; the same context never silently picks up a new
tag target. LinhaContext is a serializable reference to server-owned state, not an
exclusive lease. Closing a client handle does not stop its backend.

Ensure returns after persistence. Readiness is observed separately, allowing jobs
to queue during provisioning. Reconciliation runs without a connected client and
maintains the requested minimum; deleting a driver or restarting the server cannot
remove the recorded desired state. A failed image pull or invalid runtime produces
an observable context condition with bounded retries, not a false READY state.

Multiple server replicas can participate. Per-context reconciliation leases with
monotonic epochs coordinate decisions. Resource identities derive from context and
persisted instance records. Kubernetes operations are idempotent and use resource
versions/UID preconditions; a controller must recheck ownership before side effects.
Late creates are detected and reconciled against the durable desired instance set.
Thus recovery converges without treating a lease as a distributed transaction with
Kubernetes. Rolling image changes and user-facing context deletion are deferred;
scale-down already implements draining.

### 5. Worker claims, bounded execution, and attempt fencing

The image starts Linha's JVM worker entrypoint, loads user registrations, initializes
the engine environment, authenticates to the server, and reports readiness and
capacity. Workers long-poll for requests through any server replica and claim only
available local slots. All claims and capacity accounting are durable; an unstarted
claim expires as an interrupted attempt instead of remaining permanently RUNNING.

Separate worker-instance heartbeats from per-attempt lease renewal. Leases use database
time. Every progress, renewal, failure, and completion operation includes its worker
incarnation and fencing token; expired/replaced attempts cannot mutate current state.
Workers stop requesting work and attempt cooperative cancellation when they cannot
renew before their lease deadline. This cannot revoke arbitrary external side effects
from a partitioned process; the result commit protocol and retry contract address that.

The public state machine is:

```text
QUEUED -> RUNNING -> SUCCEEDED
                 -> FAILED
                 -> RETRYING -> RUNNING
QUEUED / RETRYING -> CANCELLED
RUNNING -> CANCELLING -> CANCELLED
```

Attempts have separate terminal outcomes including LOST and TIMED_OUT. Jobs retain
attempt history. Lease expiry/deadline recovery selects RETRYING or FAILED according
to a persisted, bounded policy and backoff. Default maxAttempts is 1; users explicitly
opt into automatic retries for retry-safe handlers. Non-retryable schema/validation
errors fail immediately. Driver replacement happens independently of job retry policy.
A missing driver is restarted even when the interrupted request will not be retried.

Cancellation first records intent and fences completion from the current attempt.
If completion committed before cancellation, success remains terminal; otherwise late
completion cannot turn cancellation into success. Cancellation stops future claims,
reaches the engine adapter/handler, and finishes after acknowledgment or ownership
expiry. The same public ID remains visible for every terminal outcome.

### 6. Result shapes, local/S3 providers, and managed publication

Result storage is selected during context ensure and snapshotted on each accepted
job. The first providers are LOCAL and S3. PostgreSQL stores status and metadata,
not result payloads. Context identity includes provider configuration and path policy;
changing these settings uses a new context name in v1. Direct per-job destination
overrides are deferred; the job's snapshot is authoritative throughout its lifetime.
All result forms share size/retention policies and the same completion protocol.
Exceeding a configured limit fails explicitly without silently switching providers.

Result types describe content rather than storage. Ordinary values such as `RowsCount`
use the result codec and become JSON bytes. `FileResult` describes a named byte stream
or file; `DatasetResult` describes partitioned output with format/schema and a manifest.
Avoid `S3Result[T]`: provider selection must not require rewriting the business class
or changing the client result type. The base worker can return raw JSON or equivalent
artifact descriptors, and the typed layer delegates to those same primitives.

Illustrative worker helpers:

```scala
// For libraries requiring a seekable local file, such as NetCDF writers.
val file: FileResult = ctx.results.file("extraction.nc", "application/x-netcdf") {
  path => MyNetcdfWriter.write(ctx.spark, input, path)
}

// For a library accepting an OutputStream.
val csv: FileResult = ctx.results.stream("summary.csv", "text/csv") {
  output => MyCsvWriter.write(summary, output)
}

// Executes a distributed write, never collect() of the DataFrame on the driver.
val dataset: DatasetResult = ctx.results.parquet(
  dataframe = events, name = "events", partitionBy = Seq("year"))
```

Helpers allocate outputs, enforce unique logical names, close resources, finish writes,
and return attempt-bound descriptors. Returning such a descriptor from execute lets
the runtime finalize the job; merely allocating or writing an output does not publish it.
User callbacks must complete their writes before returning. Callback failures abort the
publication path and schedule cleanup. Generic stream/file helpers are engine-independent;
DataFrame writers are Spark integration helpers over the same output-allocation contract.
SDK-owned resources are cleaned up without clearing another request's caches. Staging
quotas apply to seekable files and any provider-buffered stream data.

**Local provider.** The user configures and mounts a persistent filesystem root, for
example `ResultStorage.Local(root = "/mnt/linha-results")`. Linha allocates a relative
`context/request/attempt/artifact` namespace under that root, writes raw JSON/file bytes
or a dataset tree, and records its descriptor. Seekable output stages on that filesystem
where practical, with close/flush and appropriate durable completion before publication.
The user supplies mounts/permissions and storage durability. Serving replicas and replacement
workers must resolve the same stored relative paths against the same underlying data;
Spark executors must share it for distributed output. A matching path on unrelated node
filesystems, a container layer, or an ephemeral volume does not meet restart/HA guarantees.
The chart/engine settings reference existing user-managed mounts; Linha does not provision
a shared filesystem. Root containment, symlinks, names, and credentials are validated.
Readiness reports missing mounts or access failures, never silently falls back to node disk.

**S3 provider.** A destination reference selects operator-configured bucket, endpoint,
credentials, and authorized prefix; a versioned path template selects keys within it.
File helpers stage locally when necessary and upload; stream helpers use supported
streaming/multipart writes; Spark helpers write to assigned S3A locations through tested
connectors/committers. Driver and executors must have access using validated credentials;
concurrent requests must not change process-global connector credentials. Ordinary uploads
are scoped to assigned keys and distributed writers to the smallest supported assigned
prefix. A connector/provider combination that cannot satisfy the configured scope is
rejected. No worker gets general job database access.

**Publication.** Before an output is written, persist its allocation and resolved destination
under the current attempt. Writers create immutable attempt-specific outputs; they do not
publish to a shared stable filename. After writers close and Spark task/job commit finishes,
the worker submits a manifest of the final outputs. Verify provider access, expected paths,
object/file existence, sizes, and integrity metadata, then atomically commit the manifest
reference and SUCCEEDED in PostgreSQL only while the attempt still owns the job. Large
manifests live in the result provider with a verified digest and a DB reference. This is
atomic visibility through the Linha API, not a filesystem/S3 transaction or a promise that
users with direct storage credentials cannot see staging. Never treat an S3 directory
rename as atomic. Checksums are explicit, not an assumption that an ETag is an MD5.

Repeated identical completion returns the committed receipt; conflicting completion fails.
A stale attempt may leave its own objects but cannot overwrite another attempt's namespace
or update the authoritative manifest. Abandoned outputs and multipart uploads are cleaned
only after checking active allocations/leases and retained committed manifests. A job can
fail after a write without exposing that write as its completed result.

Reads authorize the owner, use persisted references (never rerender templates), and stream
files or paginated dataset manifests/parts. JSON values are decoded by the client codec;
file/dataset results stay descriptors and do not load all bytes into the API process.
Signed URLs, when used for S3, are short-lived credentials refreshed after authorization.
Neither provider needs the original worker for retrieval. Missing/unavailable storage
returns a specific retrieval error while preserving recorded success. Payload expiry returns
RESULT_EXPIRED while the job tombstone remains within its own retention period. Backup,
shared mounts, and provider retention remain operational prerequisites.

### 6a. Serializable S3 path templates and deterministic variables

Choose a small declarative template for v1. Arbitrary Scala lambdas cannot be evaluated
by the Go server or recovered after the original client exits without shipping executable
code. A future Scala builder/macro can compile lambda-like syntax to this same template;
its closure must never be part of the wire protocol. The supported initial SDK API is:

```scala
val results = ResultStorage.S3(
  destination = "analytics-results", // Registered bucket/endpoint/credential policy.
  path = ResultPath.template(
    "exports/{submittedAt:yyyy/MM/dd}/{requestId}/{attemptId}/{file}"
  )
)
```

The wire value is ordinary configuration data:

```json
{
  "type": "s3",
  "destination": "analytics-results",
  "path": {
    "version": 1,
    "template": "exports/{submittedAt:yyyy/MM/dd}/{requestId}/{attemptId}/{file}"
  }
}
```

The template is relative to the destination's allowed bucket/prefix. Bucket and endpoint
are fixed by that destination, not interpolated from job data. Full URIs and credentials
are excluded from the template. For a destination rooted at `s3://bucket/blabla/`, a job
accepted on 2026-09-28 with request ID `job-123`, attempt ID `attempt-1`, and JSON output
`result.json` resolves to:

```text
s3://bucket/blabla/exports/2026/09/28/job-123/attempt-1/result.json
```

Version 1 placeholder meanings:

| Placeholder | Persisted value / resolution rule |
| --- | --- |
| `{contextId}` | Stable context ID, an optional organizational component |
| `{requestId}` | Public Linha job ID, not client idempotency key or Spark job ID |
| `{attemptId}` | Unique execution attempt ID; reused for retransmissions, changed on job retry |
| `{submittedAt}` | Server acceptance instant in UTC, rendered as `yyyyMMddTHHmmssSSSZ` |
| `{submittedAt:yyyy/MM/dd}` | The same persisted instant formatted with supported UTC tokens |
| `{file}` | Validated logical artifact-relative path; `result.json` for a JSON value, a supplied filename for a file, or dataset name plus part-relative path |

The formatting grammar supports only `yyyy`, `MM`, `dd`, `HH`, `mm`, `ss`, `SSS` tokens
and literal `/`, `-`, `_`, `T`, `Z` separators, consistently in Go and JVM fixtures.
Unknown placeholders, arbitrary expressions, unsupported format tokens, and malformed
braces fail ensure. There is no implicit JVM date formatter, system timezone, random
value, worker-start time, or call to now(). Literal braces are unsupported in v1.
Templates use POSIX `/` separators and are versioned/canonicalized for context identity.

`{requestId}` and `{attemptId}` must each appear exactly once as a whole path segment,
and `{file}` must appear once as the final suffix. The default template is
`{contextId}/{requestId}/{attemptId}/{file}`. Missing identity segments fail validation;
Linha does not secretly append them. These constraints make inter-job and inter-attempt
paths disjoint, including simultaneous late writers. A logical name can be allocated
only once per attempt; repeating the same allocation is idempotent, while incompatible
reuse or overlapping file/dataset names fails. Internal manifest names are reserved.

Artifact names are validated relative paths, never caller-selected absolute storage
locations. Reject empty/dot/parent segments, backslashes, control characters, traversal
or encoded traversal, and reserved internal names. Static template text obeys the same
containment rules. Percent encoding occurs once at the provider transport boundary;
no subsequent decoding can turn a validated name into a different path. Enforce final
provider key/path length limits and revalidate the resolved destination before granting
write access. Unsafe configuration fails ensure; an invalid runtime artifact name fails
allocation before any write is authorized. Templates cannot escape authorized prefixes.

Persist template/version in the context and job policy, then persist each resolved output
allocation before writes. Time and IDs are read from durable records. Identical completion
or allocation retries reuse the same location, even after midnight or process restarts.
A new execution attempt changes only attempt-dependent components, keeping the original
submission time. Retrieval and cleanup follow committed locations, not whichever template
or SDK version is currently installed. Dataset writers allocate a prefix once with `{file}`
as the dataset name and retain all committed partition/part paths under it; the template
does not force single-file output or a collection of distributed data on the driver.

### 7. Spark adapter and two independent scaling loops

The user image includes a compatible JVM/Spark runtime, Linha worker SDK, handler JARs,
and native dependencies. The first Kubernetes adapter manages one driver Pod per
persisted driver instance and its own network Service. The JVM entrypoint starts a
classic SparkContext inside the driver Pod in Kubernetes client mode; executors run
in their own Pods. This avoids requiring a Spark launcher/JVM in the Go server or a
separate Spark operator for v1. The adapter sets a routable driver host and the actual
driver Pod name so executor ownership/cleanup can follow the driver. Each new driver
incarnation has a distinct identity; services must never select two live incarnations.
The driver/executor image defaults to the requested image, with validated overrides
available only through the Spark configuration schema and deployment policy.

The worker invokes handlers directly, not through a Scala interpreter. It uses one
SparkContext per driver, a bounded request thread pool, request-scoped SparkSessions,
and job groups keyed by job/attempt for cancellation. Scheduling uses FAIR mode with
explicit request pools (or configured tenant pools with a documented inner policy).
Thread-local properties/active sessions are set and cleared on the actual submission
threads. Handlers leave SparkContext lifecycle to the worker. SparkSession isolation
shares caches and context; it is not security or failure isolation between tenants.
V1 contexts are owner-scoped. Audit handler libraries for concurrency safety.

Driver policy includes minDrivers, maxDrivers, maxConcurrentRequestsPerDriver,
scale-up queue thresholds, oldest-queued wait threshold, cooldown, and scale-down idle
time. Validate 0 <= minDrivers <= maxDrivers and maxDrivers >= 1. A backlog can start
a driver when minDrivers is zero. Readiness-eligible STARTING instances count toward
the maximum; failed/deleting slots are reconciled before replacements are admitted.
Constrained cluster capacity surfaces a condition instead of unbounded pending Pods.
Scaling considers occupied/free slots, pending startups, queue depth/age, and budget.
Drain idle excess drivers before termination; never terminate active work merely to
reach a smaller target. The controller continuously replaces lost drivers as needed
to satisfy desired capacity. Healthy workers keep running when a server replica fails.

Executor policy is separate: static instances or Spark dynamic allocation with validated
minimum, initial, maximum, idle timeouts, and shuffle tracking. Spark controls executors
within those bounds; Linha controls drivers. Cluster-wide quotas bound the product of
driver and executor limits. Keeping a driver alive with zero executors is supported but
incurs executor startup latency. Cached data can inhibit shrinkage: request-owned caches
and broadcasts must be released after actions finish, without clearing another request's
shared cache. In METOC, TrackService/TrackEnricher caches need explicit ownership.

Alternative: one Spark application per request. Deferred because reusable drivers are
an explicit requirement. Multiple drivers remain useful for isolation/driver bottlenecks,
not a prerequisite for concurrent Spark jobs.

Technical references: [Spark concurrent scheduling](https://spark.apache.org/docs/3.5.6/job-scheduling.html),
[SparkSession isolation](https://spark.apache.org/docs/3.5.6/api/scala/org/apache/spark/sql/SparkSession.html),
[Kubernetes client mode](https://spark.apache.org/docs/3.5.6/running-on-kubernetes.html#client-mode),
[dynamic allocation](https://spark.apache.org/docs/3.5.6/configuration.html#dynamic-allocation),
and [cloud storage commits](https://spark.apache.org/docs/3.5.6/cloud-integration.html).
The initial JVM build will establish and test a pinned Spark/Scala/JDK compatibility
matrix before adapter work; the existing METOC baseline is Spark 3.5.6 / Scala 2.12.20.

### 8. HA deployment, identity, and operations

The Helm chart defaults to at least two stateless server replicas behind a Service.
Controllers can run in the same replicas with database coordination; there is no
required sticky session. External PostgreSQL must itself be durable and HA for an
HA installation. The chart consumes database/S3/OIDC credentials via existing Secrets,
uses constrained service accounts, probes, graceful drain, a disruption budget, and
spread/anti-affinity settings. It does not imply HA from replicas alone or embed an
unreplicated database as a production default. Local result mode additionally requires
user-managed persistent shared mounts on every serving replica, the relevant drivers,
and participating executors; S3 mode requires provider connectivity and identity.
Helm exposes existing volume/mount references and readiness checks for configured roots.

When public OIDC is enabled, identity comes from validated OIDC bearer tokens mapped to a stable tenant and
subject. Caller-supplied owner fields are rejected. V1 contexts and jobs are private
to that identity; cross-owner reads/cancel/result and ensure cannot expose resources.
SDKs accept a refreshable credential provider. Workers authenticate with projected
Kubernetes identities verified against their assigned Pod/context incarnation; their
permissions are limited to claims and reporting for that assignment. Result uploads
are restricted to assigned object keys. Raw credentials are not stored in job inputs.

Deployment configuration uses `database.postgres` connection fields and an independent
credential `secretRef`, `usernameKey`, and `passwordKey`. S3 has its own secret/key
reference. There is no deployment mode. `security.enabled: false` disables all Linha
authentication and uses the fixed anonymous owner; `security.oidc.enabled: false`
with security enabled makes the public API anonymous while authenticating workers.
No switch remaps ownership of retained records. Anonymous worker headers carry
routing identity only; fencing and durable lifecycle checks still apply.

Namespace-only RBAC is the default. It creates only Roles and RoleBindings and
verifies projected worker JWTs using API-server discovery/signing keys, then checks
live namespace, service-account UID, Pod UID/deletion state, labels, and persisted
assignment. This relies on Kubernetes' standard issuer-discovery access, not a new
cluster permission granted by Linha. A separate opt-in adds only TokenReview cluster
permission and switches worker validation accordingly. Both variants manage resources
only in the release namespace. Disabled security needs no TokenReview permission or
Linha token projection. Spark still authenticates to Kubernetes for executor management.

Use backward-compatible migrations run once with database locking before new replicas
become ready. Readiness includes required persistence/schema availability; liveness does
not depend on transient database availability. Metrics include queue age/depth, worker
capacity, lease expirations, retries, failed provisioning, result failures, and pending
Kubernetes resources. Logs correlate context/job/attempt IDs without dumping payloads.
No new claims/acceptance can succeed while the database is unavailable; existing workers
follow their lease deadlines and recovery reconciles after service restoration.

## Risks / Trade-offs

- [Cross-system writes are not atomic] -> Idempotent resource operations, immutable output staging,
  conditional completion, and reconciliation; no exactly-once promise for handler side effects.
- [Database queue contention] -> Indexed claims, bounded batches, capacity accounting and load tests;
  add a broker only when measured need outweighs the extra consistency surface.
- [Driver failure affects concurrent requests] -> Bounded concurrency and configurable multi-driver
  isolation, with durable attempt outcomes and user-selected retries.
- [User images cannot start or disagree with schemas] -> Digest-pinned identity, readiness manifests,
  deterministic compatibility failures, deployment conditions, and bounded startup retries.
- [SDK/library version conflicts] -> Separate modules, pinned JVM compatibility matrix, Spark as a
  provided dependency where appropriate, and an example custom-image integration test.
- [Leaked caches prevent executor shrinkage] -> Request-scoped ownership/cleanup and sustained-worker
  resource tests before production use.
- [Local roots or S3 templates are misconfigured] -> Validate names, destination containment,
  shared mounts and collision-free attempt namespaces; fail visibly without storage fallback.
- [All processes restart] -> Rebuild from PostgreSQL plus durable local/S3 storage; persistence loss needs backups
  and is outside the process-restart guarantee.

## Migration Plan

1. Establish raw JSON transport, schema migrations, and a fake engine with local filesystem results.
2. Layer typed Scala entrypoints and LinhaJobContext on the raw flow; verify identical
   acceptance, progress, result publication, and retrieval after restart.
3. Add S3 publication and versioned path templates, file/stream helpers, failure recovery,
   authentication, local shared-mount and multi-replica tests.
4. Deliver a real Spark worker image, managed dataset output, and Kubernetes adapter
   at fixed driver capacity.
5. Add driver scaling/dynamic allocation and package/test the HA Helm deployment.
6. Run restart, duplicate-delivery, stale-completion, storage-outage, and ownership tests.

There is no existing Linha data to migrate. Future rollback must preserve database
compatibility, drain incompatible workers, and retain job records/results. METOC adoption
is a later change; use its extraction and enrichment as realistic compatibility fixtures.

## Open Questions

- Final Maven coordinates, Go module publishing path, image registry, and release naming.
- Operator-selected numeric defaults for result size/retention, leases, retry backoff,
  concurrency, queue thresholds, and resource ceilings after measurements.
- Deployment-specific OIDC issuer/audience, PostgreSQL endpoint, shared mount bindings,
  and S3 destination settings.

## Completion decisions (2026-09-30)

Later changes supersede the original immutable-name/client-mode sections: use hashed
context versions with client leases and SparkApplication submission, as specified in
`manage-versioned-spark-applications`. Existing public IDs and result receipts stay valid.

Dataset publication adds a prefix allocation, paginated part metadata in PostgreSQL,
and an immutable JSON-lines manifest in the selected provider. Parts are registered in
bounded batches and verified before the dataset is sealed; sealing freezes the part
list. Only the existing fenced job-completion transaction makes the dataset public.
Spark writes directly to shared local storage or S3A with the magic committer. S3
writes require explicitly configured STS delegation with a session policy restricted
to the allocated prefix; credentials are returned only to the current worker, never
persisted in job inputs. Per-write Hadoop configuration disables filesystem caching
and does not rebind global credentials. Unsupported delegation fails explicitly.

Cleanup is now authorized by the user's request to finish all remaining tasks. A
reconciler may delete only persisted allocations whose attempt is terminal and whose
retained result has expired or does not reference them. It waits out delegated-write
credentials and a grace period; retries are idempotent and coordinated across replicas.
Jobs and completion receipts remain durable tombstones (no automatic job deletion).
Cleanup is an explicit deployment option, disabled by default on upgrade. Tests use
only disposable roots/buckets. Staging has bounded concurrent uploads and byte quotas.
