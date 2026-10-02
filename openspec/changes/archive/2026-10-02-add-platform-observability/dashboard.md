# Proposed Grafana dashboard: Linha Operations

This document defines the dashboard to implement and package with Helm. It is not an installed dashboard. Use standard stat, time-series, table, and heatmap panels, UTC by default, a six-hour initial range, and a 30-second refresh. Distribute a standalone JSON file as well as the optional GrafanaDashboard resource.

## Variables and navigation

- `datasource`: selectable Prometheus datasource, optionally preset by Helm UID.
- `cluster`: optional deployment label; when no cluster labels exist, omit this filter rather than select an empty nonexistent series. Configure labels consistently for multi-cluster stores.
- `namespace`, `linha_deployment`: required cascading deployment filters.
- `engine`, `context`: business panels only, multi-select with All; context is the stable logical-context alias and includes `__other__` when present.
- `pod`: local server panels only, with All; filtering one pod must not remove shared totals.
- `state` and `outcome`: panel/table controls for jobs, not global filters on unrelated metrics.
- `uiBaseUrl`: optional dashboard constant; add encoded links to `/ui/contexts?metricContext=...` and `/ui/requests?metricContext=...&state=FAILED` when configured. The admin UI owns those filters and still requires authorization.

Variables come from catalog families and deployment labels. Mark overflow contexts as aggregated. Do not generate variables from user IDs, exception strings, request IDs, raw URLs, or configuration hashes. A companion caption identifies each row's applicable filters.

## Layout

| Row | Panels | Operator question |
| --- | --- | --- |
| Service health | Scrape targets up/down, ready/unready/stale servers, uptime/build table, failed collectors, snapshot freshness | Can Linha currently accept durable work, and is the displayed data current? |
| Clients and contexts | Active leases, acquisition/release/expiry rates, versions by state, demand from clients/work, provisioning conditions, alias overflow | Who still needs capacity, and which contexts are draining or unhealthy? |
| Requests | Queue depth and oldest age, ready versus backoff, running/cancelling, retained failed records, submissions and terminal outcomes over time, failure ratio by reason | Is work flowing, failing, or stuck? |
| Latency and recovery | Queue wait p50/p95/p99, job/attempt duration by outcome, cancellation delay, retries, expired attempts and rejected stale updates | Is delay in the queue or execution, and is recovery working? |
| Workers and engines | Fresh/draining/stale workers, occupied/available slots, desired versus ready/starting instances, provisioning delay, replacements, missing observations | Is execution capacity sufficient and current? |
| Spark | Current driver/executor pods by phase, configured min/initial/max executors, main-container CPU/memory requests and limits | Is Spark scaling as configured, and which pods are pending/failing? |
| Servers and API | Per-pod request/error rates and p95 latency, in-flight requests, CPU/RSS/goroutines/GC, database pool usage/wait, background-loop age/error rates, Kubernetes failures | Which server or dependency explains the problem? |
| Results and storage | Retained/expired result sets and bytes, result-storage errors/latency, cleanup backlog/runs, staging reservation ratio | Are outputs retained and accessible, and is cleanup falling behind? |

Spark's row is hidden or explicitly not applicable without Spark data. Configuration values are labeled “requests/limits”, never “usage”. Job duration is split by outcome so fast failures cannot masquerade as improved successful runtime. Live job progress and stack traces are UI links rather than per-job Prometheus time series. Queue/backoff panels remain visible when no driver exists.

## Aggregation conventions

Let deployment dimensions be `cluster,namespace,linha_deployment`. Absent cluster labels are allowed for a single-cluster datasource. Include all business dimensions in a shared-series deduplication before any further aggregation. The snippets below omit variable selectors for readability; implementation adds the appropriate deployment/context selectors to every metric expression.

Shared current job count (deduplicate replicas, then sum contexts/engines):

```promql
sum by (state) (
  max by (cluster,namespace,linha_deployment,engine,context,state) (
    linha_job_records
  )
)
```

Shared submission rate (deduplicate before rate; do not sum each replica's copy):

```promql
sum by (engine,context) (
  rate((max by (cluster,namespace,linha_deployment,engine,context,source) (
    linha_job_submissions_total
  ))[$__rate_interval:15s])
)
```

Durable terminal failure ratio (FAILED / all terminal outcomes; no terminal traffic shows N/A):

```promql
sum(rate((max by (cluster,namespace,linha_deployment,engine,context,outcome) (
  linha_job_completions_total{outcome="FAILED"}
))[$__rate_interval:15s]))
/
sum(rate((max by (cluster,namespace,linha_deployment,engine,context,outcome) (
  linha_job_completions_total
))[$__rate_interval:15s]))
```

Shared queue-wait p95 (deduplicate buckets before rate, then combine contexts):

```promql
histogram_quantile(0.95,
  sum by (le) (
    rate((max by (cluster,namespace,linha_deployment,engine,context,le) (
      linha_job_queue_wait_seconds_bucket
    ))[$__rate_interval:15s])
  )
)
```

Local HTTP throughput and p95 (rate each pod before aggregation):

```promql
sum by (route) (rate(linha_api_requests_total[$__rate_interval]))
histogram_quantile(0.95,
  sum by (route,le) (rate(linha_api_request_duration_seconds_bucket[$__rate_interval]))
)
```

Per-server readiness and collector age:

```promql
linha_server_ready
time() - linha_collector_last_success_timestamp_seconds
```

Worker slot utilization uses deduplicated occupied slots divided by deduplicated capacity slots; zero capacity displays N/A. It does not subtract instances in unrelated states or sum replica copies. Engine observation freshness must accompany pod counts; hide/mark incomplete totals when any required instance observation is stale/missing.

The dashboard sets a query minimum interval matching the configured scrape interval (30s by default), so `$__rate_interval` contains sufficient actual samples. The subquery step is no greater than that scrape interval. Validate expression generation for different configured intervals. Durable rates can bridge server restarts; local rates reset per process. Charts remain sampled observations, not an audit ledger.

## Empty and failure behavior

- A successful empty snapshot displays zero for known count series; a histogram without samples displays N/A.
- A failed scrape displays down; an HTTP-successful scrape with failed database collection displays reachable but unhealthy/unavailable.
- A missing collector family, failed collector, or snapshot older than three configured collection intervals marks associated rows unavailable. Do not use unconditional `or vector(0)` for failures.
- If all targets disappear from discovery, the dashboard reports no targets rather than zero healthy servers. Desired-replica count is optional external kube-state-metrics data, not guessed from the surviving targets.
- Deployment health requires both per-target scrape health and local readiness; shared heartbeat status supplies recently missing instances but is itself unavailable during a database outage.
- No live clients with queued work remains a valid draining workload. Spark links and metadata do not require Grafana to be installed.

## Optional integrations and validation

External Kubernetes/JVM resource dashboards can be linked when operators already run their exporters; core panels must work without them. No alert rules or notification routes are installed in this change. The dashboard documents suggested conditions for later alerts: no ready server, sustained queue age, zero ready capacity with runnable work, stalled reconciliation, persistent provisioning failures, and storage/collector failures.

During implementation, parse every generated expression after substituting variables and validate query fixtures with Prometheus tooling. Test two replicas exposing identical shared totals, one replica disappearing/restarting, local counter resets, simultaneous server restart with persistent counters, empty queues, context overflow, all shared collectors failing, and incomplete Spark observations. Confirm failed jobs are neither doubled by replicas nor confused with failed attempts that later succeed. Import the JSON into a disposable Grafana instance and reconcile the optional CR with the chosen Grafana Operator version before declaring it validated.
