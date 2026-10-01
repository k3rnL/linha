## Why

Linha manually constructs driver Pods and applies separate memory calculations,
while a stable backend name currently conflicts whenever its configuration changes.
API rolling updates need shared configuration versions whose lifetime follows live
clients, while accepted jobs and retained results remain durable.

## What Changes

- Provision long-lived Spark workers through Kubeflow SparkApplication resources in
  cluster mode, with explicit application class/JAR and CRD-style resources.
- Expose memory, memoryOverhead, cores, coreRequest and coreLimit; preserve native
  Pod customization and use Spark submission to configure the driver JVM and Pod.
- Infer immutable backend versions from normalized effective configuration and the
  resolved worker image digest. One owner/name can have multiple versions.
- Add persisted, renewable client leases and SDK lifecycle management. Versions
  coexist during rolling updates; after the final lease is released or expires,
  finish accepted work and stop the backend, regardless of minimum driver count.
- Preserve public IDs/results, owner isolation, worker fencing and logical-name
  idempotency across configuration versions. Reconnecting clients can reactivate
  a stopped version without losing its history.
- **BREAKING:** new managed Spark definitions must declare their application launch
  settings; existing clients must adopt lease renewal. Legacy stored Pod contexts
  remain reconcilable for draining/migration.

## Capabilities

### New Capabilities
- `spark-application-provisioning`: Operator submission, CRD resource semantics,
  ownership-fenced observation/deletion and native Pod customization.
- `leased-context-versions`: Stable names, configuration-derived versions, client
  attachment/renewal/release, draining and durable rolling-update behavior.

### Modified Capabilities
None; the platform's capability specs remain in unarchived changes.

## Impact

Spark adapter, Kubernetes permissions, schema migration, context APIs, controller,
Scala client lifecycle, examples and deployment docs. Kubeflow Spark Operator is
an externally installed dependency; Linha's Role remains namespace-scoped.
Worker images need a Spark-compatible entrypoint plus their application JAR.
No automatic installation into production and no deletion of result data.
