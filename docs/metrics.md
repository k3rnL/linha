# Metric catalogue

These families are exported from `/metrics`. They supplement the deprecated aliases documented in [observability.md](observability.md#compatibility). Every family has HELP and TYPE metadata. Seconds and bytes are base units. Histogram names imply `_bucket{le}`, `_sum`, and `_count`; Info is a gauge with value 1.

## Scope and labels

- **S**: shared authoritative database state; copies on server replicas must be deduplicated.
- **L**: process-local instrumentation; keep per-pod or sum rates across pods.
- **O**: shared persisted engine observation; deduplicate, require fresh observations, and never sum server copies.
- **C** means labels `engine,context` from the persisted bounded context registry, not request IDs or version hashes.
- **D** means `provider,destination`; provider is `local|s3`, destination is a configured destination alias (`local` for the local provider), never a URI/bucket/key.
- Deployment labels `cluster` (when configured), `namespace`, `linha_deployment`, `service`, `pod`, and `instance` are attached at scrape discovery. They are not user-controlled request fields.

Snapshot-derived series disappear on collection failure. Known zero states are emitted on a successful snapshot. O-series exclude stale instance contributions and must be accompanied by observation completeness/staleness gauges; a partial observation is marked incomplete on the dashboard, not presented as a complete lower count.

## Clients and contexts

| Name | Type/scope | Labels | Meaning |
| --- | --- | --- | --- |
| `linha_client_leases` | Gauge S | C, `state` | Current stored lease generations: active, expired, released; active uses database-clock expiry. |
| `linha_client_lease_events_total` | Counter S | C, `event` | Acquired, explicitly released, or expired generations, once per event/generation. Renewals do not count as acquisitions. |
| `linha_context_versions` | Gauge S | C, `state` | Immutable versions in each normalized lifecycle state. |
| `linha_context_versions_with_demand` | Gauge S | C, `demand` | Versions with live clients or accepted work, reported separately as `clients|work`; one version can have both, so do not sum these as distinct versions. |
| `linha_context_conditions` | Gauge S | C, `reason` | Versions with a provisioning/lifecycle condition, grouped by fixed platform reason. |
| `linha_metrics_context_labels` | Gauge S | `state` | Number of permanently assigned named dimensions and logical contexts grouped into overflow. |

Expired/released lease rows are record counts, not current demand; historical acquisition/release/expiry rates use counters. Client identifiers and exact last-seen times are available in the admin API only.

## Jobs and execution

| Name | Type/scope | Labels | Meaning |
| --- | --- | --- | --- |
| `linha_job_records` | Gauge S | C, `state` | Retained jobs currently in QUEUED, RETRYING, RUNNING, CANCELLING, SUCCEEDED, FAILED, CANCELLED; terminal values include retained history. |
| `linha_queue_jobs` | Gauge S | C, `eligibility` | Queued/retrying work classified `ready|backoff`; never includes running work. |
| `linha_queue_oldest_age_seconds` | Gauge S | C | Age since original submission of oldest QUEUED/RETRYING job, including retry backoff; zero for an empty queue. |
| `linha_job_submissions_total` | Counter S | C, `source` | Newly committed jobs (`sdk|admin`), excluding idempotent repeats. |
| `linha_job_completions_total` | Counter S | C, `outcome` | First committed terminal job transitions: SUCCEEDED, FAILED, CANCELLED. |
| `linha_job_failures_total` | Counter S | C, `reason` | Terminal FAILED jobs classified by bounded reason; business exception text never becomes a label. |
| `linha_job_retries_total` | Counter S | C, `reason` | Persisted additional-attempt decisions, not duplicate claim attempts. |
| `linha_attempts_current` | Gauge S | C, `state` | Nonterminal current attempts with `running|cancelling|lease_expired`, a disjoint classification. |
| `linha_attempt_completions_total` | Counter S | C, `outcome` | First persisted attempt termination, including failure/interruption/cancellation; duplicate completion callbacks excluded. |
| `linha_job_queue_wait_seconds` | Histogram S | C | Submission to first assignment, exactly one observation when first claimed. |
| `linha_attempt_duration_seconds` | Histogram S | C | Assignment to persisted terminal attempt state, exactly one observation per terminated attempt. |
| `linha_job_duration_seconds` | Histogram S | C, `outcome` | Submission to terminal job outcome, including retries and queue wait. |
| `linha_job_cancellation_duration_seconds` | Histogram S | C | First cancellation intent to CANCELLED; success-before-cancel is excluded. |
| `linha_worker_updates_rejected_total` | Counter L | `operation,reason` | Rejected worker reports; operations are a fixed set and reasons include identity mismatch, stale attempt, expired lease, invalid state, invalid payload, and other. |
| `linha_accounting_started_timestamp_seconds` | Gauge S | none | Baseline of new durable lifecycle counters/histograms; old history is represented by gauges, not fabricated events. |

Fixed failure categories: `business_error`, `lease_expired`, `worker_lost`, `deadline_exceeded`, `provisioning`, `result_storage`, `invalid_result`, `other`. Map arbitrary worker failure codes into this set. Do not label by handler or exception type. Attempts use documented normalized outcomes `SUCCEEDED|FAILED|CANCELLED|INTERRUPTED`; preserve richer raw states in the detail API.

Job/attempt/queue/cancellation histogram boundaries: 0.1, 1, 5, 15, 30, 60, 120, 300, 600, 1800, 3600, 10800, 43200, 86400 seconds, plus +Inf. Bucket changes are a metric contract change, not an arbitrary per-replica setting.

## Workers and engines

| Name | Type/scope | Labels | Meaning |
| --- | --- | --- | --- |
| `linha_workers` | Gauge S | C, `state` | Registered workers classified `ready|draining|stale`; stale takes precedence over draining. |
| `linha_worker_capacity_slots` | Gauge S | C | Total configured claim slots on fresh non-draining registered workers. |
| `linha_worker_occupied_slots` | Gauge S | C | Slots held by unexpired current running/cancelling attempts on those workers; unrelated/stale instances are excluded. |
| `linha_engine_instances` | Gauge S | C, `state` | Managed instances by lifecycle state, including STARTING, READY, DRAINING, DEAD. |
| `linha_engine_desired_instances` | Gauge S | C | Current desired instance count after client/work lifetime and scaler rules, not just configured minimum. |
| `linha_engine_configured_instances` | Gauge S | C, `bound` | Sum of configured `min|max` for versions with current demand; stopped versions do not inflate limits. |
| `linha_engine_provisioning_duration_seconds` | Histogram S | C | Instance demand to first ready registered worker, boundaries 1, 5, 15, 30, 60, 120, 300, 600, 1800, +Inf. |
| `linha_engine_instances_replaced_total` | Counter S | C, `reason` | Persisted replacement decisions for lost/failed instances; reconciliation polls are not new events. |
| `linha_engine_observations` | Gauge S | C, `state` | Required live-instance observations classified `fresh|stale|missing`, measured independently of their content. |
| `linha_engine_observation_oldest_timestamp_seconds` | Gauge S | C | Oldest observation among currently required observed instances; missing observations are indicated separately. |
| `linha_spark_driver_pods` | Gauge O | C, `state` | Current owned Spark driver pods by normalized Kubernetes phase, separate from SDK readiness. |
| `linha_spark_executor_pods` | Gauge O | C, `state` | Current owned executor pods by phase; deleted executors disappear. |
| `linha_spark_executor_configuration` | Gauge S | C, `bound` | Sum of configured executor `min|initial|max` across currently desired drivers; static instances use the same value for all three. |
| `linha_spark_pod_cpu_cores` | Gauge O | C, `role,allocation` | Sum of Spark main-container CPU requests/limits; role driver/executor, allocation request/limit. |
| `linha_spark_pod_memory_bytes` | Gauge O | C, `role,allocation` | Sum of observed Spark main-container memory requests/limits, including container overhead; not memory usage. |

Normalize pod phases to `Pending|Running|Succeeded|Failed|Unknown|Terminating`, using Terminating when deletion is observed. Ready workers and Running pods are not interchangeable. Desired demand can change between snapshots; explicitly timestamp it. Do not publish partial resource totals as complete when a quantity is missing; expose a fixed `missing_resources` observation condition in the internal detail/condition projection.

## Servers, dependencies, and background loops

| Name | Type/scope | Labels | Meaning |
| --- | --- | --- | --- |
| `linha_server_build_info` | Info L | `version,revision` | Build metadata, once per responding server. |
| `linha_server_ready` | Gauge L | none | Latest bounded readiness observation, 0/1; timestamped by its collector. |
| `linha_servers` | Gauge S | `state` | Latest process per server pod classified `ready|unready|stale|stopping`, with 24-hour heartbeat retention. |
| `linha_dependency_available` | Gauge L | `dependency` | Latest configured database/local-storage check; 0/1, never fabricated for an unchecked service. |
| `linha_background_loop_runs_total` | Counter L | `loop,outcome` | Runs of reconcile/recovery/retention/cleanup/heartbeat/observation, success/error. |
| `linha_background_loop_duration_seconds` | Histogram L | `loop` | Duration of each loop run, including errors. |
| `linha_background_loop_last_success_timestamp_seconds` | Gauge L | `loop` | Last success; zero before the first success. |
| `linha_database_pool_connections` | Gauge L | `state` | Acquired, idle, constructing connections. |
| `linha_database_pool_max_connections` | Gauge L | none | Configured pool capacity. |
| `linha_database_pool_acquire_wait_seconds_total` | Counter L | none | Accumulated pool acquisition wait time. |
| `linha_database_pool_acquire_failures_total` | Counter L | `reason` | Timeout/cancel/error acquisition failures. |
| `linha_kubernetes_api_requests_total` | Counter L | `resource,verb,outcome` | Bounded resource/verb set and outcome success/forbidden/not_found/timeout/error. |
| `linha_kubernetes_api_duration_seconds` | Histogram L | `resource,verb` | Kubernetes call latency without names, selectors, URLs, or error strings. |
| `linha_collector_success` | Gauge L | `collector` | Latest collection completed successfully, 0/1. |
| `linha_collector_last_success_timestamp_seconds` | Gauge L | `collector` | Last usable sample time; zero before initialization. |
| `linha_collector_duration_seconds` | Histogram L | `collector` | Background collection latency. |
| `linha_collector_errors_total` | Counter L | `collector,reason` | Bounded timeout/unavailable/invalid/other errors. |

Use fixed collector names `database|readiness|engine_snapshot`; the engine_snapshot collector reports completeness of required engine observations, while per-context detail identifies gaps. No synthetic `up` metric: Prometheus supplies `up` per scrape target. Standard Go/process collectors supply CPU time, resident memory, goroutines, GC and process start time. Runtime metrics do not imply Kubernetes pod readiness; optional external dashboards can use kube-state-metrics for desired replica count and pod restarts.

## HTTP and results/storage

| Name | Type/scope | Labels | Meaning |
| --- | --- | --- | --- |
| `linha_api_requests_total` | Counter L | `route,method,status_code` | All API responses, including unauthorized, worker and admin routes, and unmatched paths; probes, metrics and static assets are excluded. |
| `linha_api_request_duration_seconds` | Histogram L | `route,method` | Time through response completion, including streamed results. |
| `linha_api_requests_in_flight` | Gauge L | `route,method` | Currently executing API requests. |
| `linha_api_response_bytes_total` | Counter L | `route,method` | Bytes written by API responses. |
| `linha_results` | Gauge S | C, D, `state` | Published result sets classified retained/expired, not per-file parts. |
| `linha_result_published_bytes` | Gauge S | C, D, `state` | Logical published file/part bytes recorded in manifests; exclude manifest bytes from dataset part totals and do not count replicas/staging copies. |
| `linha_result_cleanup_candidates` | Gauge S | D | Allocations currently eligible under terminal/expiry/delegation/grace rules. |
| `linha_result_cleanup_runs_total` | Counter L | D, `outcome` | Cleanup executions success/error; these are operations, not exactly-once object deletion counts. |
| `linha_storage_operations_total` | Counter L | D, `operation,outcome` | Server-observed allocate/upload/download/publish/list/delete/delegate/probe/other operations. |
| `linha_storage_operation_duration_seconds` | Histogram L | D, `operation` | Complete server-observed operation latency. |
| `linha_storage_transferred_bytes_total` | Counter L | D, `direction` | Server-observed upload/download bytes; direct worker/Spark S3 writes are excluded. |
| `linha_staging_reserved_bytes` | Gauge L | none | Current server staging budget reservation. |
| `linha_staging_limit_bytes` | Gauge L | none | Configured server staging reservation limit. |

HTTP/local operation duration boundaries: 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, +Inf. Storage/JVM business failures also appear in durable job failure categories when they terminate a request; local storage call metrics do not claim to cover direct worker I/O.

## Compatibility and validation

The old unlabeled HTTP counters and legacy shared gauges retain their original name/shape for one release. New code registers no mixed-definition family. Tests parse exposition, assert HELP/TYPE, escaped values and zero states, and verify fixture totals against PostgreSQL. Test HTTP route normalization with attacker-chosen paths/methods and label-registry overflow with more than the configured context budget. Report the measured series count and SQL/collection cost in validation documentation.
