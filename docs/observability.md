# Observability

Use `/livez` for process liveness, `/readyz` for database/schema and configured
local mount readiness, and `/metrics` for Prometheus. There is no `/health`
metrics endpoint. `metrics.enabled` defaults to true; disabling it removes metric
export but preserves operational observations, server heartbeats and admin views.

The [metric catalogue](metrics.md) defines all families, types, scopes, fixed
labels, reasons and histogram boundaries. Go/process collectors are included.
Request IDs, version hashes, owners, arbitrary paths, selectors, exception text
and result URIs are never metric labels.

## Collection and failure behavior

One background loop collects a coherent, read-only database snapshot every ten
seconds, with a two-second timeout. Scrapes only read the cached snapshot and
process collectors. They do not call PostgreSQL, Kubernetes or result stores.
Scrapes allow at most eight concurrent requests and have a five-second timeout.
A failed database collection omits shared families while serving local metrics
with HTTP 200 and collector success=0. Cold collectors have last-success=0;
unknown data is never substituted with a healthy zero.

Server heartbeats are informational, not scheduling authority. They record
process/pod/version, readiness and background-loop summaries every ten seconds,
become stale after thirty seconds, and are removed after twenty-four hours.
Graceful shutdown records stopping. Kubernetes observations are stored under
controller epoch/application UID/pod incarnation fencing. Spark totals include
only current owned executor pods. Missing resource quantities mark the snapshot
incomplete; stale/missing observations mask Spark totals on the dashboard.

Job acceptance, terminal transitions, first assignment, attempts, retries,
cancellation, client lease generations and instance readiness/replacement are
accounted in the same winning database transaction. Duplicate HTTP callbacks,
idempotent submissions and reconciliation polls do not create duplicate events.
Counters/histograms survive all-server restart. Migration records an accounting
baseline; existing history seeds current gauges, without fabricated timings.

HTTP metrics normalize routes and methods. Pool errors distinguish timeout,
cancel and error; loop and Kubernetes labels use fixed sets. Storage metrics
cover server-observed upload/download streams and publish/delete/delegation/probe
operations. Transfer bytes count those upload/download streams, excluding internal
provider copies/verification reads and direct worker/Spark I/O. Dataset logical
published bytes come from the sealed manifest, independent of transport traffic.
Staging reservations and cleanup backlog/attempts are separate. Cleanup remains
opt-in; scrape/export settings do not enable deletion.

## Context labels and HA PromQL

A persisted registry admits 100 logical owner/name dimensions by default, stable
across configuration versions and restart. Further names aggregate as
`context="__other__"`. Assignment is lifetime-bounded rather than reused after
shutdown. `metrics.maxContexts` accepts 0–10,000; increasing it admits future
names and does not relabel previously enrolled overflow names. Optional
`metrics.contextAliases` entries contain owner, name and alias; configure them
before first enrollment. Assigned aliases cannot be renamed. The admin API
exposes `metricContext` for drill-through. Keep cap/overrides identical on replicas.

Shared state (**S**) and observations (**O**) appear on every server. Deduplicate
by deployment and all business dimensions *before* summing or taking rates:

```promql
sum(max by (cluster, namespace, linha_deployment, engine, context, state)
  (linha_job_records{state="RUNNING"}))

sum(rate((max by (cluster, namespace, linha_deployment, engine, context, source)
  (linha_job_submissions_total))[5m:15s]))

sum(rate(linha_api_requests_total[5m]))
```

The last query is local (**L**): calculate rates for each process series first,
then sum. Histograms follow the same rule, including `le` in shared bucket
deduplication. Never sum shared server copies, average counts, or deduplicate
local counters. Scope deployments when using these examples.

## Helm integrations

Neither monitoring resource is enabled by default. Both are independently
optional and add no Linha ClusterRole. A ServiceMonitor selects the release's
labelled Service and named HTTP port, discovering each pod independently,
including unready pods while their metrics endpoint is reachable.

```yaml
metrics:
  enabled: true
  cluster: production
  maxContexts: 100
  serviceMonitor:
    enabled: true
    labels:
      release: kube-prometheus-stack
    interval: 30s
    scrapeTimeout: 10s
    sampleLimit: 100000
  grafanaDashboard:
    enabled: true
    instanceSelector:
      matchLabels:
        dashboards: platform
    folder: Linha
    datasourceUID: prometheus
    allowCrossNamespaceImport: false
    resyncPeriod: 10m
```

Use a Prometheus selector that matches your ServiceMonitor labels and namespace;
its service account needs discovery access in the release namespace. Stable
scrape labels are namespace/service/pod/instance/linha_deployment, plus cluster
when configured. Supported options include relabelings, metricRelabelings,
scheme, tlsConfig and authorization. CRD transport fields are passed through;
configure a proxy/service mesh if your deployment terminates HTTPS externally.
The server metrics route itself has no SDK/admin authentication. Restrict scrape
network access using your existing network policy or proxy.

Install Prometheus Operator (`monitoring.coreos.com/v1`) before enabling
ServiceMonitor, and Grafana Operator (`grafana.integreatly.org/v1beta1`) before
enabling GrafanaDashboard. Missing APIs produce a clear Helm rendering error.
For offline rendering supply `--api-versions monitoring.coreos.com/v1/ServiceMonitor`
and/or `--api-versions grafana.integreatly.org/v1beta1/GrafanaDashboard`.
Timeout must not exceed interval; chart values use positive integer seconds.
Reserved selector labels cannot be overridden by user monitoring labels.

## Dashboard import

The standalone dashboard is
[linha-operations.json](../deploy/helm/linha/dashboards/linha-operations.json).
Import it through Grafana's dashboard import page and select your Prometheus
datasource. Alternatively enable GrafanaDashboard with an instance selector and
datasource UID. The chart adds console links only when the UI is enabled.

The default view has 17 panels arranged around three questions:

- **Queries:** running and queued now, succeeded and failed during the selected
  time range, success rate, ready servers, activity over time and completions/minute.
- **Time spent waiting and processing:** average and p95 execution time, median
  and p95 queue wait, end-to-end request time, and the oldest queued request.
- **Capacity and resources:** client connections and context versions, ready
  workers and execution slots, running Spark drivers/executors, CPU requests/limits
  and memory requests/limits.

Four diagnostic panels remain in a collapsed row: server readiness, monitoring
data age, API failures and dependency errors. All exported metrics remain available
for custom dashboards; simplifying the default view does not remove instrumentation.

Cascading variables filter cluster, namespace, deployment, engine and context.
Server health is deployment-wide, as its caption explains. The server-pod variable
is hidden and applies only to diagnostics. Chart-managed console links open the
Overview, Contexts and Requests pages. Aggregated series do not carry a single
context identity, so they do not link to an arbitrary context.

Outcome totals are scrape-based estimates for the selected dashboard period,
not lifetime totals. Success rate excludes cancellations. Processing time measures
finished attempts (retries count separately); end-to-end time includes queueing
and retries. No completed executions means no duration/percentage. Oldest queue
age uses the maximum across contexts. Shared queries deduplicate all business
dimensions before aggregation/rate; local event rates are calculated per replica.

Pod requests/limits are **allocations, not actual resource usage**. Measuring usage
requires an external container metrics source. Retired instances do not contribute.
If any required observation in the selected scope is stale or incomplete, the
resource total is unavailable rather than a sum of just the healthy instances.

Regenerate using `python3 scripts/generate-dashboard.py`. Validate layout, all
40 expressions and HA/reset/outage fixtures, including the actual generated range
and allocation queries, with `python3 scripts/check-observability.py` (Docker and
Prometheus 3.7.1). Dashboard UID `linha-operations` is preserved; this layout is
dashboard version 2.

## Compatibility

For one release the old names/shapes remain: unlabeled
`linha_http_requests_total`, `linha_http_errors_total`, and shared
`linha_jobs{state}`, `linha_queue_oldest_seconds`, `linha_running_attempts`,
`linha_expired_leases`, `linha_ready_workers`, `linha_worker_slots`,
`linha_backend_conditions`, `linha_retained_results`. The running-attempt alias
includes expired attempts; expired leases are its subset. Prefer typed families
for new dashboards. These aliases are shared too and require HA deduplication.

Migrations 7–9 add the read model, metric accounting and admin sessions/audit.
Migration 10 adds client hostnames and indexes for the admin Overview.
Back up PostgreSQL and deploy the matching server/chart. Migration is serialized
and preserves existing ownership, IDs, policy snapshots and result paths.
Exposure rollback means disabling the UI/monitoring; do not automatically
downgrade schema or start an older binary that rejects schema 10. These changes
have bounded fixture evidence, not a production capacity guarantee. See
[validation](admin-observability-validation.md).
