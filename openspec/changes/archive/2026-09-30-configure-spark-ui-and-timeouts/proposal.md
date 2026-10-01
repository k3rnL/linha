## Why

Linha currently disables the Spark UI unconditionally and leaves idle executor retention at Spark defaults. Clients need to enable diagnostics and choose finite cache/shuffle retention for long-lived drivers without rebuilding worker code.

## What Changes

- Add optional per-context `ui.enabled` and `ui.port` Spark settings.
- Expose `executors.executorIdleTimeout`, `cachedExecutorIdleTimeout`, and `shuffleTrackingTimeout` as duration strings.
- Validate and normalize settings, applying the same configuration to SparkApplications and legacy driver launches.
- Preserve omitted defaults and existing normalized backend identities; document access and scale-down semantics.

## Capabilities

### New Capabilities

- `spark-ui-and-retention`: Configurable driver UI and executor retention in managed Spark contexts.

### Modified Capabilities

None.

## Impact

Go Spark adapter, public OpenAPI schema, configuration examples and documentation. Existing JVM clients already forward arbitrary engine-settings JSON and require no protocol change. No database migration, Helm exposure, production deployment or request lifecycle changes.
