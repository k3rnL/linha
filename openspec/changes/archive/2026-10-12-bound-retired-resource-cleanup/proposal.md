## Why

Retired Spark instances are observed and stopped on every one-second reconciliation, even after their Kubernetes resources disappear. The production audit export records 4,053 missing-resource calls; retained context history makes this load grow indefinitely.

## What Changes

- Exclude stopped contexts and dead instances from frequent capacity reconciliation.
- Discover late orphan resources through a bounded periodic inventory of existing managed resources, including legacy driver Services.
- Require durable retired-intent identity, controller ownership and resource UID preconditions before deleting discovered resources.
- Preserve context reactivation, accepted-work draining and all durable request/result history.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `managed-spark-engine`: bound retired-resource discovery independently of historical instance count while retaining eventual orphan cleanup.

## Impact

Go controller and engine cleanup contracts, Kubernetes adapter, regression tests and operational documentation. No public API, SDK, database migration or additional Kubernetes permissions are required.
