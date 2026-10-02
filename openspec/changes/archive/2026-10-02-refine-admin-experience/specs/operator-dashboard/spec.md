## Purpose

Help operators understand current query activity, outcomes, time spent waiting
and processing, and engine capacity without reading internal metric vocabulary.

## ADDED Requirements

### Requirement: Query-focused dashboard
The default dashboard SHALL prioritize running and queued queries, successes and
failures in the selected time range, success rate, throughput, processing and
queue duration, oldest queued age, clients, workers, servers, and Spark resources.
Panel labels SHALL identify current values versus selected-window outcomes and
explain processing versus end-to-end duration in plain language. Low-level
collector and dependency diagnostics SHALL be collapsed by default.

#### Scenario: Everyday query monitoring
- **WHEN** an operator opens the dashboard
- **THEN** query counts, failures, timing and capacity are visible before internal diagnostics

### Requirement: Honest aggregation and resource meaning
Queries SHALL preserve deployment/context filters and HA deduplication for shared
state and events. Multiple contexts' oldest queue ages SHALL use the maximum,
not a sum. Allocated CPU/memory SHALL be labelled as requests/limits rather than
actual consumption, and stale or incomplete observations SHALL not look complete.

#### Scenario: Duplicate replicas and selected time range
- **WHEN** two replicas expose the same committed events and one restarts
- **THEN** shared counts and durations remain deduplicated, and selected-window outcomes exclude older events

#### Scenario: Missing resource observations
- **WHEN** a selected engine has stale or unavailable resource observations
- **THEN** affected totals are unavailable or explicitly incomplete rather than silently undercounted
