# status-only-results Specification

## Purpose

Allow requests that write business output elsewhere to publish durable completion without allocating or retaining a separate result artifact.

## Requirements

### Requirement: Status-only completion allocates no result file
The protocol SHALL support a status result descriptor and typed unit entrypoints. Successful completion SHALL persist a result descriptor and terminal status without output allocations.

#### Scenario: Completed unit entrypoint
- **WHEN** a valid worker completes a status-only request
- **THEN** the server persists success, the typed handle resolves unit, and there are no result files

### Requirement: Status-only completion retains existing guarantees
Ownership, idempotency, execution fencing, failure diagnostics and status durability SHALL apply to status-only requests.

#### Scenario: Restart and stale completion
- **WHEN** a completed request is read after a server restart or an older worker attempts another completion
- **THEN** the original success is readable and conflicting or fenced completion is rejected
