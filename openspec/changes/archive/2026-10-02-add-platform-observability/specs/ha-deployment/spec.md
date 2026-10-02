## MODIFIED Requirements

### Requirement: Operational visibility and configuration safety
Deployment SHALL expose correlation by context/job/attempt, queue age/depth, worker health/capacity, retry/lease events, provisioning failures, and result errors. Logs SHALL avoid recording secret credentials or entire request/result payloads by default. Typed `/metrics` exposition SHALL additionally cover clients, contexts, job outcomes/timings, engines, results/storage, and each server's runtime/readiness/background work with bounded filterable dimensions. Shared totals SHALL remain correctly interpretable across replicas, and failed/stale collection SHALL be distinguishable from successful zero values. Operational read models and monitoring SHALL preserve existing ownership, attempt fencing, and lifecycle rules.

#### Scenario: Backend repeatedly fails to start
- **WHEN** a worker image cannot initialize after bounded retries
- **THEN** operators can identify the affected context and provisioning cause without inspecting private request payloads

#### Scenario: Observe an HA deployment
- **WHEN** multiple Linha replicas export the same durable request state and distinct local runtime metrics
- **THEN** documented dashboards deduplicate shared totals and retain per-server health and local traffic attribution

#### Scenario: Persistence unavailable during observation
- **WHEN** persistence fails while a server's HTTP process remains alive
- **THEN** local diagnostic metrics remain available, shared collection is marked unavailable, and liveness remains independent from readiness
