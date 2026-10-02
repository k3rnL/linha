## MODIFIED Requirements

### Requirement: Owner-scoped observation
The server SHALL derive ordinary SDK ownership from authenticated identity and SHALL provide paginated listing, specific-job state, and result access only within that identity's authorization. Separately authorized administrative endpoints SHALL permit configured administrators to inspect requests across owners and operators to create/replay/cancel them, retaining target ownership and durable actor attribution. Administrator status SHALL NOT broaden ordinary SDK endpoints. Public IDs SHALL remain independent of native engine IDs and execution attempts.

#### Scenario: Cross-owner query or cancellation
- **WHEN** an ordinary caller tries to read, list, cancel, or retrieve results for another owner's job
- **THEN** the operation discloses no unauthorized job information and makes no mutation

#### Scenario: A request receives another attempt
- **WHEN** an interrupted request is retried under its policy
- **THEN** its public ID stays unchanged and its attempt history reflects both executions

#### Scenario: Explicit administrative access
- **WHEN** a configured administrator uses the dedicated admin endpoints within its viewer/operator permissions
- **THEN** the authorized cross-owner operation succeeds without changing job ownership or granting equivalent access to ordinary clients
