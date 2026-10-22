## Why

Workers in another namespace need application object storage without mounting client-owned configuration, credentials or volumes. Per-item requests must also support a durable completion status when their business output is written directly to an application dataset.

## What Changes

- Add explicitly enabled, named application-storage destinations to a backend specification, independent of managed job results.
- Add owner-bound client sessions and attempt-fenced worker sessions for storage access, persisted in PostgreSQL for HA.
- Provide bounded S3 file operations through Linha and generic renewable resource locks for shared metadata updates.
- Add JVM storage helpers and a Hadoop filesystem integration for Spark driver/executor access without application Kubernetes resources.
- Add status-only entrypoint results: successful completion persists state without allocating a result file.
- Keep provider credentials on the server; never include them in persisted request payloads or Hadoop configuration distributed to workers.

## Capabilities

### New Capabilities

- `workload-storage-access`: Explicit application-storage access, scoped sessions, fenced IO, resource locking and Hadoop integration.
- `status-only-results`: A result descriptor for completion without result artifacts.

### Modified Capabilities

None. Application files remain independent of retained result allocations.

## Impact

Go domain/API/storage/PostgreSQL, JVM protocol/client/worker/Spark SDKs, security tests and operator/developer documentation. The server becomes the data gateway for application file IO; bandwidth and staging capacity must be sized for workloads. S3 and PostgreSQL remain external services. Business side effects are not exactly-once.
