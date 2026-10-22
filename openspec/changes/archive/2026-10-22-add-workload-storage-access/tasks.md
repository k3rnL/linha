## 1. Application storage authorization

- [x] 1.1 Add explicit destination access policy, backend fingerprints and persisted client/worker sessions.
- [x] 1.2 Implement session authorization, path validation and stale-attempt rejection across replicas.
- [x] 1.3 Implement bounded S3 read/list/write/move/delete operations and resource locks.

## 2. JVM access and status results

- [x] 2.1 Add raw protocol/client/worker session helpers and unit/status result support.
- [x] 2.2 Add a Hadoop filesystem binding with seekable range reads and session-isolated credentials.
- [x] 2.3 Verify status-only publication, concurrent session isolation and storage locking.

## 3. Validation and documentation

- [x] 3.1 Validate PostgreSQL session/lock recovery, cross-owner rejection and stale-worker failure.
- [x] 3.2 Validate S3-compatible IO through real client and worker/executor bindings.
- [x] 3.3 Document destination opt-in, server capacity and independent application-file lifecycle.
