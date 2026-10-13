# Monitor queries and resources

Use Prometheus for trends and Grafana for the operational dashboard. Use the
administrator console for exact requests, exceptions and instance details.
Linha exports `/metrics`; `/livez` and `/readyz` are separate probes.

## 1. Enable per-replica scraping

The metrics route is enabled by default. With Prometheus Operator installed:

```yaml
metrics:
  enabled: true
  cluster: production
  serviceMonitor:
    enabled: true
    labels:
      release: kube-prometheus-stack
    interval: 30s
    scrapeTimeout: 10s
    sampleLimit: 100000
```

Set labels that match your Prometheus ServiceMonitor selector. The resource
selects the release Service's named HTTP port and discovers each server Pod
independently. It adds no Linha ClusterRole. Keep scrape access inside your
operational network; the metrics route does not use application OAuth or admin
sessions.

Without the operator, configure Prometheus to scrape each Pod rather than one
load-balanced Service address. Cluster, namespace, deployment and Pod labels
come from scrape discovery.

## 2. Install the dashboard

Import [the dashboard JSON](https://github.com/k3rnL/linha/blob/main/deploy/helm/linha/dashboards/linha-operations.json)
into Grafana and choose its Prometheus datasource. Alternatively, with Grafana
Operator installed:

```yaml
metrics:
  grafanaDashboard:
    enabled: true
    instanceSelector:
      matchLabels:
        dashboards: platform
    folder: Linha
    datasourceUID: prometheus
    allowCrossNamespaceImport: false
```

ServiceMonitor and GrafanaDashboard are independently optional. Enabling either
requires its installed CRD. For offline Helm rendering, pass the corresponding
`--api-versions` flag. Set `ui.grafanaURL` to link from the console.

## 3. Read the dashboard

| Question | Relevant panels |
| --- | --- |
| Is work moving? | Running, queued, succeeded and failed queries; completion rate and success rate |
| Where is time spent? | Queue wait, attempt processing, end-to-end duration and oldest queued request |
| Do we have capacity? | Connected clients, active versions, ready workers and occupied/available slots |
| What resources are allocated? | Running drivers/executors; CPU and memory requests/limits |
| Are measurements trustworthy? | Ready servers, collector age, API/dependency failures and observation completeness |

Filter by cluster, namespace, deployment, engine and context. Completed outcomes
refer to the selected time range, not the lifetime count of retained job records.
Success rate excludes cancellation. Processing time measures completed attempts;
retries count separately. End-to-end time includes queueing and retries.

CPU/memory panels show **allocations**, not live usage. Use your existing container
metrics source for usage. Retired instances are excluded; missing or stale engine
observations make the total unavailable rather than showing a partial healthy sum.

## 4. Write correct HA queries

All server replicas export the same durable business state. Deduplicate those
copies before summing or taking rates. Local HTTP/runtime counters belong to each
process; calculate each process rate and then sum.

Running requests across the selected deployment:

```promql
sum(max by (cluster, namespace, linha_deployment, engine, context, state) (
  linha_job_records{state="RUNNING"}
))
```

Ready and backoff work in the queue:

```promql
sum by (eligibility) (
  max by (cluster, namespace, linha_deployment, engine, context, eligibility) (
    linha_queue_jobs
  )
)
```

Failed jobs over five minutes:

```promql
sum(increase((
  max by (cluster, namespace, linha_deployment, engine, context, outcome) (
    linha_job_completions_total{outcome="FAILED"}
  )
)[5m:15s]))
```

p95 processing duration of completed attempts:

```promql
histogram_quantile(0.95, sum by (le) (rate((
  max by (cluster, namespace, linha_deployment, engine, context, le) (
    linha_attempt_duration_seconds_bucket
  )
)[5m:15s])))
```

Local API error rate:

```promql
sum(rate(linha_api_requests_total{status_code=~"5.."}[5m]))
```

Apply the same deployment filters to every expression when several deployments
share Prometheus. Shared histogram deduplication must include `le`. The complete
[metric reference](metrics.md) identifies each family's scope and labels.

## Diagnose missing data or excess API traffic

Database snapshots refresh every ten seconds with a two-second timeout. Scrapes
read a cache and do not call PostgreSQL, Kubernetes or storage. A failed collection
omits shared families and still returns local metrics with HTTP 200; it does not
pretend the queue is empty. Check collector success and last-success timestamps.

Context labels are lifetime-bounded: the first `metrics.maxContexts` owner/name
dimensions get stable labels, and further names aggregate as `__other__`.
No job ID, exception text, owner, result URI or version hash becomes a metric label.

Active capacity reconciles frequently. Retired engine resources are inventoried
once per minute per replica, independent of dead history size. In an empty namespace
this costs three collection requests per minute per replica. Repeated individual
GET/DELETE 404s from an idle deployment can indicate an older replica still running.
Inspect the process-local `linha_kubernetes_api_requests_total` rate by
`resource`, `verb` and `outcome`.

Engine cleanup is automatic; result-file cleanup is a separate opt-in setting.
See [result retention](retention-proposal.md) and [validation](admin-observability-validation.md).
