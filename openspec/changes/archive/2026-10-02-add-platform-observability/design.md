## Context

See [proposal.md](proposal.md). `/metrics` currently performs database queries synchronously and fails the whole scrape when they fail. It exports a small shared snapshot plus two process-local HTTP counters without HELP/TYPE metadata. Every HA replica exposes the same database counts. Health probes already use `/livez` and `/readyz`; `/health` is not an existing API.

Context versions, client leases, workers, attempts, and results are durable. The controller reconciles with persisted ownership epochs; its Spark observation currently retains little beyond readiness and a condition. Server processes have no durable heartbeat view. The existing Helm Service has a named `http` port but needs metadata labels for ServiceMonitor selection.

## Goals / Non-Goals

**Goals:** make failure, demand, capacity, and server health observable with predictable labels; share timestamped internal observations with the admin UI; preserve correct totals under HA, retries, fencing, and restart.

**Non-Goals:** installation of a monitoring stack, business-result analytics, an unrestricted Kubernetes browser, automatic alert routing, distributed tracing, Spark stage/task instrumentation, or replacing kube-state-metrics/cAdvisor/JVM exporters. CPU and memory configuration are observable here; actual pod/JVM usage belongs to those existing exporters and must not be inferred from configured resources.

## Decisions

### 1. Endpoint and collection architecture

Keep `GET /metrics` on the current HTTP service. Use Prometheus's Go client with typed collectors, explicit HELP/TYPE, valid escaping, base units, and a dedicated registry including Go/process collectors. Keep `/livez` dependency-independent and `/readyz` consistent with existing persistence/local-storage readiness. Do not introduce a `/health` alias or repurpose a health probe as a metrics response.

Export process metrics synchronously from memory. Refresh shared database observations in a bounded background collector (default every 10 seconds, query timeout 2 seconds); engine detail is refreshed during existing fenced reconciliation rather than by browser requests or scrapes. Multiple scrapes reuse a consistent snapshot rather than repeating unbounded history scans. Use maintained aggregates/indexed queries, capped detail pages, and a separate bounded query budget so monitoring does not exhaust request connections.

Return HTTP 200 with local metrics and explicit collector failure indicators when a dependency collector fails. Omit the failed collector's shared families immediately instead of returning stale data or zeros. Keep last-success timestamps and error counters. Prometheus `up=1` then means the endpoint is reachable, not that the database or workers are healthy. Emit fresh zeros for known empty state combinations after successful collection. Cold start has unavailable shared data until its first successful snapshot. This preserves diagnostics during database outages, unlike the current all-or-nothing scrape.

The endpoint retains its internal deployment exposure and does not adopt end-user OIDC. Helm monitoring does not create a public ingress. Metrics contain aggregates and bounded context aliases, never request contents or operational-detail credentials; detailed data remains behind the admin API.

### 2. Shared operational read model

Add an internal operational repository and DTOs with `observedAt`, `available`, `stale`, and fixed reason codes separately from human diagnostic text. Database-clock queries classify live client leases and current workers using the same thresholds as scheduling. Record client last renewal time additively; old rows lacking that timestamp report it unknown until renewed, not fabricated.

Provide context summaries, version/client/worker detail, instance demand/state, server heartbeat detail, queue/state aggregates, and result retention summaries. Reading them never ensures a backend, attaches a client, renews a lease, resolves an image, or schedules work. Ownership-aware lookup supports existing owner-scoped callers and a separately authorized admin facade; metrics consume only its aggregate projection.

Engine adapters extend observation with optional typed engine detail and named links. Persist snapshots under the same controller ownership epoch and resource UID/incarnation checks as reconciliation. A stale controller or replaced Pod cannot overwrite the current snapshot. Retain last-known detail for diagnostics with timestamps, but a missing/stale snapshot is not current capacity. Generic/fake adapters remain supported without Spark fields.

For Spark, collect SparkApplication state/condition, driver pod identity/state, current executor pod counts by normalized state, observed resource requests/limits, and UI service/port/ingress information. Use namespaced, paginated pod list/watch with ownership/application association checks; the operator's cumulative executor history must not count deleted executors as currently running. Extend only namespaced get/list/watch RBAC as needed. SparkApplication UID and driver incarnation are checked before publishing detail or links.

Use `status.driverInfo.webUIIngressAddress` for a published `spark-ui` link, preserving the provided host/path. Accept explicit HTTP(S) addresses; resolve scheme-less addresses only against a matching owned Ingress's TLS/host/path data. If that cannot be established, expose the reported address as text rather than guess a clickable URL. Reject active-content schemes/userinfo and report “no published URL” when absent. Do not provision or proxy an ingress. A stopped/replaced driver disables its live link.

Persist a heartbeat every 10 seconds per server process incarnation with pod identity, build version, startup time, readiness, and background-loop last-success/last-error summaries. Use database time; mark stale after 30 seconds without heartbeat, retain stopped/stale records for 24 hours, and mark graceful shutdown when possible. Heartbeat expiry is diagnostic only and must not control request ownership or controller election. Local metrics remain authoritative about the responding process when the database cannot receive its heartbeat.

### 3. Labels and context cardinality

Prometheus target labels provide `namespace`, `service`, `pod`, and `instance`. The chart adds `linha_deployment` (release identity) and an optional configured `cluster` target label for multi-cluster stores. Do not overload Prometheus `job` or `instance` with Linha job/worker identifiers. HTTP labels use matched route templates, a fixed `unmatched` fallback, normalized methods (unknown methods become `OTHER`), and numeric status codes.

Business metric labels are `engine`, `context`, and the bounded states/reasons specified in [metrics.md](metrics.md). `context` denotes an owner-scoped logical context across configuration versions. Automatically assign a stable readable alias from the logical name plus a short collision-checked owner/name digest; persist that mapping independently of versions. An operator can configure aliases before enrollment. Do not publish raw owners or the config/image hash. The admin API exposes the alias-to-context mapping for filters and drill-through.

Cap the lifetime registry at 100 named logical contexts by default; raising the cap is an explicit configuration choice. Once full, route additional contexts into the reserved `__other__` group and expose overflow counts. Never silently recycle aliases, grow the registry during a scrape, or drop excess work from global totals. Existing jobs retain their recorded metric dimension if the mapping/cap changes. Rollouts and restarts reuse existing entries. All versions of one logical context aggregate together; per-version and per-client investigation belongs in the UI.

Never label metrics with request/attempt/client/worker/server-incarnation IDs, payloads, arbitrary handlers, exception classes/text, URLs, file/bucket paths, access tokens, or unconstrained image versions. Configured storage destinations and fixed engine types are bounded enumerations. Bound user-defined failure codes into platform reason categories plus `business_error`/`other`. Cap and test the complete series product, including histogram buckets; the default 100-context fixture must stay below 100,000 Linha series per target, with observed counts documented. Platform-wide totals are always recoverable by aggregation including `__other__`.

### 4. Durable lifecycle accounting and timing

Separate current gauges from monotonically accumulating lifecycle counters. Current job-state gauges include all retained job records, including terminal history; failed-over-time uses terminal-event counters, not the derivative of a historical gauge. Client metrics count logical context leases, not TCP connections or human users. Engine pod readiness and worker registration/claim readiness are separate quantities.

Update durable submission/terminal/attempt/retry/lease-expiry counters and fixed histogram buckets in the same transaction that wins the corresponding lifecycle transition. Use stable event identity/transition guards and sharded aggregate rows to avoid counting duplicate callbacks, retried submissions, competing recovery passes, or a stale worker. Histograms are persisted bucket counts/sum/count, not repeated observations on every scrape. Administrative replays are new submissions; idempotent transport retries are not. This guarantees accounting for committed platform transitions, not exactly-once business side effects.

Definitions:

- Queue wait: submitted-to-first-claim time; one sample per first claimed job. Queue age includes queued/retrying records, with a separate ready-to-run depth so backoff is distinguishable.
- Attempt duration: assignment to persisted attempt termination, including interrupted attempts and cancellation, one sample per terminated attempt.
- Job duration: submission to authoritative terminal outcome, including all queue/backoff/attempt time, one sample per terminal job.
- Cancellation duration: first committed cancellation intent to terminal cancellation, excluding a success that committed first.
- Provisioning delay: committed instance demand to registered ready worker; API transport latency alone is not provisioning latency.

Current gauges include pre-existing records. Lifecycle counters/histograms start at activation of this schema feature and count new transitions thereafter; publish the accounting start timestamp and do not invent historical histogram samples. Backfill current aggregate state under migration coordination. Client expiry is counted once per expired lease generation, not on every poll; a newly acquired generation is independent. Stale-worker rejections affect bounded local rejection counters, not accepted lifecycle events.

Instrument process-local HTTP, database pool, storage calls, Kubernetes API calls, controller/recovery/cleanup loops, and standard Go/process runtime metrics separately. Instrument actual streaming completion/error, not just response-header creation. Local counters/histograms can reset with their process, which Prometheus rate functions handle.

### 5. HA aggregation

All replicas expose the same shared database metric families; local runtime families describe only the scraping target. This avoids a metrics-only leader election and remains usable through any healthy server. Every family in [metrics.md](metrics.md) declares its scope.

For shared gauges, deduplicate across `pod`/`instance`/endpoint labels with `max by (cluster,namespace,linha_deployment,<business dimensions>)`, then aggregate business dimensions. Samples are point-in-time observations, so small polling skew is possible. Never sum replica copies.

For durable shared counters and histogram buckets, deduplicate the monotonic series before applying `rate`/`increase`, using a PromQL subquery. For local counters/histograms, apply `rate` per target before summing. Keep per-server readiness and runtime graphs per pod. Do not combine unrelated Linha databases under one deployment label. [dashboard.md](dashboard.md) gives concrete expressions and no-data behavior. No recording-rule CRD is required; optional hand-written recording rules can use the same expressions.

Collection failures remove shared families and set explicit failure/freshness metrics. A dashboard must show unavailable when all copies disappear rather than invent healthy zero counts. Counter decreases caused by a restored older database are documented resets; server restarts do not reset durable accounting.

### 6. ServiceMonitor and GrafanaDashboard

Choose **ServiceMonitor**, using the existing server Service's named `http` port. Add stable metadata labels to that Service and match exactly the release/namespace. Prometheus discovers each selected endpoint; it must not scrape only the Service's load-balanced IP. Preserve discovery of unready endpoints to diagnose failed readiness. Configurable scrape interval defaults to 30s and timeout to 10s, with timeout shorter than interval.

Proposed Helm surface (defaults shown; details become values/schema during implementation):

```yaml
metrics:
  enabled: true
  collectionInterval: 10s
  collectionTimeout: 2s
  maxContextLabels: 100
  serviceMonitor:
    enabled: false
    labels: {}
    annotations: {}
    interval: 30s
    scrapeTimeout: 10s
    targetLabels: {}
    relabelings: []
    metricRelabelings: []
    sampleLimit: 0
    authorization: {}
    tlsConfig: {}
    scheme: http
    cluster: ""
grafana:
  dashboard:
    enabled: false
    labels: {}
    annotations: {}
    instanceSelector: {}
    allowCrossNamespaceImport: false
    folder: Linha
    resyncPeriod: 10m
    datasourceUid: ""
    uiBaseUrl: ""
```

`targetLabels` is an administrator-defined map translated to target relabelings; namespace/release/pod identity labels are reserved and cannot be overwritten. `cluster` must be set for a central multi-cluster datasource, either here or by consistent Prometheus external labels. Validate reserved-label conflicts and durations. Authorization/TLS values configure scraping when operators add transport protection; secrets are referenced, not placed in values as token strings. `serviceMonitor.enabled=true` requires metrics enabled. Disabling metric export does not disable the internal UI observations.

Render `monitoring.coreos.com/v1` ServiceMonitor and `grafana.integreatly.org/v1beta1` GrafanaDashboard independently, both in the release namespace. Require the matching CRD when its resource is enabled, with a useful error and documented `helm template --api-versions` flags for offline rendering. Disabled options render no custom monitoring resources and do not require either operator. The chart does not install operators, CRDs, Prometheus, Grafana, or new ClusterRoles.

GrafanaDashboard requires a nonempty instance selector and uses packaged JSON via `spec.json`; configure folder, datasource UID, cross-namespace import, labels, and resync period. The existing operator must watch the namespace and be allowed to manage its target Grafana. Check the chosen stable operator CRD fields during implementation. Also distribute the same dashboard JSON for manual import when no Grafana Operator is installed. No external JSON download or Grafana credentials are required by Linha.

### 7. Dashboard and UI coordination

[dashboard.md](dashboard.md) proposes the panels, variables, PromQL, and primitive links; [metrics.md](metrics.md) is the collector contract. The dashboard uses standard Grafana panels and a selectable Prometheus datasource. It shows deploy/server health, client/context demand, job flow/errors/duration, worker/engine capacity, Spark allocation, storage/results, and collector freshness.

The UI consumes the operational repository directly via its own authorized routes. Metrics are not parsed by the UI and are not the source of truth for replay/cancel. Both changes use one definition of active leases, current attempts, worker freshness, desired versus observed capacity, and last observation. Implement this read-model milestone before the UI's engine/server screens. Ship monitoring independently of enabling the UI; UI links are optional when `uiBaseUrl` is empty.

### 8. Existing metrics

Keep existing `linha_jobs{state}`, `linha_queue_oldest_seconds`, `linha_running_attempts`, `linha_expired_leases`, `linha_ready_workers`, `linha_worker_slots`, `linha_backend_conditions`, and `linha_retained_results` as deprecated aggregate gauge aliases for one release, preserving their meaning and label shape. Keep the old unlabeled HTTP counters too; new labeled HTTP families have distinct names. Document their scope and planned removal; do not emit labeled and unlabeled samples under one family with incompatible semantics. New dashboards use the catalog names exclusively.

## Risks / Trade-offs

- Shared totals double-counted across replicas → scope every family, deduplicate before rates, and validate queries against two-replica fixtures.
- Monitoring competes with request traffic → bounded polling, incremental/sharded aggregates, indexed snapshots, no per-scrape external calls, and load tests.
- Rich labels grow Prometheus storage → fixed dimensions, persistent capped context aliases, visible overflow, and cardinality tests.
- Engine/Kubernetes observations lag → distinguish stale/unknown from zero and keep timestamps; stale owners cannot overwrite current observations.
- Durable counters add transaction work → shard aggregates, verify concurrent duplicate transitions, and keep local transport metrics independent of durable accounting.
- External operator schemas differ → pin tested compatibility in documentation and validate CRDs/manifests; keep monitoring optional.

## Migration Plan

1. Coordinate additive read-model, heartbeat, label-registry, accounting, and expiry-marker schema migrations under the existing migration lock. Reserve schema versions before the UI change adds its own migrations.
2. Backfill current-state aggregates, initialize durable event accounting at a recorded activation time, and retain existing metric aliases.
3. Deploy the metrics-capable server with monitoring CRs disabled; verify raw exposition, durable-state parity, collector failure behavior, and namespace-only RBAC.
4. Enable ServiceMonitor and/or import the dashboard against caller-managed operators; verify actual per-pod discovery including unready replicas and dashboard deduplication.
5. Verify all-server restart, controller replacement, database/Kubernetes/storage outages, expired clients, stale worker callbacks, and concurrent identical ensure/submission.
6. Disable optional monitoring resources to roll back integration. Do not downgrade a migrated database automatically; follow the server's schema compatibility checks.

## References

- [Prometheus metric naming and labels](https://prometheus.io/docs/practices/naming/)
- [Prometheus instrumentation guidance](https://prometheus.io/docs/practices/instrumentation/)
- [Prometheus Operator ServiceMonitor discovery](https://prometheus-operator.dev/docs/developer/getting-started/)
- [Grafana Operator dashboard API](https://grafana.github.io/grafana-operator/docs/api/#grafanadashboardspec)
- [Spark Operator API](https://github.com/kubeflow/spark-operator/blob/master/docs/api-docs.md)
