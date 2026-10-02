## 1. Operational read model shared with the UI

- [x] 1.1 Define engine-neutral operational DTO/repository contracts for context/version/client/worker/instance/server detail and aggregates, including owner scope, desired/observed state, timestamps, freshness, and named engine links; align them with `add-admin-web-ui`.
- [x] 1.2 Coordinate sequential schema versions and add observation/heartbeat/client-last-renewal fields plus bounded paginated queries; preserve existing durable identity and context lifetime.
- [x] 1.3 Extend fenced engine observations with SparkApplication/driver metadata, current owned executor pods, main-container resources, and validated existing ingress/service links; keep non-Spark adapters supported.
- [x] 1.4 Persist observed snapshots under controller epoch/resource UID/incarnation fencing; implement stale/missing state and reject replaced-resource updates.
- [x] 1.5 Add server process heartbeat, readiness/background-loop detail, crash expiry, graceful shutdown state, and bounded stale-record cleanup without changing scheduling authority.
- [x] 1.6 Verify owner-isolated reads, concurrent identical ensure, client expiry with accepted work, stale-controller/worker updates, deleted executor history, and all-server restart; publish the usable read-model milestone for the UI change.

## 2. Metric dimensions and durable accounting

- [x] 2.1 Finalize the documented family/label/type registry from `metrics.md`, fixed reason/state mappings, histogram buckets, legacy aliases, and per-family shared/local scope.
- [x] 2.2 Implement the persistent capped logical-context alias registry with collision handling, overflow aggregation, configurable aliases/cap, stable rollout/restart behavior, and UI mapping.
- [x] 2.3 Add coordinated aggregate/accounting migrations, backfill current state, and record the event-accounting activation baseline without fabricating historical timings.
- [x] 2.4 Instrument committed submissions, authoritative terminal jobs, attempt termination, retry decisions, cancellation timing, and provisioning/replacement decisions with duplicate-safe durable counters/histograms.
- [x] 2.5 Account client acquisition/release/expiry once per generation, including reacquisition and concurrent expiry recovery; retain last-seen observations independently.
- [x] 2.6 Maintain query-efficient current job/queue/worker/context/result gauges and published-byte totals; verify state transitions, expiry/cleanup, and manifest byte accounting against authoritative rows.
- [x] 2.7 Add database tests for concurrent duplicate acceptance/completion, audit/admin-compatible submission hooks, cancellation races, stale worker rejection, multiple recovery controllers, and full restart with counters/histograms unchanged.

## 3. Prometheus endpoint and local instrumentation

- [x] 3.1 Add typed Prometheus Go collectors, Go/process runtime collectors, explicit HELP/TYPE, and an optional metrics-export switch independent of internal observations.
- [x] 3.2 Add bounded cached database/readiness/engine-snapshot collection with timeouts, success/error/last-success metrics, cold-start behavior, fresh-zero emission, and failed-family omission while local metrics remain available.
- [x] 3.3 Instrument normalized HTTP routes/methods/status, in-flight requests, full streaming duration/bytes, and rejected worker updates without raw paths or arbitrary labels.
- [x] 3.4 Instrument database pool use/wait/failures, background-loop timing/last success, and bounded Kubernetes operation latency/errors.
- [x] 3.5 Instrument server-observed storage operations/transfers, cleanup operations/backlog, and staging reservations; document exclusion of direct worker/Spark traffic.
- [x] 3.6 Preserve existing metric aliases/HTTP family shapes and document compatibility; validate exposition with Prometheus tooling and fixed-family fixtures.
- [x] 3.7 Measure series cardinality and collection/query cost using default-cap, overflow, large retained-history, arbitrary-path/error, and concurrent-scraper fixtures; confirm failed monitoring cannot exhaust request resources.

## 4. Dashboard and Helm integrations

- [x] 4.1 Implement the standalone Linha Operations dashboard JSON from `dashboard.md`, including cascading filters, each proposed row, optional UI links, unknown/incomplete states, and applicable-filter captions.
- [x] 4.2 Validate generated PromQL with variable substitution and HA fixtures: duplicate shared snapshots, local counter resets, replica loss, empty queues, no samples, collector failures, and stale engine observations.
- [x] 4.3 Add stable Service metadata labels and optional ServiceMonitor values/schema/template selecting the named HTTP endpoint and each replica; support scrape/label/transport settings and namespace-only operation.
- [x] 4.4 Add independently optional GrafanaDashboard values/schema/template with packaged JSON, instance selector, folder, datasource UID, cross-namespace import, and resync options.
- [x] 4.5 Validate monitoring-disabled and independent/both-enabled Helm combinations with the chosen CRD versions, clear missing-CRD errors, offline rendering flags, reserved-label checks, and no new ClusterRoles.
- [x] 4.6 Verify live ServiceMonitor discovery including an unready replica and import/reconcile the dashboard in disposable Prometheus/Grafana/operator fixtures; document the tested versions.

## 5. HA, outage, and operational acceptance

- [x] 5.1 Verify two replicas report one logical job/queue total, distinct server runtime, and correct shared counter/histogram rates before/after one-replica replacement and full server restart.
- [x] 5.2 Inject database, Kubernetes API, and result-store failures; verify reachable metrics, honest collector/readiness state, stale engine detail, result error attribution, and recovery without lost acknowledged work.
- [x] 5.3 Verify real Spark demand/allocation observations with multiple drivers, dynamic executor scale-down, lost-driver replacement, client expiry/drain, and existing UI ingress metadata.
- [x] 5.4 Document metric families/labels/scopes, context cap/overflow, accounting baseline, HA PromQL, dashboard import, optional operators, namespace selection, scrape access, migration/rollback, and validation limits.
- [x] 5.5 Update CI and acceptance mapping for meaningful Go/PostgreSQL/exposition/query/Helm checks; run strict OpenSpec validation and record measured evidence before completing tasks.
