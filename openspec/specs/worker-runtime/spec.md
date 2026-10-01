# worker-runtime Specification

## Purpose

Allow user-supplied JVM images to dispatch raw JSON requests or typed entrypoints through a job context, with managed results, bounded capacity, and recoverable work ownership.

Authentication and ownership requirements below apply when the relevant security
switch is enabled. The explicit anonymous deployment behavior is specified in
ha-deployment; durable identity, lease fencing, and result rules still apply.

## Requirements


### Requirement: Raw dispatch and optional typed execution
The base worker runtime SHALL expose a versioned JSON request and a job context to a user-defined dispatcher. A Scala wrapper SHALL deserialize registered request parameters, reconstruct the installed entrypoint, invoke its execute method, and encode its result through the same base protocol. Both modes SHALL advertise supported handler/schema contracts; arbitrary class names or executable objects from submissions SHALL NOT select unregistered code.

#### Scenario: User dispatches JSON manually
- **WHEN** a compatible request reaches a raw worker callback
- **THEN** the user's dispatcher selects its business code and returns a result using the same lifecycle and publication rules as a typed entrypoint

#### Scenario: SDK dispatches a typed entrypoint
- **WHEN** a request matches a registered typed descriptor
- **THEN** the SDK reconstructs the entrypoint from JSON parameters and calls its execute method with the current job context

### Requirement: Job context and engine capabilities
Execution SHALL receive job/attempt identity, cooperative cancellation, progress reporting, managed result helpers, and the applicable engine capabilities through an SDK job context. Progress SHALL be associated with and accepted only from the current valid attempt. Progress alone SHALL NOT publish success. Spark context access SHALL use the request's session while keeping the shared engine lifecycle under SDK control.

#### Scenario: Job reports progress
- **WHEN** valid running code reports a progress update
- **THEN** the owner can observe the persisted attempt-associated update through the job status API

#### Scenario: Progress reaches completion before results finish
- **WHEN** user code reports full progress before result publication completes
- **THEN** the job remains non-successful until the authoritative completion protocol succeeds

#### Scenario: Callback fails while holding managed resources
- **WHEN** execution fails after opening an SDK-managed writer or registering request-owned resources
- **THEN** the runtime closes or aborts those resources, reports failure, and does not intentionally release another request's resources

### Requirement: Packaged worker entrypoint and readiness
The worker SDK SHALL supply a Linha JVM process entrypoint embeddable in the user's Docker image. The runtime SHALL load user handlers, initialize its engine environment, authenticate its assigned context/incarnation, and advertise supported entrypoint schemas and execution capacity before becoming ready.

#### Scenario: Worker starts successfully
- **WHEN** an image starts with valid registrations and engine configuration
- **THEN** the worker registers its capabilities and accepts work only after initialization completes

#### Scenario: Handler version is unavailable
- **WHEN** a ready backend cannot satisfy a queued request's entrypoint schema
- **THEN** the server records a compatibility failure rather than executing a different handler or leaving the request indefinitely queued

### Requirement: Capacity-aware work acquisition
A worker SHALL acquire work only for its assigned context and compatible handlers, within a configured concurrent-request limit. It SHALL stop acquiring work while draining or unable to maintain ownership.

#### Scenario: Worker is full
- **WHEN** all configured request slots are occupied
- **THEN** additional requests remain in the durable queue or are assigned to other eligible workers

#### Scenario: Worker is draining
- **WHEN** the server requests drain
- **THEN** the worker finishes or resolves its active attempts and accepts no new ones before shutdown

### Requirement: Lease and incarnation protocol
The runtime SHALL renew attempt leases, report instance heartbeats, and attach attempt/incarnation identifiers to all updates. A worker unable to renew before ownership expires SHALL attempt to stop its execution and SHALL NOT treat itself as authorized to commit a result.

#### Scenario: Network partition outlasts the lease
- **WHEN** a worker cannot renew ownership before expiry
- **THEN** it stops claiming work and attempts cancellation, while the server can recover the expired attempt under the job's policy

#### Scenario: Old process reconnects
- **WHEN** an obsolete worker incarnation reconnects after replacement
- **THEN** its claims, progress, and completion messages cannot mutate the replacement's jobs

### Requirement: Handler outcomes and durable result publication
Handlers SHALL return a raw JSON value, a typed value, or an attempt-bound file/dataset descriptor, or report a structured failure. The SDK SHALL encode and store successful outputs using the accepted result policy and expose managed file/stream writing and engine-specific dataset helpers. Handler exceptions SHALL become durable job errors. Success SHALL be reported only after selected result storage is ready for an authoritative completion commit.

#### Scenario: Handler throws
- **WHEN** user code raises an execution exception
- **THEN** the runtime reports a structured failure associated with the current attempt and releases its local slot

#### Scenario: Completion acknowledgement is lost
- **WHEN** the server commits success but the worker loses the response
- **THEN** repeating completion returns the committed outcome without publishing a second result

### Requirement: Scoped worker authorization
Workers SHALL authenticate with credentials restricted to their context/incarnation and authorized result destinations. A worker SHALL NOT gain general access to the job database or other owners' requests.

#### Scenario: Worker requests another context's job
- **WHEN** an authenticated worker attempts a claim or update outside its assignment
- **THEN** the server rejects the operation without exposing that context's data
