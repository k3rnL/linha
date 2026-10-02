## ADDED Requirements

### Requirement: Informational client hostname
Ensure and attach requests SHALL accept optional bounded hostname metadata.
The JVM SDK SHALL automatically report its hostname on ensure, restore and
lease reacquisition. Administrators SHALL see the hostname above the stable
client ID; clients without metadata SHALL show a clear unavailable label.
Hostname SHALL NOT affect ownership, configuration hashes, lease identity,
authorization, idempotency or Prometheus label cardinality.

#### Scenario: Concurrent replicas with identical configuration
- **WHEN** clients on different hosts ensure the same owner/name/configuration
- **THEN** they share a context version with separate leases and hostnames

#### Scenario: Legacy client and persisted lease
- **WHEN** a request omits hostname or all servers restart
- **THEN** legacy requests remain accepted and any persisted metadata remains readable

#### Scenario: Expired or foreign lease
- **WHEN** a lease expires or another owner attempts to attach/renew
- **THEN** hostname metadata does not bypass ownership and lease fencing
