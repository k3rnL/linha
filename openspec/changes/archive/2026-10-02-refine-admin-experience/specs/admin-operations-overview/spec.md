## Purpose

Give administrators a useful landing summary and distinguish current engine
capacity from retained instance observations during daily operations.

## ADDED Requirements

### Requirement: Overview landing page
The console SHALL open on an Overview page, retaining Contexts, Requests and
Servers navigation. It SHALL show complete current running/queued/cancelling
counts, success/failure outcomes over an explicitly labelled 24-hour window,
queue age, processing time, active clients/versions, ready worker capacity,
server freshness and bounded links to active contexts and recent failures.
The summary SHALL require admin read access and SHALL work without Prometheus.

#### Scenario: More requests than one list page
- **WHEN** there are more requests or contexts than one paginated page
- **THEN** overview totals cover the full database and only detail lists are capped

#### Scenario: Failed collection or no completed executions
- **WHEN** summary collection fails or there are no completed execution durations
- **THEN** unavailable or missing values are explicit instead of appearing as healthy zeros

#### Scenario: Restart and authorization
- **WHEN** a viewer reloads after server restart
- **THEN** persisted jobs determine the summary without acquiring client leases
- **AND** a caller without admin access cannot read the summary

### Requirement: Current capacity and retired history
The instance view SHALL independently paginate current instances and retired
history, showing retired entries only after an explicit expansion. Stopping
instances SHALL be identified in current capacity. Driver and executor CPU/memory
requests and limits SHALL be displayed in a compact table with stable units.
Historical or stale values SHALL be labelled as last known allocations with their
observation time, and SHALL NOT provide live Spark links for retired instances.

#### Scenario: Old active driver and many replacements
- **WHEN** a current driver is older than a full page of retired instances
- **THEN** current pagination still finds it independently of history

#### Scenario: Stale worker and retired driver
- **WHEN** a retired instance retains a RUNNING application snapshot or a current observation becomes stale
- **THEN** the view explains that the values are historical and does not present them as fresh running capacity
