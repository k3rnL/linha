# workload-storage-access Specification

## Purpose

Provide application object storage to clients and leased workers without distributing provider credentials or requiring client-owned Kubernetes resources in worker namespaces.

## Requirements

### Requirement: Application storage requires explicit authorization
The server SHALL grant access only to administrator-enabled destinations declared in the backend specification, constrained to the configured bucket prefix. Managed result destinations SHALL not become application storage implicitly.

#### Scenario: Disabled destination
- **WHEN** a caller ensures a backend requesting application access to a destination without explicit enablement
- **THEN** the server rejects the request without issuing credentials or storage access

#### Scenario: Path escape
- **WHEN** an authorized session requests a traversal path or another destination
- **THEN** the server rejects the operation before contacting storage

### Requirement: Sessions preserve ownership and attempt fencing
Client access SHALL require a live owner-bound context attachment. Worker and executor access SHALL be delegated from a valid active execution attempt and SHALL stop when that attempt is expired, replaced or terminal. Provider secrets SHALL remain on the server.

#### Scenario: Cross-owner access
- **WHEN** another owner requests a client session for a context it does not own
- **THEN** the server denies access

#### Scenario: Replaced worker
- **WHEN** a worker uses its previous session after its execution attempt has been fenced
- **THEN** new reads and writes are rejected

#### Scenario: Server restart
- **WHEN** a server restarts while a valid execution and storage session remain live
- **THEN** another replica accepts that same session using persisted authorization state

### Requirement: File operations and resource locks are reusable
Authorized applications SHALL be able to read, list, write, move and delete objects within their destination. Shared metadata updates SHALL support renewable resource locks independent of the processing engine, and stale lock holders SHALL not start writes after losing the lock.

#### Scenario: Concurrent shared metadata updates
- **WHEN** two valid workers lock the same physical storage object
- **THEN** only one owns the lock and can perform protected writes until release or expiry

### Requirement: Hadoop access requires no application mounts
Spark drivers and executors SHALL receive a session-specific Hadoop filesystem configuration that grants access through Linha without provider access keys, client ConfigMaps, Secrets or shared PVCs.

#### Scenario: Remote execution
- **WHEN** a worker image executes a self-contained request in a different namespace
- **THEN** driver and executor file IO use the same authorized storage session without mounting client resources

### Requirement: Application files are independent of retained results
Application files SHALL not be deleted by managed result retention. Access policies and backend versioning SHALL identify physical storage location so reconfigured destinations cannot silently redirect existing sessions.

#### Scenario: Result expiry
- **WHEN** a status-only job expires or its result metadata is retained
- **THEN** its application dataset files remain under application control
