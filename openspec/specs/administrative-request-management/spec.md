# administrative-request-management Specification

## Purpose

Let authorized operators deliberately create, edit and replay, and cancel durable requests across existing contexts while retaining ownership, auditability, and accepted-work lifetime.

## Requirements

### Requirement: Shared creation and edited replay
An operator SHALL create a request for an existing context/version using a handler identifier/version, JSON payload, bounded retry settings, and optional future deadline. Replay SHALL open the same creation page prefilled from an existing job and SHALL allow editing before submission. Replay SHALL default to the exact original context version; selecting another version SHALL be explicit and restricted to the source owner's logical context. Fresh creation SHALL derive ownership from the selected context.

#### Scenario: Edit and replay a failed request
- **WHEN** an operator opens Replay, edits the JSON parameters, and submits
- **THEN** Linha creates a new request linked to the source with the edited input while the source input, status, and results remain unchanged

#### Scenario: Select a newer version
- **WHEN** an operator changes the replay target to a newer configuration version
- **THEN** the selected version and its result policy are shown explicitly and the accepted request records that version and policy

#### Scenario: Invalid payload or another owner's replay target
- **WHEN** an edited request has invalid JSON, invalid retry/deadline settings, or a replay target under another owner/logical name
- **THEN** acceptance fails before scheduling work and leaves the original request unchanged

### Requirement: Durable administrative acceptance
Administrative submission SHALL commit its public ID, context-derived owner, immutable input, policies, fresh submission time, optional replay source, actor, and accepted audit record before acknowledgement. It SHALL preserve owner/logical-name idempotency across versions. Each deliberate new submission SHALL use a fresh key, while retries of the same submission SHALL reuse its key. Old deadlines and idempotency keys SHALL NOT be copied automatically from the replay source.

#### Scenario: Lost response and concurrent retry
- **WHEN** the acceptance response is lost and the same submission reaches two replicas with the same key
- **THEN** successful responses identify one new durable job and one accepted administrative action

#### Scenario: Edits reuse an accepted key
- **WHEN** an operator changes input, provenance, or result policy while retrying an already used key
- **THEN** the server reports conflict and does not mutate or create another accepted request

#### Scenario: Accepting replica dies
- **WHEN** the accepting replica stops immediately after acknowledgement
- **THEN** another replica retrieves and schedules the same job with its actor and replay provenance intact

### Requirement: Browser-independent accepted-work lifetime
Administrative acceptance SHALL be a separately authorized alternative to SDK client-lease submission. Accepting work and requesting backend reconciliation SHALL be atomic with respect to context retirement. A stopped or draining version SHALL require explicit activation for new administrative work. Accepted jobs, configured retries, and required replacement capacity SHALL continue after browser closure or logout; once work is terminal and no SDK clients remain, the version SHALL drain and stop regardless of minimum drivers.

#### Scenario: Submit to a stopped context
- **WHEN** an operator submits against a stopped version without explicit activation
- **THEN** the server returns an actionable conflict and creates no job or capacity

#### Scenario: Activate and close browser
- **WHEN** an operator explicitly activates a stopped version and submits, then closes the browser while all servers restart
- **THEN** the durable accepted work causes the backend to recover and run, and the version stops after work completes if no clients need it

#### Scenario: SDK lease expiry races with admin submission
- **WHEN** the last SDK lease expires concurrently with authorized administrative acceptance
- **THEN** the context lock and committed job demand prevent loss of acknowledged work without creating a permanent administrative lease

### Requirement: Request-scoped cancellation
Operators SHALL cancel queued or running requests using the existing authoritative cancellation ordering. Running work SHALL show CANCELLING until termination or lease recovery resolves it. Cancellation SHALL NOT stop the shared engine or unrelated requests, and completed results SHALL remain stable when completion won the race.

#### Scenario: Cancel one of two running Spark requests
- **WHEN** two requests share a driver and an operator cancels one
- **THEN** only that request's execution is cancelled and the other remains eligible to finish normally

#### Scenario: Cancellation wins and stale worker completes
- **WHEN** cancellation commits before completion and an old worker later reports success
- **THEN** the report cannot publish a result and the job resolves to CANCELLED according to lease/termination rules

#### Scenario: Completion or repeated cancellation wins
- **WHEN** success commits before cancellation or an already accepted cancellation is repeated
- **THEN** the terminal result is preserved or the existing cancellation state is returned without conflicting mutations

### Requirement: Persistent administrative provenance
Every accepted administrative create/replay/cancel operation SHALL retain its actor, target identity/owner, action, source request where relevant, time, and outcome in durable audit history. Mutation acceptance and its audit record SHALL commit together. Ordinary clients SHALL NOT gain cross-owner permissions through replay provenance or administrator-created records.

#### Scenario: Audit persistence fails
- **WHEN** the audit record cannot commit with an administrative mutation
- **THEN** the server does not acknowledge the mutation as accepted

#### Scenario: Original owner retrieves a replay
- **WHEN** an administrator replays a job and the original application later lists its owned jobs
- **THEN** the new request remains owned by that application, while another ordinary owner still cannot read or cancel it
