# Validation record

Admin UI and observability were added on 2026-10-01. The
[acceptance record](admin-observability-validation.md) maps their 59 tasks to
checks, measured collection/cardinality results and disposable-cluster evidence.


The platform acceptance run completed on 2026-09-30. All 68 foundation tasks and
six follow-up changes were implemented and archived. The
[acceptance matrix](acceptance.md) maps 163 scenarios across 14 capabilities to
their regression groups. [Measured results](validation/README.md#measurements)
come from disposable fixtures, not production load.

## Verified behavior

- Durable acceptance, idempotency, owner isolation, attempts, cancellation,
  lease fencing and configured retries against PostgreSQL 17.
- Raw and typed Scala entrypoints, progress, persisted exception diagnostics,
  local/S3 files and partitioned datasets, and retrieval after process restart.
- Immutable part publication, scoped STS writes, incremental manifest/part access,
  staging limits and opt-in expired/abandoned file and multipart cleanup.
- SparkApplication provisioning, CPU requests/limits, heap plus memory overhead,
  template mounts/Secrets, driver replacement and leased configuration versions.
- Real Spark 3.5.6 on Kubernetes 1.33.1 with Operator 2.4.0: independent driver and
  executor scaling, UI access, scoped cache cleanup and concurrent requests.
- Two-replica API rollout, local mount readiness failure/recovery, actual database
  outage beyond lease expiry and retained results after all drivers stopped.
- Go race tests, JVM tests, Helm configuration tests, OpenAPI and strict OpenSpec
  checks. The 2026-10-01 local Maven installation passed all 16 JVM tests and
  verified that installed source JARs match the source files.

The executable checks and their scopes are listed in the
[validation summary](validation/README.md#checks). Raw logs, generated request IDs,
Pod names and environment-specific image digests are local artifacts and are
excluded from Git. Scripts and curated findings remain versioned.

## Deployment boundaries

The fixture used one Kind host with persistent test PostgreSQL, MinIO and a
shared POSIX result volume. This validates application recovery and replica
coordination, not HA of the underlying database, object store or filesystem.
OIDC signatures, expiry, key rotation and owner mapping have signed-token tests;
a production external identity provider was not exercised in the cluster.

Production IAM, multi-node storage failure, maximum-size payloads/part counts and
capacity sizing need validation in the target environment. No production rollout,
image push or example applications migration was performed as part of this acceptance run.

Schema v10 includes optional client hostnames and overview indexes, in addition
to the operational observations, durable metrics, admin sessions/audit and dataset
metadata. Older server binaries cannot run against v10. Result cleanup remains opt-in, and job metadata,
completion receipts and idempotency records remain indefinitely. See
[dataset configuration](dataset-results.md), [operations](operations.md) and the
[client/worker walkthrough](walkthrough.md) before deployment.

## Retired-resource cleanup regression (2026-10-12)

A read-only investigation of a two-replica 0.1.1 deployment found 64 DEAD
instances, zero live engine instances, and approximately 150 missing-resource
Kubernetes requests per second across the replicas. Two metrics snapshots were
181.5 seconds apart. The supplied audit CSV contained 4,053 404 calls: 1,102 each
for Pod GET, Pod DELETE and Service GET, plus 747 SparkApplication GETs.
The retired-intent loop was repeatedly probing already absent resources.

The regression suite uses PostgreSQL 17 and HTTP Kubernetes fixtures. With 5,000
retired records and no resources, 100 controller ticks issue three collection
GETs and no per-resource GET/DELETE calls; the next scheduled sweep adds three
collection GETs. Tests cover pagination, namespace/label/name filtering, UID
preconditions, legacy Service-only cleanup, unknown/active/foreign intents,
competing controllers and ownership transfer, a late create after controller
restart, bounded retries after inventory/deletion failure, and restored capacity
when a client reattaches. Historical records remain intact.

Validation passed: `make check` with `LINHA_TEST_DATABASE`, uncached
`go test -race -count=1 ./server/...` against PostgreSQL 17, Java 17 `mvn -B
package`, a rebuilt Go server with `scripts/e2e.py`, and `openspec validate --all
--strict` (25 items). The restart smoke verified exception diagnostics and
retained file results across client/server/worker restarts. Temporary PostgreSQL
containers were removed after testing.

The cluster investigation made no production changes. Post-fix request rates
must be checked after all server replicas receive the corrected image; the
fixture measurements do not represent an already deployed production fix.
