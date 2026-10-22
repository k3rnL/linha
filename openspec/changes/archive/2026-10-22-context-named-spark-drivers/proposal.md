## Why

Spark driver names contain only internal IDs, making it difficult to identify an application's drivers in Kubernetes. Include the logical context name without compromising instance uniqueness or worker authentication.

## What Changes

- Name newly submitted Spark driver Pods after their logical context, followed by the full unique instance ID and `-driver`.
- Normalize context names and bound the readable prefix to produce valid Kubernetes DNS labels.
- Preserve application ownership and Pod UID checks when authenticating named drivers, and continue observing existing drivers through their recorded application status.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `spark-application-provisioning`: Context-readable driver names with unique instance suffixes and preserved lifecycle and authentication fencing.

## Impact

SparkApplication driver submission, Kubernetes naming helpers, worker authentication, focused tests and Spark operations documentation. No public API, SDK, database schema or context-version changes.
