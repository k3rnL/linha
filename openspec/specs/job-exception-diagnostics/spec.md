# job-exception-diagnostics Specification

## Purpose
Preserve attributable exception details so job owners can diagnose worker failures from durable job and attempt history after processes restart.

## Requirements

### Requirement: Durable exception diagnostics
Worker-reported failures SHALL include the exception type, stack trace with causes and suppressed exceptions, and execution phase when available, alongside code/message/retryable.
#### Scenario: Handler exception
- **WHEN** a handler throws a nested exception with a suppressed exception
- **THEN** the owner's job and attempt responses SHALL retain its type, frames, cause and suppressed information after server restart
#### Scenario: Initialization exception
- **WHEN** assignment initialization or engine creation fails before the business handler runs
- **THEN** the worker SHALL report the exception and phase through the same fenced failure endpoint
#### Scenario: Observed memory exhaustion
- **WHEN** an OutOfMemoryError is observed directly or in the cause chain
- **THEN** the worker SHALL classify it as OUT_OF_MEMORY and retain the diagnostic trace when the process can still report
### Requirement: Bounded compatible diagnostics
The server SHALL accept legacy failures without diagnostic fields and SHALL validate bounded diagnostic strings; SDK truncation SHALL be explicit.
#### Scenario: Large Unicode failure
- **WHEN** an exception message or trace exceeds the wire bounds
- **THEN** the SDK SHALL produce valid bounded UTF-8 fields and mark a shortened trace
#### Scenario: Old stored failure
- **WHEN** an old record contains only code/message/retryable
- **THEN** it SHALL remain readable without fabricated exception information
### Requirement: Correlation and failure integrity
Worker logs SHALL correlate the exception with job and attempt identity, and secondary cleanup/reporting errors SHALL NOT overwrite the original failure.
#### Scenario: Reporting unavailable
- **WHEN** reporting the failure to the server fails
- **THEN** worker logs SHALL retain the original exception and the reporting error with the job/attempt IDs
#### Scenario: Retry and stale worker
- **WHEN** a job retries and a previous attempt sends a late failure
- **THEN** the late update SHALL be rejected and earlier attempt diagnostics SHALL remain available
#### Scenario: Cross-owner access
- **WHEN** another owner requests failed job or attempt details
- **THEN** exception details SHALL remain inaccessible
#### Scenario: Deadline authority
- **WHEN** an already validated worker failure reaches finalization as its deadline elapses
- **THEN** the job SHALL retain deadline-exceeded classification while attempt history preserves the observed exception
