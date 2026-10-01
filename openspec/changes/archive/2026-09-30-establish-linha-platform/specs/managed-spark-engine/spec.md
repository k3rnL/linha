## Purpose

Manage reusable Spark drivers and executor resources while supporting concurrent requests, bounded scaling, and replacement after driver loss.

## ADDED Requirements

### Requirement: User-configured Spark backend
Ensure SHALL support a user-supplied compatible image and validated Spark settings, including minimum/maximum driver counts, concurrent requests per driver, scaling thresholds, and static or dynamic executor allocation. Invalid resource bounds SHALL be rejected.

#### Scenario: Invalid driver range
- **WHEN** minimum drivers exceeds maximum drivers or maximum drivers is below one
- **THEN** ensure fails validation before creating resources

#### Scenario: Driver minimum is zero
- **WHEN** a valid context with zero minimum drivers receives queued work
- **THEN** the controller can provision capacity up to its allowed maximum and eventually dispatch when ready

### Requirement: Concurrent requests within a driver
A Spark driver SHALL support a configurable number of concurrent requests using one SparkContext, bounded execution slots, request-associated job groups, and a documented fair-sharing policy. Session settings and temporary views SHALL be separated per request; shared caches SHALL NOT be represented as full process isolation.

#### Scenario: Two requests have available capacity
- **WHEN** two compatible requests are assigned to a driver configured with two slots and sufficient executor resources
- **THEN** their Spark jobs can progress concurrently without creating another driver solely for concurrency

#### Scenario: One request is cancelled
- **WHEN** cancellation targets one running request
- **THEN** the worker targets that request's Spark job group and does not intentionally cancel unrelated requests

### Requirement: Driver scaling follows queue and capacity
The controller SHALL maintain requested minimum capacity and adjust desired driver count within configured maximum and resource limits using queue demand, wait age, occupied capacity, and pending startups. Starting instances SHALL be counted to avoid repeated overscaling. Excess workers SHALL drain before planned removal.

#### Scenario: Queue remains above the configured trigger
- **WHEN** workers are saturated, the queue trigger is reached, and limits permit additional capacity
- **THEN** the controller increases desired driver capacity with configured cooldown and accounts for already starting drivers

#### Scenario: Queue becomes empty
- **WHEN** excess drivers remain idle beyond the configured scale-down period
- **THEN** the controller drains/removes excess capacity while preserving the configured minimum

#### Scenario: Cluster is full
- **WHEN** desired drivers cannot be scheduled within available cluster resources
- **THEN** the context exposes constrained capacity and does not create an unbounded number of additional pending drivers

### Requirement: Lost drivers are replaced automatically
The controller SHALL reconcile disappeared or failed drivers to persisted desired capacity without another client request. Requests on the lost driver SHALL follow their own interruption/retry policies while keeping public IDs.

#### Scenario: Minimum-capacity driver is deleted
- **WHEN** a required driver disappears while its client is offline
- **THEN** replacement capacity is started automatically and becomes eligible only after worker readiness

#### Scenario: Lost driver carried a non-retryable request
- **WHEN** a driver dies while executing a request without retry permission
- **THEN** the request becomes durably failed and the driver is still replaced to maintain backend capacity

### Requirement: Executor allocation is independently controlled
The Spark backend SHALL support fixed executor counts or optional Spark dynamic allocation within caller-configured minimum/initial/maximum bounds and deployment quotas. Driver scaling SHALL NOT be confused with executor scaling.

#### Scenario: Dynamic allocation is enabled
- **WHEN** Spark has pending tasks or idle executors under a valid dynamic allocation policy
- **THEN** Spark can grow or shrink its executor population within the configured limits using an appropriate shuffle-preservation mechanism

### Requirement: Request resource cleanup
The Spark worker SHALL release request-owned caches, broadcasts, and execution-local properties after completion or termination without stopping the shared SparkContext or clearing resources owned by other requests.

#### Scenario: Repeated requests reuse one driver
- **WHEN** a series of requests completes on a long-running driver
- **THEN** completed requests do not retain their owned caches or thread-local scheduling/cancellation identity into later requests

### Requirement: Managed distributed result writing
The Spark integration SHALL provide job-context helpers that write DataFrames or Datasets to the configured local or S3 provider using distributed execution and assigned attempt-specific locations. It SHALL record the committed dataset format and file manifest without collecting all result bytes on the driver. Required shared mounts or compatible storage connectors, credentials, and committers SHALL be validated. Helpers SHALL NOT mutate global connector credentials between concurrent requests.

#### Scenario: Partitioned dataset is saved
- **WHEN** a handler invokes a managed Parquet write with partition columns
- **THEN** executors write under the allocated dataset prefix and successful publication references all committed output parts

#### Scenario: Executors cannot access local storage
- **WHEN** a local result configuration lacks the shared filesystem access needed for distributed output
- **THEN** the configuration or write reports an explicit capability/access failure rather than publishing a driver-only or incomplete result

#### Scenario: Distributed write fails after some tasks finish
- **WHEN** some output tasks succeed but the overall Spark write fails
- **THEN** the job cannot publish the partial dataset as a completed result and abandoned output follows cleanup policy
