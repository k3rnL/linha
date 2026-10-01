# durable-result-storage Specification

## Purpose

Persist values, files, and datasets in configured local or S3 storage with deterministic output paths, controlled publication, and authorized retrieval after process restarts.

## Requirements


### Requirement: Explicit local or S3 placement
A context SHALL select LOCAL or S3 result storage during ensure. Each accepted request SHALL snapshot its provider, destination, path policy, limits, and retention. Result payloads SHALL use that provider; PostgreSQL SHALL retain job state and result metadata rather than act as a result-payload provider. Unauthorized destinations or unsupported policies SHALL be rejected, and limit failures SHALL NOT silently change providers.

#### Scenario: Local JSON result
- **WHEN** a job returns a JSON value under a local result policy
- **THEN** its encoded bytes are persisted beneath the configured root and its authoritative record references that location

#### Scenario: S3 result
- **WHEN** a job returns a result under an S3 policy
- **THEN** its bytes are stored in the assigned destination and its durable record retains verified immutable references

#### Scenario: Result exceeds a configured limit
- **WHEN** a result or required staging allocation exceeds its configured limit
- **THEN** execution reports an explicit limit failure without switching providers or publishing partial success

### Requirement: Local storage lifecycle and access
Local storage SHALL use a caller-managed filesystem root and SHALL validate containment and required access. Documentation and deployment settings SHALL identify persistent shared storage and mount requirements for serving replicas, replacement workers, and any executors performing distributed writes. The SDK SHALL NOT claim a worker-only path survives worker loss or silently substitute ephemeral storage.

#### Scenario: Worker is replaced with the shared volume retained
- **WHEN** a completed result's worker disappears and the same persistent filesystem remains available to serving replicas
- **THEN** the owner can retrieve the original result without that worker

#### Scenario: Required root is absent
- **WHEN** a serving process or writer cannot access the configured local root
- **THEN** the affected component exposes an access/readiness failure and does not write or serve from unrelated node-local storage

#### Scenario: File path escapes the result root
- **WHEN** an output or read would escape the assigned namespace through traversal or a symlink
- **THEN** the operation is rejected without accessing the escaped destination

### Requirement: Serializable S3 path configuration
S3 storage SHALL support versioned relative path templates with placeholders for context ID, public request ID, attempt ID, persisted submission time, and artifact-relative file path. The bucket, endpoint, and allowed prefix SHALL come from an authorized destination. Templates SHALL be persisted as data and SHALL NOT require executing user-supplied code in the server or a future client process.

#### Scenario: Date-partitioned custom path
- **WHEN** a destination rooted at s3://bucket/blabla/ uses exports/{submittedAt:yyyy/MM/dd}/{requestId}/{attemptId}/{file} for job-123 accepted on 2026-09-28 UTC, attempt-1, and result.json
- **THEN** the assigned result URI is s3://bucket/blabla/exports/2026/09/28/job-123/attempt-1/result.json

#### Scenario: Unknown variable or invalid format
- **WHEN** ensure includes an unknown placeholder, malformed template, unsupported date format, or an executable path callback
- **THEN** ensure returns a configuration error before creating a context or backend resources

#### Scenario: Destination escape in a template
- **WHEN** a template contains an absolute URI/path, traversal, or otherwise escapes the authorized bucket/prefix
- **THEN** ensure rejects the template rather than granting write access elsewhere

### Requirement: Deterministic template resolution
Template time SHALL mean the immutable server-recorded acceptance instant in UTC. Request ID SHALL mean the public job ID. Supported date formatting SHALL produce identical results across Go and JVM implementations. Resolved output allocations SHALL be persisted before writes are authorized. Retransmission SHALL reuse the allocation; retrieval SHALL use stored references without reevaluating templates.

#### Scenario: Client or worker restarts after midnight
- **WHEN** an output allocation or completion is retried for the same attempt after a day boundary and process restart
- **THEN** its resolved path retains the original submission date and identity

#### Scenario: Job retries on a later day
- **WHEN** the server schedules another execution attempt for an accepted request
- **THEN** the path retains the same public request ID and submission time but uses the new attempt ID

#### Scenario: Client timezone differs from server timezone
- **WHEN** clients or workers run with different system timezone settings
- **THEN** they agree with the authoritative UTC path expansion

### Requirement: Collision-free attempt and artifact namespaces
Every S3 template SHALL include public request ID and attempt ID exactly once as complete path segments and the artifact path exactly once as the final suffix. Local outputs SHALL also have distinct job/attempt namespaces. Artifact-relative names SHALL be validated and conflicting or overlapping allocations SHALL fail before writing. Validating the resolved location SHALL preserve authorized-prefix containment and provider length limits.

#### Scenario: Template omits attempt identity
- **WHEN** ensure supplies a template without a complete request or attempt segment
- **THEN** ensure rejects it instead of allowing retries to overwrite another execution's outputs

#### Scenario: Late writer overlaps a retry
- **WHEN** an expired attempt keeps writing while a newer attempt executes
- **THEN** the attempts write to distinct namespaces and only the current valid attempt can publish the authoritative result

#### Scenario: Duplicate logical filename
- **WHEN** an attempt allocates an already used logical name with conflicting kind or settings
- **THEN** allocation fails rather than overwriting the first output

#### Scenario: Invalid artifact name
- **WHEN** a runtime name contains forbidden segments, encoded traversal, reserved internal names, or causes an overlong destination
- **THEN** allocation fails before authorizing that write

### Requirement: Managed result production
The worker SDK SHALL support automatic JSON value encoding, managed file-path callbacks, output-stream callbacks, and engine-specific dataset writers. Provider selection SHALL NOT change logical result types. Helpers SHALL manage their resources and return attempt-bound descriptors; writing an output SHALL NOT itself publish job success.

#### Scenario: Seekable writer with S3 storage
- **WHEN** user code writes a file through a managed local-path callback under an S3 policy
- **THEN** the SDK provides staging, finishes and uploads the file, and tracks cleanup without requiring upload code in the handler

#### Scenario: Stream callback fails
- **WHEN** a user stream callback throws after writing some bytes
- **THEN** the SDK closes or aborts its resources and no partial artifact is exposed as a successful result

#### Scenario: Distributed dataset is produced
- **WHEN** an engine helper successfully writes multiple dataset parts
- **THEN** the result describes the committed dataset and its files without collecting the dataset bytes into the driver or API server

### Requirement: Atomic public result visibility
Only the current valid attempt SHALL publish a result. Public success SHALL imply a verified result descriptor or manifest is durably recorded after all required writes finish. Staged or abandoned outputs SHALL NOT appear as completed results through the Linha API. Repeated identical completion SHALL return its committed receipt; conflicting completion SHALL fail.

#### Scenario: Worker dies during upload
- **WHEN** a worker writes only part of a result or dies before authoritative completion
- **THEN** the job does not become SUCCEEDED and the partial output is not exposed as its result

#### Scenario: Stale attempt finishes uploading
- **WHEN** an expired attempt uploads objects and asks to finalize
- **THEN** those objects cannot replace the current attempt's committed result

#### Scenario: Dataset has an incomplete write
- **WHEN** some distributed output tasks have finished but the overall write fails
- **THEN** the partial part files are not published as a completed dataset

#### Scenario: Completion acknowledgement is lost
- **WHEN** the server committed success but the worker retries after losing the response
- **THEN** the server returns the original receipt and retains the original manifest and locations

### Requirement: Retrieval independent of engine lifecycle
Authorized result retrieval SHALL use persisted job metadata and configured storage, without requiring the original driver, worker, or engine session. Unavailable storage SHALL produce an explicit retrieval error without deleting recorded success.

#### Scenario: All processes restart after completion
- **WHEN** server and engine processes restart while the database and configured local or S3 storage retain their data
- **THEN** the owner can retrieve the completed result by its original public ID before retention expiry

#### Scenario: Storage is temporarily unavailable
- **WHEN** a committed local or S3 result cannot be read due to a storage outage
- **THEN** the API reports a retrieval error without changing the job to unknown or discarding recorded success

### Requirement: Authorized values and artifact access
Result metadata and payload access SHALL enforce ownership. The API SHALL support JSON values, streaming file access, and incremental dataset-manifest/part retrieval. Temporary download credentials SHALL be issued only after authorization; permanent result identity SHALL NOT depend on an expiring URL.

#### Scenario: An old download URL expires
- **WHEN** an owner returns after a signed URL expires but the result is retained
- **THEN** the owner can obtain fresh authorized access through the original job ID

### Requirement: Retention and abandoned output cleanup
Payload expiry SHALL follow the recorded retention policy and preserve job metadata, completion receipts and idempotency records indefinitely in version 1. An expired result SHALL report RESULT_EXPIRED. Cleanup SHALL distinguish abandoned output from active allocations and retained published results.

#### Scenario: Result payload expires
- **WHEN** an owner requests a result after payload expiry
- **THEN** the API reports the original job state and explicit result expiry rather than an unknown job

#### Scenario: Cleanup observes an active writer
- **WHEN** an allocated output is still owned by a valid active attempt
- **THEN** orphan cleanup does not delete its data solely because completion has not yet been published

#### Scenario: Expired delegated writer
- **WHEN** a terminal attempt still has an unexpired delegated write grant
- **THEN** physical cleanup SHALL wait for the recorded grant expiry and cleanup grace before deleting the allocation

#### Scenario: Abandoned multipart upload
- **WHEN** opt-in cleanup finds an eligible dataset prefix with incomplete S3 multipart writes
- **THEN** it SHALL abort those writes and delete only objects beneath that allocation, preserving unrelated keys

### Requirement: Bounded immutable dataset publication
Distributed dataset output SHALL use attempt-scoped local or STS-authorized S3 staging. The server SHALL snapshot parts with conditional immutable publication, index at most 100000 parts, and expose paginated authorized retrieval only after a matching sealed manifest completes. Dataset payload bytes SHALL NOT be stored in PostgreSQL. API temporary copies SHALL reserve the configured staging budget.

#### Scenario: Conflicting part replay
- **WHEN** a worker re-registers an already published part with changed bytes
- **THEN** publication SHALL fail without replacing the original immutable bytes

#### Scenario: Scoped S3 authorization
- **WHEN** a dataset worker uses its delegated credentials outside its staging prefix or against published parts
- **THEN** the storage service SHALL reject the operation

#### Scenario: Cross-owner parts
- **WHEN** another owner lists or downloads a retained dataset part
- **THEN** the API SHALL reject access before exposing any result metadata or bytes

#### Scenario: Concurrent jobs and restart
- **WHEN** concurrent jobs publish datasets and all workers and API processes subsequently restart
- **THEN** their owners SHALL retrieve distinct retained manifests and parts by the original public job IDs
