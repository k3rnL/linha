## Why

The console needs an immediate operational summary, clearer separation of current
capacity from old instances, and recognizable client hosts. The Grafana dashboard
currently exposes too much internal detail for everyday query monitoring.

## What Changes

- Add an Overview landing page backed by complete database aggregates, recent failures and active contexts.
- Separate current/stopping instances from collapsed retired history, with compact driver/executor CPU and memory tables and explicit snapshot freshness.
- Collect optional client hostname metadata through ensure/attach and the JVM SDK; display it above client IDs.
- Replace the main Grafana layout with useful query activity, completion/failure, timing, capacity and resource panels; collapse diagnostics by default.

## Capabilities

### New Capabilities

- `admin-operations-overview`: Overview landing page and clear current/historical engine inspection.
- `operator-dashboard`: Query-focused Grafana layout with clear windows, units and capacity semantics.

### Modified Capabilities

- `admin-web-ui`: Make Overview the default landing page, including after login.
- `monitoring-deployment`: Replace the original exhaustive dashboard and per-series links with the curated view and console navigation.

- `leased-context-versions`: Add informational hostname metadata to client leases without changing identity or context versions.

## Impact

Go admin API and read queries, an additive schema 10 migration, optional JSON
hostname fields, automatic JVM client metadata, React views, Grafana generator,
Helm dashboard links, checks and documentation. The full metric export remains
available. Server upgrade precedes updated SDK rollout; old clients remain valid.
