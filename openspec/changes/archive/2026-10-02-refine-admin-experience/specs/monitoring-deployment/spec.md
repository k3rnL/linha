## MODIFIED Requirements

### Requirement: Useful filtered operational dashboard
The packaged dashboard SHALL prioritize current query activity, selected-period outcomes, processing and queue timings, worker capacity, and Spark resource requests/limits. Low-level server, collector and dependency diagnostics SHALL be collapsed by default. It SHALL filter by applicable cluster/namespace/deployment, engine/logical-context, and server dimensions; provide optional admin UI links; and apply each family's correct HA aggregation. It SHALL distinguish unavailable data, successful zero counts, and incomplete engine observations. The full documented metrics export SHALL remain available for custom dashboards.

#### Scenario: Shared counts and local traffic
- **WHEN** the same durable jobs are visible through multiple replicas while each handles different HTTP traffic
- **THEN** dashboard job totals are deduplicated and local traffic rates are combined correctly

#### Scenario: Dependency and engine observations fail
- **WHEN** shared collection is unavailable or some current engine observations are stale
- **THEN** affected panels display unavailable/incomplete state rather than zero jobs or deceptively complete pod counts

#### Scenario: Context drill-through
- **WHEN** the UI base URL is configured and an administrator follows a console link
- **THEN** Overview, Contexts or Requests opens with its own authorization checks, and aggregated panels do not invent a single-context filter
