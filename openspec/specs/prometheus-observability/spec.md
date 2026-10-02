# prometheus-observability Specification

## Purpose

Provide useful, bounded-cardinality Prometheus measurements of Linha demand, execution, capacity, results, and server health with correct durability and HA aggregation semantics.

## Requirements

### Requirement: Typed metrics endpoint and health separation
Linha SHALL expose documented Prometheus metric families at `/metrics`, with HELP/TYPE metadata, valid escaping, consistent units, and an explicit scope for each family. `/livez` and `/readyz` SHALL remain separate health probes. When metrics are disabled, internal operational observations SHALL still support the admin UI.

#### Scenario: Parse an ordinary scrape
- **WHEN** a scraper requests metrics from a healthy server
- **THEN** the response is valid Prometheus exposition with documented types and no conflicting family definitions

#### Scenario: Metrics disabled and UI enabled
- **WHEN** metric export is disabled while the administrator console is enabled
- **THEN** administration can still obtain operational observations without a Prometheus endpoint

### Requirement: Coverage of demand and execution
Metrics SHALL cover stored client-lease states/events, context versions/demand/conditions, queue depth/eligibility/age, job states/submissions/terminal outcomes/failures, attempts/retries/cancellation, and execution timings. Current-state gauges SHALL be distinguished from event counters and cumulative histograms. Failed attempts followed by success SHALL NOT be counted as terminal failed jobs.

#### Scenario: Retry succeeds
- **WHEN** a job's first attempt fails and a later permitted attempt succeeds
- **THEN** metrics show the failed attempt and retry but one successful job outcome and no terminal job failure

#### Scenario: Last client expires with queued work
- **WHEN** the final client lease expires while accepted work remains
- **THEN** live client demand drops while queue/work demand remains observable until the accepted work finishes

### Requirement: Worker engine result and server coverage
Metrics SHALL cover worker freshness/claim capacity, desired and observed instance state, Spark driver/executor counts and resource configuration, provisioning/replacement, published results/retention/cleanup, storage errors/latency, and per-server runtime/HTTP/database/background-loop health. Unsupported or unobserved resource usage SHALL NOT be represented as measured usage. Server-observed storage traffic SHALL be distinguished from direct worker I/O.

#### Scenario: Running pod has no registered worker
- **WHEN** a Spark pod is running but its SDK worker has not registered or its heartbeat is stale
- **THEN** pod state and usable worker capacity remain distinct in metrics

#### Scenario: Results survive engine shutdown
- **WHEN** a context stops but published outputs are retained
- **THEN** result metadata totals remain observable independently of current driver/executor counts

### Requirement: Bounded useful dimensions
Metrics SHALL support deployment/namespace/server, engine, logical-context, and applicable state/outcome/operation filters using bounded labels. Logical-context labels SHALL be stable across versions/restarts and distinguish different owners without publishing raw owner identities. Default named-context enrollment SHALL be capped at 100 with visible overflow aggregation; excess contexts SHALL remain included in totals. IDs, arbitrary handler/error text, credentials, raw URLs, and file paths SHALL NOT be metric labels.

#### Scenario: Rolling versions and duplicate context names
- **WHEN** one logical context gains new configuration versions and another owner uses the same name
- **THEN** versions reuse their logical-context dimension while the other owner remains a distinct named group within the configured budget

#### Scenario: Cardinality budget is exceeded
- **WHEN** more logical contexts exist than the configured named-context cap
- **THEN** additional contexts appear in the reserved overflow aggregate, an overflow indicator is exposed, and no work is omitted from platform totals

#### Scenario: Unbounded request paths and business errors
- **WHEN** requests contain arbitrary job IDs, paths, methods, or worker exception codes
- **THEN** route/method/reason labels normalize to bounded categories and do not grow with those values

### Requirement: Durable committed lifecycle accounting
Shared lifecycle counters/histograms SHALL account once for each committed platform transition, survive server/controller restarts, and exclude rejected stale or duplicate transitions. Submission acknowledgement and accounting SHALL not disagree about whether a new request committed. Historical state SHALL remain queryable, and the accounting activation baseline SHALL be explicit rather than inventing pre-feature event timings.

#### Scenario: Concurrent idempotent submission
- **WHEN** two replicas accept identical submissions with one scoped key
- **THEN** they return one job and increment the new-submission counter once

#### Scenario: Duplicate completion or stale worker
- **WHEN** a successful completion is repeated or a fenced-out worker reports another outcome
- **THEN** only the authoritative committed transition contributes to job/attempt counters and histograms

#### Scenario: Crash and recovery race
- **WHEN** all servers restart and multiple recovery controllers compete to resolve an expired attempt
- **THEN** one winning transition updates durable totals once and counters do not reset with process lifetimes

### Requirement: HA-safe metric interpretation
Each metric family SHALL declare whether it is shared durable state, shared engine observation, or local process instrumentation. Dashboard/query examples SHALL deduplicate shared replicas before summing gauges or applying rates to shared counters/histograms. Local counter rates SHALL be computed per process before summation.

#### Scenario: Two replicas expose the same queue
- **WHEN** two healthy servers each export a shared queue of ten requests
- **THEN** the supplied dashboard displays ten requests, not twenty

#### Scenario: Replica replacement
- **WHEN** a server restarts while durable counter state survives
- **THEN** shared query totals remain correct and process-local counter resets do not create false traffic spikes

### Requirement: Failure-visible bounded collection
Scrapes SHALL remain available with local health and collector failure/freshness metrics during dependency failures. Unavailable shared families SHALL be omitted rather than replaced with healthy-looking zeros. Collection SHALL have bounded time/resource use, reuse snapshots, and avoid full-history scans or external engine calls per scrape.

#### Scenario: Database outage
- **WHEN** PostgreSQL collection fails while the HTTP server is reachable
- **THEN** `/metrics` returns valid local metrics and failed-collector indicators, shared database metrics are unavailable, and readiness reflects the dependency failure

#### Scenario: Empty queue after a successful collection
- **WHEN** collection succeeds and no requests are queued
- **THEN** known queue count and age series report zero rather than an outage/no-data state

#### Scenario: Many simultaneous scrapers
- **WHEN** many scrapes arrive concurrently against a large retained history
- **THEN** bounded cached collection serves them without one history scan or Kubernetes request per scrape

### Requirement: Documented metrics compatibility
Existing published gauge aliases and unlabeled HTTP counters SHALL retain their prior label shape and meaning for a documented transition release. New labeled families SHALL use distinct names where retaining old definitions would otherwise conflict. Metric documentation SHALL define labels, scope, units, timing, histogram boundaries, overflow, and migration behavior.

#### Scenario: Existing dashboard scrapes an upgraded server
- **WHEN** an existing integration requests a legacy metric during the compatibility release
- **THEN** the family retains its previous definition while the new dashboard uses the new documented families
