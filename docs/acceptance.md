# Platform acceptance — 2026-09-30

All 68 platform tasks and the six subsequent corrections are implemented and
covered by the development acceptance record. This is a bounded, single-host
release exercise, not a production throughput or infrastructure-HA certification.

## Completed work

| Tasks | Outcome | Evidence |
| --- | --- | --- |
| 2.1, 2.2 | Versioned JSON client/worker API, output access, immutable parts and manifest completion | [OpenAPI](../api/openapi.json), [validator](validation/README.md#checks) |
| 2.5, 5.8 | Go/JVM request and config fixtures, image identity, schema descriptors, UTC/path validation and persisted allocation replay | [conformance](protocol-conformance.md), [Go tests](validation/README.md#checks), [JVM tests](validation/README.md#checks) |
| 4.8 | File/stream helpers, callback cleanup, staging reservations and size bounds | worker StagingBudgetTest, storage limit tests, [restart test](../scripts/e2e.py) |
| 5.4 | Owner-scoped values/files, paginated dataset index and streaming parts, fresh API authorization | [dataset run](../scripts/platform-results-e2e.py), httpapi ownership tests |
| 5.5, 5.6 | Opt-in retention/orphan cleanup, grant grace, immutable replay, bounded retries, expired metadata and tombstones | postgres dataset_cleanup/output/store tests, storage dataset/S3 tests, [fault run](../scripts/platform-fault-e2e.py) |
| 6.4 | Persisted startup conditions/backoff, result-access readiness, incarnation tracking and drain | controller/HTTP tests, [cluster mount check](../scripts/platform-load-e2e.py) |
| 7.1 | Runnable raw/typed Spark image with pinned Hadoop AWS/cloud connectors | [example](../examples/jvm/src/main/scala/linha/example/SparkMain.scala), [image check](../scripts/platform-final-smoke.py) |
| 7.8, 7.9 | Partitioned local/S3 Parquet, scoped STS, immutable snapshots, failure isolation, missing-mount rejection and concurrent paths | [dataset run](../scripts/platform-results-e2e.py), [Kubernetes run](../scripts/platform-load-e2e.py), DistributedResultsTest |
| 8.5 | Driver and executor growth/shrink measured separately; request cache and broadcast lifecycle exercised | [measurements](validation/README.md#measurements), SparkRuntimeTest |
| 9.6 | Two Helm replicas, PostgreSQL, MinIO and shared results; rollout and access failure/recovery | [cluster run](../scripts/platform-load-e2e.py), [chart tests](../scripts/test-helm.py), [final images](../scripts/platform-final-smoke.py) |
| 10.2 | Real PostgreSQL pause beyond lease expiry, no acknowledged acceptance, stale completion rejection and opt-in retry | [fault run](../scripts/platform-fault-e2e.py) |
| 10.4 | Retained local/S3 values and datasets after restart/replacement, ownership and refreshed credentials, different worker timezone | [dataset run](../scripts/platform-results-e2e.py), [final images](../scripts/platform-final-smoke.py), TransportTest and HTTP ownership tests |
| 10.5 | Raw dispatch, typed jobs, values/files/streams/Parquet, local/S3 paths and restored handles | [walkthrough](walkthrough.md) |
| 10.6 | Explicit scenario coverage and measured limits | This record and tables below |

## Measured defaults and limits

The four-job load used two allowed drivers, one request slot per driver, and
0–2 one-core executors per driver. Driver/executor heap was 512m with 384m
overhead. Timeouts were executor idle 5s, cached idle 8s and shuffle tracking 8s;
driver idle was 120s. All measurements are observations from one Kind host.

- Durable acceptance across seven submissions: 2.96–4.99 ms through the local port-forward.
- First-attempt queue waits for four load jobs: 7.44s, 7.47s, 37.84s, 38.04s (includes cold startup and occupied capacity).
- Observed maximum: two drivers, four executors, two concurrent requests.
- After load completion, executor removal reached zero in another 4.24s while both drivers remained. Polling began after the UI/cache check, not at the last Spark task finish.
- Spark UI listed 0 retained request RDDs after completion. SparkRuntimeTest also checks shared-cache reference counting and scoped cleanup.
- Waiting after the later dataset/rollout checks observed idle drivers return to zero in 107.44s, with clients still attached. The configured 120s timer begins at each driver's last activity.
- Attempt lease: 30s. Client lease: 90s, SDK renewal every 30s. Default retry: one attempt; maximum ten.
- Default result size: 64 MiB; maximum 5 GiB. Default server/worker staging reservations: 5 GiB. Dataset cap: 100,000 parts; API page/batch cap: 200, SDK registration batch: 100.
- Default result retention: seven days. Cleanup is off by default; when enabled, it waits for attempt termination, delegated-write expiry and ten minutes of grace. Tombstones/receipts remain indefinitely.
- STS grants last at most one hour; no transparent renewal during a distributed write. Seekable user callbacks need filesystem quotas for a hard disk bound. Dataset snapshots cost additional API I/O and temporary disk.

No expiring public S3 download URLs are issued: every download authorizes the
current principal through the API. Thus signed-URL expiry has no permanent-ID
failure mode; credential refresh and cross-owner rejection are exercised instead.
S3 was tested against MinIO's API and STS implementation. Production IAM, an
external OIDC provider, multi-node filesystem failure, load at the 5 GiB/100,000-part
ceilings, and production capacity sizing were not certified by this fixture.
OIDC cryptographic validation, expiry, key rotation and owner mapping have local
signed-token tests. These are deployment validation boundaries, not unimplemented
platform features.

## Scenario coverage

Each row maps the specification's scenario to its executable regression group.
The recorded runs cover cancellation, replacement, client expiry and
resource/template behavior. The scripts reproduce these checks; raw run artifacts
are intentionally excluded from Git.

### client-pod-customization

Regression group: Engine pod_template/registry tests; PostgreSQL pod_templates tests; PodTemplateTest; pod-template-e2e.py; pod-template-e2e.py.

| Scenario | Coverage |
| --- | --- |
| Application config on both roles | Implemented; covered by the group above |
| Raw and Scala client equivalence | Implemented; covered by the group above |
| Defaults change after acceptance | Implemented; covered by the group above |
| Concurrent ensure with different settings | Implemented; covered by the group above |
| Worker identity override | Implemented; covered by the group above |
| Foreign context access | Implemented; covered by the group above |
| Repeated provisioning | Implemented; covered by the group above |
| Private worker image | Implemented; covered by the group above |
| Secret outside policy | Implemented; covered by the group above |
| Namespace-only chart | Implemented; covered by the group above |

### spark-ui-and-retention

Regression group: Engine/runtime-settings tests; PostgreSQL spark_runtime_settings tests; platform-load-e2e.py.

| Scenario | Coverage |
| --- | --- |
| Enabled UI on a custom port | Implemented; covered by the group above |
| Invalid port | Implemented; covered by the group above |
| Finite retention | Implemented; covered by the group above |
| Invalid duration | Implemented; covered by the group above |
| Static allocation | Implemented; covered by the group above |
| Concurrent equivalent ensures | Implemented; covered by the group above |
| Restart and stale worker | Implemented; covered by the group above |
| Another owner | Implemented; covered by the group above |

### durable-request-management

Regression group: PostgreSQL store/output tests; platform-fault-e2e.py; e2e.py.

| Scenario | Coverage |
| --- | --- |
| Non-JVM client submits a job | Implemented; covered by the group above |
| Submission response is lost across a date boundary | Implemented; covered by the group above |
| Server fails immediately after acknowledgement | Implemented; covered by the group above |
| Database is unavailable | Implemented; covered by the group above |
| Concurrent duplicate submissions | Implemented; covered by the group above |
| Idempotency key is reused with different input | Implemented; covered by the group above |
| Cross-owner query or cancellation | Implemented; covered by the group above |
| A request receives another attempt | Implemented; covered by the group above |
| Backend has not started | Implemented; covered by the group above |
| Two workers compete for the same request | Implemented; covered by the group above |
| Worker dies with retries disabled | Implemented; covered by the group above |
| Worker dies with a retry available | Implemented; covered by the group above |
| Every server restarts during a queued retry | Implemented; covered by the group above |
| Expired worker reports success | Implemented; covered by the group above |
| Cancellation wins the race | Implemented; covered by the group above |
| Completion wins the race | Implemented; covered by the group above |

### durable-result-storage

Regression group: PostgreSQL output/dataset_cleanup tests; storage local/S3/dataset tests; path fixtures; platform-results-e2e.py; platform-load-e2e.py; HTTP ownership tests.

| Scenario | Coverage |
| --- | --- |
| Local JSON result | Implemented; covered by the group above |
| S3 result | Implemented; covered by the group above |
| Result exceeds a configured limit | Implemented; covered by the group above |
| Worker is replaced with the shared volume retained | Implemented; covered by the group above |
| Required root is absent | Implemented; covered by the group above |
| File path escapes the result root | Implemented; covered by the group above |
| Date-partitioned custom path | Implemented; covered by the group above |
| Unknown variable or invalid format | Implemented; covered by the group above |
| Destination escape in a template | Implemented; covered by the group above |
| Client or worker restarts after midnight | Implemented; covered by the group above |
| Job retries on a later day | Implemented; covered by the group above |
| Client timezone differs from server timezone | Implemented; covered by the group above |
| Template omits attempt identity | Implemented; covered by the group above |
| Late writer overlaps a retry | Implemented; covered by the group above |
| Duplicate logical filename | Implemented; covered by the group above |
| Invalid artifact name | Implemented; covered by the group above |
| Seekable writer with S3 storage | Implemented; covered by the group above |
| Stream callback fails | Implemented; covered by the group above |
| Distributed dataset is produced | Implemented; covered by the group above |
| Worker dies during upload | Implemented; covered by the group above |
| Stale attempt finishes uploading | Implemented; covered by the group above |
| Dataset has an incomplete write | Implemented; covered by the group above |
| Completion acknowledgement is lost | Implemented; covered by the group above |
| All processes restart after completion | Implemented; covered by the group above |
| Storage is temporarily unavailable | Implemented; covered by the group above |
| An old download URL expires | Implemented; covered by the group above |
| Result payload expires | Implemented; covered by the group above |
| Cleanup observes an active writer | Implemented; covered by the group above |
| Expired delegated writer | Implemented; covered by the group above |
| Abandoned multipart upload | Implemented; covered by the group above |
| Conflicting part replay | Implemented; covered by the group above |
| Scoped S3 authorization | Implemented; covered by the group above |
| Cross-owner parts | Implemented; covered by the group above |
| Concurrent jobs and restart | Implemented; covered by the group above |

### ha-deployment

Regression group: Auth/config/controller tests; test-helm.py; platform-fault-e2e.py; platform-load-e2e.py; restart-e2e.py (historical fixture).

| Scenario | Coverage |
| --- | --- |
| Install with external dependencies | Implemented; covered by the group above |
| One server replica fails | Implemented; covered by the group above |
| Two replicas start migrations together | Implemented; covered by the group above |
| All application processes stop and restart | Implemented; covered by the group above |
| Database outage exceeds attempt leases | Implemented; covered by the group above |
| Backend repeatedly fails to start | Implemented; covered by the group above |
| Result is read through another replica | Implemented; covered by the group above |
| Restore with a missing required mount | Implemented; covered by the group above |
| Custom database Secret keys | Implemented; covered by the group above |
| Authentication disabled | Implemented; covered by the group above |
| Only OIDC disabled | Implemented; covered by the group above |
| Namespace-only authenticated installation | Implemented; covered by the group above |
| Token belongs to a replaced Pod or service account | Implemented; covered by the group above |
| Signing-key discovery is inaccessible | Implemented; covered by the group above |

### managed-backend-contexts

Regression group: PostgreSQL client_leases/pod_templates tests; controller tests; pod-template-e2e.py.

| Scenario | Coverage |
| --- | --- |
| Several API replicas start together | Implemented; covered by the group above |
| New specification version | Implemented; covered by the group above |
| Image tag changes in its registry | Implemented; covered by the group above |
| Unsupported engine setting | Implemented; covered by the group above |
| Driver disappears | Implemented; covered by the group above |
| Client closes its handle | Implemented; covered by the group above |
| Provisioning cannot complete | Implemented; covered by the group above |
| Non-Spark test backend | Implemented; covered by the group above |
| Controller fails after resource creation | Implemented; covered by the group above |
| Caller changes the S3 template | Implemented; covered by the group above |
| Worker restarts without the original client | Implemented; covered by the group above |

### managed-spark-engine

Regression group: Engine/controller tests; SparkRuntimeTest; DistributedResultsTest; platform-load-e2e.py; cluster-e2e.py (historical fixture).

| Scenario | Coverage |
| --- | --- |
| Invalid driver range | Implemented; covered by the group above |
| Driver minimum is zero | Implemented; covered by the group above |
| Two requests have available capacity | Implemented; covered by the group above |
| One request is cancelled | Implemented; covered by the group above |
| Queue remains above the configured trigger | Implemented; covered by the group above |
| Queue becomes empty | Implemented; covered by the group above |
| Cluster is full | Implemented; covered by the group above |
| Minimum-capacity driver is deleted | Implemented; covered by the group above |
| Lost driver carried a non-retryable request | Implemented; covered by the group above |
| Dynamic allocation is enabled | Implemented; covered by the group above |
| Repeated requests reuse one driver | Implemented; covered by the group above |
| Partitioned dataset is saved | Implemented; covered by the group above |
| Executors cannot access local storage | Implemented; covered by the group above |
| Distributed write fails after some tasks finish | Implemented; covered by the group above |

### typed-client-sdk

Regression group: ProtocolFixtureTest; TransportTest; ClientLeaseTest; e2e.py; platform-results-e2e.py; HTTP ownership tests.

| Scenario | Coverage |
| --- | --- |
| Submit without a typed SDK | Implemented; covered by the group above |
| Raw and typed submissions are equivalent | Implemented; covered by the group above |
| Submit and reconstruct an entrypoint | Implemented; covered by the group above |
| Incompatible result schema | Implemented; covered by the group above |
| Backend is starting | Implemented; covered by the group above |
| API process restarts | Implemented; covered by the group above |
| S3 path variables are configured before a job exists | Implemented; covered by the group above |
| Submission response is lost | Implemented; covered by the group above |
| Same job type uses different providers | Implemented; covered by the group above |
| Dataset result contains multiple parts | Implemented; covered by the group above |
| Retrieve a completed job after all processes restart | Implemented; covered by the group above |
| Caller requests another user's job | Implemented; covered by the group above |

### worker-runtime

Regression group: WorkerCancellationTest; FailureDiagnosticsTest; PostgreSQL fencing tests; e2e.py; platform-results-e2e.py.

| Scenario | Coverage |
| --- | --- |
| User dispatches JSON manually | Implemented; covered by the group above |
| SDK dispatches a typed entrypoint | Implemented; covered by the group above |
| Job reports progress | Implemented; covered by the group above |
| Progress reaches completion before results finish | Implemented; covered by the group above |
| Callback fails while holding managed resources | Implemented; covered by the group above |
| Worker starts successfully | Implemented; covered by the group above |
| Handler version is unavailable | Implemented; covered by the group above |
| Worker is full | Implemented; covered by the group above |
| Worker is draining | Implemented; covered by the group above |
| Network partition outlasts the lease | Implemented; covered by the group above |
| Old process reconnects | Implemented; covered by the group above |
| Handler throws | Implemented; covered by the group above |
| Completion acknowledgement is lost | Implemented; covered by the group above |
| Worker requests another context's job | Implemented; covered by the group above |

### spark-collection-cleanup

Regression group: test-helm.py; test-worker-rbac.py; test-worker-rbac.py.

| Scenario | Coverage |
| --- | --- |
| Spark cleanup by application labels | Implemented; covered by the group above |
| Namespace boundary | Implemented; covered by the group above |
| Permission applies to existing worker identity | Implemented; covered by the group above |

### spark-resource-enforcement

Regression group: Engine resource/SparkApplication tests; pod-template-e2e.py; pod-template-e2e.py.

| Scenario | Coverage |
| --- | --- |
| Distinct resource budgets | Implemented; covered by the group above |
| Dynamic allocation | Implemented; covered by the group above |
| Driver replacement after server restart | Implemented; covered by the group above |
| Memory accounting | Implemented; covered by the group above |

### leased-context-versions

Regression group: PostgreSQL client_leases tests; controller tests; ClientLeaseTest; pod-template-e2e.py.

| Scenario | Coverage |
| --- | --- |
| Concurrent ensure | Implemented; covered by the group above |
| Rolling change | Implemented; covered by the group above |
| Client killed | Implemented; covered by the group above |
| Accepted work | Implemented; covered by the group above |
| HA restart | Implemented; covered by the group above |
| Stale or foreign lease | Implemented; covered by the group above |
| Reactivation | Implemented; covered by the group above |
| Retry during rollout | Implemented; covered by the group above |
| Retained result | Implemented; covered by the group above |

### spark-application-provisioning

Regression group: Engine SparkApplication tests; auth owner checks; pod-template-e2e.py; platform-load-e2e.py.

| Scenario | Coverage |
| --- | --- |
| Heap plus overhead | Implemented; covered by the group above |
| Native customization | Implemented; covered by the group above |
| Pending driver | Implemented; covered by the group above |
| Lost driver | Implemented; covered by the group above |
| Legacy stored context | Implemented; covered by the group above |

### job-exception-diagnostics

Regression group: Domain/PostgreSQL diagnostics tests; FailureDiagnosticsTest; WorkerCancellationTest; e2e.py; HTTP ownership tests.

| Scenario | Coverage |
| --- | --- |
| Handler exception | Implemented; covered by the group above |
| Initialization exception | Implemented; covered by the group above |
| Observed memory exhaustion | Implemented; covered by the group above |
| Large Unicode failure | Implemented; covered by the group above |
| Old stored failure | Implemented; covered by the group above |
| Reporting unavailable | Implemented; covered by the group above |
| Retry and stale worker | Implemented; covered by the group above |
| Cross-owner access | Implemented; covered by the group above |
| Deadline authority | Implemented; covered by the group above |

Total: 163 specification scenarios across 14 capabilities.
