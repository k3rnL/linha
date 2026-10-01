# typed-client-sdk Specification

## Purpose

Provide raw JSON job interaction and optional typed Scala/JVM wrappers with durable handles, backend contexts, and storage-independent result access.

Authentication and ownership requirements below apply when the relevant security
switch is enabled. The explicit anonymous deployment behavior is specified in
ha-deployment; durable identity, lease fencing, and result rules still apply.

## Requirements


### Requirement: Base JSON client operations
The public API and client SDK SHALL support submission of a versioned handler identifier and JSON payload without a Scala entrypoint class. Ensure, submission, listing, status, cancellation, and result retrieval SHALL be usable through the base protocol. Typed operations SHALL wrap these same operations and preserve their ownership, idempotency, and outcome semantics.

#### Scenario: Submit without a typed SDK
- **WHEN** an authenticated caller sends a valid handler identifier, version, JSON payload, and idempotency key through the base protocol
- **THEN** the server durably accepts it and returns a public job ID without requiring JVM type information

#### Scenario: Raw and typed submissions are equivalent
- **WHEN** raw and typed clients submit the same handler, schema version, JSON data, effective policy, and scoped idempotency key
- **THEN** they identify the same job and observe the same persisted state and result

### Requirement: Typed executable entrypoint wrapper
The Scala SDK SHALL allow user-defined parameter classes with an associated result type and an execute method receiving a job context. A registered descriptor SHALL identify the handler, request/result schemas, result kind, and codecs. Submission SHALL encode request data only; executable classes SHALL already be installed in the worker image. Derived and explicitly provided codecs SHALL be supported.

#### Scenario: Submit and reconstruct an entrypoint
- **WHEN** a caller submits a registered entrypoint instance
- **THEN** the wrapper encodes its parameters through the JSON API and the worker reconstructs the registered type and invokes its execute method

#### Scenario: Incompatible result schema
- **WHEN** a caller retrieves a result whose schema is incompatible with its descriptor
- **THEN** the SDK returns a compatibility error without silently decoding it as another type

### Requirement: Ensure and restore a backend context
The SDK SHALL let callers ensure a backend using a logical name, image, engine type/version, engine settings, and result/retry/retention policies. Result configuration SHALL support a local root or an S3 destination with a serializable path template. The returned context SHALL be restorable by identifier, with readiness observable separately from successful ensure.

#### Scenario: Backend is starting
- **WHEN** ensure succeeds while the backend has no ready workers
- **THEN** the caller receives a context reference, can inspect provisioning state, and can submit requests to its durable queue

#### Scenario: API process restarts
- **WHEN** an API process restores an owned context or ensures the same backend specification after restarting
- **THEN** it accesses the existing context and its jobs without creating an additional backend

#### Scenario: S3 path variables are configured before a job exists
- **WHEN** a caller ensures a context with a supported request/time/file path template
- **THEN** the SDK transmits the template as versioned configuration data without evaluating future job IDs, submission timestamps, or a remotely executable closure

### Requirement: Retry-safe client transport
The SDK SHALL reuse an idempotency key when retrying a submission and SHALL allow the caller to supply a key stable across client restarts.

#### Scenario: Submission response is lost
- **WHEN** the server committed a request but the client retries after losing the response
- **THEN** the client receives the original job ID for the same idempotency key and payload

### Requirement: Storage-independent result types
Typed result contracts SHALL distinguish encoded values, files, and datasets independently of local or S3 placement. A storage-provider change in a newly ensured context SHALL NOT require changing the entrypoint's logical result type or client decoding contract. File and dataset retrieval SHALL support streaming rather than requiring all result bytes in client memory.

#### Scenario: Same job type uses different providers
- **WHEN** the same entrypoint executes in separate contexts configured for local and S3 results
- **THEN** both handles expose the same result type and provider-appropriate authorized retrieval

#### Scenario: Dataset result contains multiple parts
- **WHEN** a caller retrieves a completed dataset result
- **THEN** the SDK exposes its format and committed file manifest with incremental listing/download rather than a single assumed file

### Requirement: Owned job and result access
The SDK SHALL expose owner-scoped paginated listing, status, cancellation, typed value retrieval, and artifact retrieval. A handle SHALL be reconstructible from a public ID and expected result descriptor after restart. Reading a pending result SHALL report not-ready without implying synchronous job execution.

#### Scenario: Retrieve a completed job after all processes restart
- **WHEN** a caller recreates a handle after process recovery while configured storage retains the result and before expiry
- **THEN** it can retrieve persisted status and result without reconnecting to the original worker

#### Scenario: Caller requests another user's job
- **WHEN** a caller supplies a job ID owned by another identity
- **THEN** the SDK exposes access denial without returning that job's status or result
