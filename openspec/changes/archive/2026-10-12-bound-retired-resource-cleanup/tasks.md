## 1. Engine resource discovery

- [x] 1.1 Add engine resource inventory and UID-guarded deletion contracts, including Kubernetes namespace/label filtering, pagination and legacy Service cleanup; verify with HTTP adapter tests.

## 2. Controller cleanup

- [x] 2.1 Replace per-dead-instance polling with a minute sweep and exclude stopped contexts from frequent reconciliation; keep cleanup errors observable and bounded.
- [x] 2.2 Verify PostgreSQL-backed restart/failover, lease fencing, late orphan removal, active/foreign identity protection and prompt context reactivation; assert historical instance count does not increase Kubernetes calls.

## 3. Validation and documentation

- [x] 3.1 Document the operational behavior and production findings without secrets; run formatting, Go checks with PostgreSQL integration tests, race tests and strict OpenSpec validation.
