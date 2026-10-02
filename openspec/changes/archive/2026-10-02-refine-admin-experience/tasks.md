## 1. Durable operational data

- [x] 1.1 Add schema 10, optional hostname contracts/persistence and automatic JVM client reporting; preserve legacy requests and lease/version semantics.
- [x] 1.2 Implement the authorized, consistent overview API with complete counts, recent outcomes/timing, bounded details and indexed queries.
- [x] 1.3 Add independently paginated current/historical instance filters and update OpenAPI contracts.

## 2. Console and dashboard

- [x] 2.1 Build the Overview landing page with useful links, loading/error/empty states and read-only visibility-aware polling.
- [x] 2.2 Separate current/stopping and collapsed retired instances, improve allocation tables, and display hostname above client ID.
- [x] 2.3 Curate Grafana activity/timing/capacity/resource panels with clear labels and collapsed diagnostics; preserve HA/filter/freshness correctness and Helm links.

## 3. Verification and delivery

- [x] 3.1 Verify PostgreSQL migration/metadata/overview/filter behavior, auth/OpenAPI, SDK lease/reacquisition, browser flows and generated PromQL/Helm output.
- [x] 3.2 Run formatting, Go/DB race, JVM package, build and restart checks; update docs/validation and build the updated server image.
