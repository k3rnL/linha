## Why

Configured Spark CPU cores currently produce scheduling requests without CPU
limits, both for directly created Linha drivers and Spark-created executors.
This allows generated containers to exceed the client's configured CPU budget.

## What Changes

- Set driver CPU requests and limits to `drivers.cores`.
- Set executor Kubernetes CPU requests and limits to `executors.cores`, preserving
  Spark task parallelism and memory settings in static and dynamic allocation.
- Add regression coverage and verify actual driver/executor resources and replacement.
- Document resource mapping and when existing running Pods pick up the correction.

## Capabilities

### New Capabilities
- `spark-resource-enforcement`: Map engine CPU settings to explicit Pod requests
  and limits while retaining heap/overhead accounting for memory.

### Modified Capabilities
None. Main capability specs have not yet been archived from the platform change.

## Impact

Only the Spark engine adapter, tests and resource documentation change. No public
JSON fields, database schema, request ownership, acceptance, retries, result
publication, client SDK or worker SDK changes are required. The corrected limits
apply when drivers are provisioned; existing running Pods are not mutated.
