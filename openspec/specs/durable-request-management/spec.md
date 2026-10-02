# durable-request-management Specification

## Purpose

Preserve owned requests, stable identifiers, execution history, and authoritative outcomes independently of server and engine process lifetimes.

Authentication and ownership requirements below apply when the relevant security
switch is enabled. The explicit anonymous deployment behavior is specified in
ha-deployment; durable identity, lease fencing, and result rules still apply.

## Requirements

### Requirement: Engine-neutral JSON acceptance
The server SHALL accept versioned handler identifiers and JSON payloads without requiring JVM types or decoding business objects. Accepted records SHALL preserve immutable request data, the effective result-policy snapshot, and the server-recorded submission time used for output paths. Raw and typed SDK requests SHALL use the same durable lifecycle.

#### Scenario: Non-JVM client submits a job
- **WHEN** an authorized caller submits a valid JSON envelope for an available handler contract
- **THEN** the server queues it without requiring Scala serialization or a Spark-specific request model

#### Scenario: Submission response is lost across a date boundary
- **WHEN** a previously accepted submission is repeated under the same scoped idempotency key on the next day
- **THEN** its original public ID, submission timestamp, and result-policy snapshot remain unchanged

### Requirement: Durable acceptance and idempotency
The server SHALL commit a request, its authenticated owner, immutable input and policies, initial queued state, and public ID before acknowledging acceptance. An identical submission with the same owner/logical-name idempotency key across configuration versions SHALL return its original ID; conflicting content SHALL be rejected.

#### Scenario: Server fails immediately after acknowledgement
- **WHEN** the accepting server stops after returning a job ID
- **THEN** another replica or restarted server can retrieve and schedule that request from persistent state

#### Scenario: Database is unavailable
- **WHEN** persistence is unavailable during submission
- **THEN** the server rejects or fails the call without claiming durable acceptance

#### Scenario: Concurrent duplicate submissions
- **WHEN** two replicas receive identical submissions with the same scoped idempotency key
- **THEN** exactly one public request record is created and both successful responses identify it

#### Scenario: Idempotency key is reused with different input
- **WHEN** a submission changes parameters or execution/result policy under an existing scoped key
- **THEN** the server returns a conflict and leaves the existing request unchanged

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

### Requirement: Durable queue and bounded assignment
Accepted work SHALL remain queued until a compatible ready worker has capacity. Assignment SHALL create a durable, exclusive current attempt with expiring ownership. Work SHALL remain recoverable across server restarts.

#### Scenario: Backend has not started
- **WHEN** a request is accepted for a provisioning context
- **THEN** it remains queryable in QUEUED state until capacity becomes available, cancellation occurs, or its deadline expires

#### Scenario: Two workers compete for the same request
- **WHEN** workers claim work concurrently through different server replicas
- **THEN** only one current attempt owns the request and advertised capacity is not exceeded

### Requirement: Recovery and retry policy
The server SHALL persist bounded retry settings, attempt outcomes, deadlines, and backoff. Automatic retries SHALL require explicit opt-in for retry-safe handlers; the default maximum number of attempts SHALL be one. Expired ownership SHALL resolve to retry or durable failure according to policy, and SHALL NOT leave a job permanently RUNNING.

#### Scenario: Worker dies with retries disabled
- **WHEN** an attempt loses ownership after its worker disappears and no retries remain
- **THEN** the job becomes FAILED with an interruption reason and retains its original ID

#### Scenario: Worker dies with a retry available
- **WHEN** ownership expires for a retry-enabled request with attempts remaining
- **THEN** the server schedules a new attempt after backoff without changing the public ID

#### Scenario: Every server restarts during a queued retry
- **WHEN** server processes restart while the database retains a retrying job
- **THEN** the remaining retry/deadline policy is reconstructed from persisted state

### Requirement: Authoritative terminal outcomes
Progress and outcome updates SHALL be accepted only from the current valid attempt. Completion and cancellation SHALL have a deterministic ordering. Terminal outcomes SHALL be stable under duplicate messages and late attempts.

#### Scenario: Expired worker reports success
- **WHEN** an old worker reports completion after its lease was lost or a replacement attempt was assigned
- **THEN** the server rejects the update and does not replace the current job outcome or result

#### Scenario: Cancellation wins the race
- **WHEN** cancellation intent commits before a running attempt's completion
- **THEN** completion cannot publish success and the job resolves to CANCELLED after execution termination or ownership expiry

#### Scenario: Completion wins the race
- **WHEN** success commits before a cancellation request
- **THEN** the completed result and SUCCEEDED state remain available
