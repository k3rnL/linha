## 1. Spark provisioning

- [x] 1.1 Add validated application launch settings and SparkApplication-style resources with legacy parsing.
- [x] 1.2 Provision and observe SparkApplications with native templates and ownership/UID-fenced teardown.

## 2. Versioned lifetime

- [x] 2.1 Migrate persisted context versions, client leases and logical-name idempotency; add owner-scoped HTTP lifecycle endpoints.
- [x] 2.2 Reconcile live, draining and stopped versions safely across controller failover, preserving accepted work.
- [x] 2.3 Add JVM attachment sharing, renewal, reacquisition and close/shutdown release.

## 3. Deployment and verification

- [x] 3.1 Update Helm permissions, API schema, examples and migration documentation.
- [x] 3.2 Verify Go/JVM builds, PostgreSQL concurrency/restart/lifetime tests and Helm rendering.
- [x] 3.3 Verify actual Spark Operator driver/executor resources, replacement and rolling-version drain in an isolated cluster; build the server image and record evidence.
