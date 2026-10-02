## Why

The current `/metrics` endpoint exposes only a few unlabeled totals and cannot explain client demand, failed work, engine capacity, or the health of each HA server. Operators need filterable, correctly aggregated metrics and a ready-to-import Grafana dashboard; the admin UI also needs the same reliable live observations without depending on a monitoring stack.

## What Changes

- Expand the existing `/metrics` Prometheus endpoint using typed collectors, documented metric names, bounded labels, and explicit local-versus-shared aggregation semantics. Keep `/livez` and `/readyz` as health probes; there is no existing `/health` route to extend.
- Cover client leases, context versions, queued/running/terminal jobs, attempts/retries/cancellation, worker freshness and slots, managed engine capacity, Spark driver/executor state, provisioning, results/storage, HTTP, database connections, and server/controller health.
- Add an engine-neutral operational read model, fenced engine snapshots, and server heartbeat records usable by both metrics and the admin APIs. Live details include existing Spark UI ingress links and freshness indicators.
- Preserve shared durable counters/histograms across server restarts and avoid multiplying shared database counts by the number of scraped replicas.
- Provide a bounded context-label registry for useful Grafana filtering without labeling by job, client, worker, version hash, exception text, or other unbounded identifiers.
- Propose a Linha Operations Grafana dashboard with explicit HA aggregation, deployment/context filters, and drill-through to the admin console.
- Add independently optional Helm `ServiceMonitor` and `GrafanaDashboard` resources. Existing Prometheus Operator and Grafana Operator installations supply their CRDs and controllers; Linha adds no cluster-scoped permissions.

## Capabilities

### New Capabilities

- `operational-read-model`: Shared context/client/worker/server observations and engine-specific snapshots, including freshness, lifecycle explanations, and Spark ingress links.
- `prometheus-observability`: Metric families, bounded labels, durable lifecycle accounting, outage behavior, and HA-safe aggregation.
- `monitoring-deployment`: Optional ServiceMonitor and GrafanaDashboard resources, standalone dashboard JSON, Helm configuration, and deployment validation.

### Modified Capabilities

- `ha-deployment`: Specify reliable operational visibility across replicas and persistence outages, linked to the new observability capabilities.

## Impact

- Go instrumentation and operational repository interfaces, controller/engine observations, PostgreSQL migrations and aggregate accounting, metrics documentation, and performance/failure tests.
- Helm values/schema/templates and packaged dashboard JSON; external monitoring operators remain caller-managed and optional.
- `add-admin-web-ui` consumes the internal read model and owns its admin HTTP authorization and screens. Prometheus collectors do not expose an unauthenticated detail API.
- Existing metrics receive documented compatibility handling; ordinary SDK ownership, request acknowledgement, attempt fencing, result durability, and engine lifecycle remain authoritative.
