# How Linha works

Linha separates durable job management from engine execution. Clients send JSON;
PostgreSQL owns requests, attempts and leases; workers run business code and
publish results through Linha's storage policy.

## Acceptance and execution

The server returns an opaque job ID after committing acceptance to PostgreSQL.
A driver need not be ready yet. A later worker claims an attempt and renews its
lease while it executes. Progress and completion are accepted only for the current
attempt and worker identity.

An idempotency key makes repeated acceptance safe. The same owner, logical context
name, key, request and result policy return the original job. Reusing the key with
different content is a conflict. It does not make arbitrary external business
writes exactly-once. Execution retries are separate and disabled by default.

## Context names and versions

A context is an owner's logical workload, such as `reporting`. A version records
its effective engine settings, result policy, Pod templates and resolved worker
image digest. Linha normalizes and hashes these values; callers do not add version
suffixes to the context name.

Each application process leases the version it uses. SDK leases last 90 seconds
and renew every 30 seconds. Old and new versions can coexist during a rolling
update. When the final client releases its lease or disappears, the version drains:
it finishes accepted work, then stops its drivers. `minDrivers` does not keep an
unused drained version alive. A stuck accepted job can delay draining until it
finishes or is cancelled.

Restoring a job handle does not attach to a context or start an engine. Reading
retained results requires only the same authenticated owner and job ID.

## Spark concurrency and capacity

A driver owns one classic SparkContext. Each request receives a separate
SparkSession, scheduler pool and attempt-specific job group. Several requests can
run at once, bounded by `maxConcurrentRequestsPerDriver`; cancellation targets
one job group. Spark executor slots still determine how much task work runs in
parallel.

Linha scales the driver pool within the requested bounds and replaces lost
required drivers. Spark independently manages executors through dynamic allocation.
A worker interruption can fail or retry an attempt according to the accepted retry
policy. Replacing a driver alone does not guarantee that a job is retried.

## Result publication and retention

JSON, files and datasets use the context's local or S3 policy. A completed write
is not yet a public result: the current attempt must complete successfully. Dataset
parts are copied into immutable published allocations before they are exposed.
Late writers and stale attempts cannot replace the retained result.

Job IDs and status remain after result payload expiry. Retrieval then returns
`RESULT_EXPIRED`. Optional physical cleanup removes only eligible tracked
allocations; it does not delete job history or arbitrary bucket contents.

## Authentication boundaries

| Connection | Identity and purpose |
| --- | --- |
| Application → API | OAuth 2.0 client credentials; signed JWT access token with audience `linha` |
| Browser → administrator console | OIDC authorization code with PKCE; browser client audience and explicit viewer/operator claims |
| Managed worker → server | Bound Kubernetes service-account token, checked against the live managed instance |
| Server → PostgreSQL/S3/Kubernetes | Deployment credentials managed by the operator |

The issuer identifies who issued a token. The audience identifies the service it
was issued for. Signature validation alone does not establish that the token was
intended for Linha. All callers can share the `linha` audience while retaining
distinct subjects and owner-isolated job records.

Administrator roles apply only to the separate admin surface. Worker credentials
are injected into managed worker Pods; they are unrelated to application OAuth
client secrets and must not be removed when simplifying application authentication.

## Availability boundaries

Multiple server replicas share PostgreSQL locks, persisted controller intent and
sessions. Restarts preserve accepted jobs, result references and client leases.
The Helm chart cannot make a single database, object store or local disk highly
available. Use durable infrastructure and test failures appropriate to the target
environment. Authorized worker images execute with namespace permissions and
shared storage access, so deployments should group workloads by their trust boundary.
