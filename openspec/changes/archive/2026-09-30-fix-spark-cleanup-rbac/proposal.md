## Why

Spark cleanup issues collection DELETE requests for executor Pods and ConfigMaps.
The worker Role grants single-object delete only, producing Forbidden errors.

## What Changes

- Grant worker `deletecollection` on Pods and ConfigMaps in the release namespace.
- Keep other resource and server permissions unchanged.
- Add chart regression checks and verify allowed/denied API operations in an isolated cluster.

## Capabilities

### New Capabilities
- `spark-collection-cleanup`: Authorize Spark executor cleanup through namespaced collection deletion.

### Modified Capabilities
None; deployment specs remain in the unarchived platform change.

## Impact

Helm worker Role, deployment documentation and RBAC tests. No server, SDK, image,
result storage or database changes. The updated Role applies to existing worker
service accounts when the chart is upgraded.
