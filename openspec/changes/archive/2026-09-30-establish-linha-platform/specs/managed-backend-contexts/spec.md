## Purpose

Give clients stable managed execution contexts whose engine resources converge to persisted requirements with durable client leases and independently retained results.

## ADDED Requirements

### Requirement: Idempotent backend ensure
Ensure SHALL accept a caller-selected image and engine configuration, result-provider settings, and versioned path policy and SHALL create or return one owned context per logical key and canonical specification. Concurrent identical calls SHALL converge on one context, including while provisioning is in progress.

#### Scenario: Several API replicas start together
- **WHEN** multiple API replicas ensure the same owned context name and specification
- **THEN** they receive the same context ID and do not independently launch duplicate backend capacity

#### Scenario: New specification version
- **WHEN** a caller ensures an existing logical context key with a different image or configuration
- **THEN** ensure returns a new immutable version under the same logical name without changing accepted work on the old version

### Requirement: Validated immutable backend identity
A context SHALL record its engine version, resolved immutable image identity, validated engine settings, local/S3 result configuration, versioned path template where applicable, and policies. Result configuration SHALL use authorized destination and mount references, and its canonical representation SHALL participate in context identity. Invalid or unauthorized settings SHALL be rejected before resource creation. Specification changes SHALL derive a new version under the existing logical name.

#### Scenario: Image tag changes in its registry
- **WHEN** a previously ensured image tag points to different content after a worker restart
- **THEN** replacement workers use the context's recorded immutable image identity

#### Scenario: Unsupported engine setting
- **WHEN** ensure includes an unsupported engine or invalid engine-specific setting
- **THEN** the server returns a validation error and creates no backend resources

### Requirement: Independent reconciliation
The server SHALL persist desired backend capacity and reconcile it after client disconnects, driver loss, and server restarts. Transport connections SHALL NOT own backend lifetime; persisted client leases SHALL maintain demand. Once the last lease is released or expires, accepted jobs SHALL finish before the version stops.

#### Scenario: Driver disappears
- **WHEN** a managed driver disappears and remaining capacity is below the recorded desired count
- **THEN** the server provisions replacement capacity without another client ensure call and reports readiness separately from creation

#### Scenario: Client closes its handle
- **WHEN** a client process stops or closes its LinhaContext handle
- **THEN** its lease is released or expires, accepted work finishes according to persisted retry policies, and the unused version stops

#### Scenario: Provisioning cannot complete
- **WHEN** image pull, permissions, or cluster capacity prevents worker startup
- **THEN** the context exposes a provisioning condition, retries with bounded backoff, and does not claim to be ready

### Requirement: Engine-independent request semantics
Every registered engine SHALL expose validated settings, readiness/capacity, and lifecycle capabilities through a common contract. Switching the engine adapter SHALL NOT change public job identity, ownership, queueing, or result semantics.

#### Scenario: Non-Spark test backend
- **WHEN** the same request lifecycle is exercised with a fake engine implementation
- **THEN** acceptance, polling, cancellation, and result persistence work without Spark-specific request fields

### Requirement: Coordinated backend management
Concurrent controllers SHALL coordinate desired instance identities and reconcile ambiguous create/delete outcomes. A controller losing ownership SHALL NOT publish authoritative lifecycle decisions under its obsolete ownership epoch.

#### Scenario: Controller fails after resource creation
- **WHEN** a controller creates a driver but fails before recording its observation
- **THEN** the next controller discovers the same managed instance and converges without allocating a second logical instance for that slot

### Requirement: Result configuration survives backend replacement
Context ensure SHALL persist result-provider configuration independently of connected clients. Replacement workers SHALL receive the same accepted provider/template configuration and required authorized mounts or credentials. Changes to provider, destination, or path template SHALL produce a new immutable version under the logical name and SHALL NOT move existing results.

#### Scenario: Caller changes the S3 template
- **WHEN** a caller ensures an existing context name with a different result path template
- **THEN** ensure derives a new version and does not move or reinterpret existing results

#### Scenario: Worker restarts without the original client
- **WHEN** a managed worker is replaced while finishing accepted work after the last client exits
- **THEN** it receives the persisted storage settings and can use allocations resolved from durable job/attempt data
