## Purpose
Allow stable logical context names across rolling client deployments while keeping durable job identities and reclaiming unused backend versions after accepted work finishes.

## ADDED Requirements
### Requirement: Configuration versions
The server SHALL derive an immutable version from normalized effective configuration and the resolved worker image, scoped by owner and logical name.
#### Scenario: Concurrent ensure
- **WHEN** two replicas ensure identical effective settings under the same owner and name
- **THEN** they SHALL receive the same context ID and version with independent client leases
#### Scenario: Rolling change
- **WHEN** a replica ensures changed settings using the same logical name
- **THEN** a new version SHALL coexist with the old version until the old version has no live clients and no accepted work
### Requirement: Durable client lifetime
Clients SHALL renew persisted expiring leases; release and expiry SHALL retire a version after its accepted jobs finish.
#### Scenario: Client killed
- **WHEN** the last client disappears without releasing its lease
- **THEN** database-clock expiry SHALL trigger draining and eventual shutdown despite minimum driver settings
#### Scenario: Accepted work
- **WHEN** the last lease expires with queued or running work
- **THEN** the backend SHALL finish that work, including configured retries and replacement capacity, before stopping
#### Scenario: HA restart
- **WHEN** all servers restart
- **THEN** stored leases, versions, jobs and results SHALL remain authoritative
#### Scenario: Stale or foreign lease
- **WHEN** an expired lease or another owner's lease is used to submit new work or renew
- **THEN** the operation SHALL be rejected without affecting accepted work
#### Scenario: Reactivation
- **WHEN** a client attaches to a stopped version
- **THEN** the server SHALL create a new lease and recreate required capacity without changing historical IDs
### Requirement: Logical idempotency
Job idempotency SHALL be scoped by owner, logical name and key across configuration versions.
#### Scenario: Retry during rollout
- **WHEN** the same request and result policy are retried against a new version using the same key
- **THEN** the original durable job ID SHALL be returned
#### Scenario: Retained result
- **WHEN** a context has stopped
- **THEN** its owner SHALL still retrieve retained job status and result metadata and bytes
