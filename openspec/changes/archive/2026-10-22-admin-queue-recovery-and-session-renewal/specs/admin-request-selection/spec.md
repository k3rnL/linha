## Purpose

Make global and context-version request tables usable for locating and recovering selected accepted work.

## ADDED Requirements

### Requirement: Shared request selection and batch controls
Both request views SHALL provide operator row checkboxes, selection of the current page, selected counts, cancellation confirmation and a move dialog with compatible destination versions. The UI SHALL state the selection scope, clear selection when filters or pages change, prevent duplicate pending submissions and display each batch outcome. Viewer tables SHALL remain read-only. Only pending requests SHALL be offered for movement, and failed entries SHALL remain selected for review or retry.

#### Scenario: Move selected requests from a context version
- **WHEN** an operator selects queued rows and confirms an active destination version
- **THEN** the UI sends their IDs and expected context IDs, reports outcomes and refreshes both queue and context observations

#### Scenario: Viewer or changed page
- **WHEN** a viewer opens a table or an operator changes its filters or page
- **THEN** viewer mutation controls are absent and selection cannot silently apply to rows outside the displayed page

### Requirement: Searchable context and date filters
Context filters SHALL suggest existing contexts while permitting text entry. Request date filters SHALL use local date/time inputs translated into timezone-aware API values and SHALL validate reversed ranges before querying. Filter choices SHALL include retained context versions so stranded historical queues remain discoverable.

#### Scenario: Locate an old queue within a date range
- **WHEN** an administrator searches a context name and picks local start/end times
- **THEN** available context choices are shown, the request query uses the selected context filter and UTC timestamps, and the table updates without manual RFC3339 entry
