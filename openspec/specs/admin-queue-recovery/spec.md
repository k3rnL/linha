# admin-queue-recovery Specification

## Purpose

Allow operators to recover pending work stranded on an unusable context version without replacing durable request identities.

## Requirements

### Requirement: Durable movement of pending requests
Operators SHALL move QUEUED or RETRYING requests only to a different active version of the same owner's logical context and engine type/version whose workload storage destinations cover those of the source. Moves SHALL preserve request ID, owner, idempotency scope, payload, submission time, retry history, deadline, backoff and original result policy. Expired, running, cancelling and terminal requests SHALL NOT move. Each actual move SHALL have an audit record containing actor, source and destination. Current queue gauges SHALL follow the request; historical execution counters SHALL remain attributed to where work occurred.

#### Scenario: Recover an unusable draining version
- **WHEN** an operator moves pending requests to a compatible corrected version
- **THEN** the same IDs remain pollable and claimable only on the destination, and the source can finish draining after its remaining accepted work ends

#### Scenario: Worker claim races with a move
- **WHEN** a worker claim and operator move target the same pending request concurrently
- **THEN** exactly one wins the request lock and the other observes its resulting state without creating a duplicate attempt

#### Scenario: Invalid target or changed source
- **WHEN** ownership, logical name, engine, storage access, destination lifecycle or expected source context does not match
- **THEN** the move fails with a clear conflict and leaves the request and audit unchanged

#### Scenario: Repeated operation and repeated genuine moves
- **WHEN** a completed move is retried or a request is later moved again
- **THEN** the retry does not duplicate work or audit while each genuine subsequent move remains audited

### Requirement: Bounded operator batch actions
An operator-only API SHALL accept at most 200 explicitly selected unique requests with their expected source contexts for cancellation or movement. It SHALL validate the batch shape before effects, commit each request action independently, and report success, no-op or a safe error per request. Viewer access and browser mutations without valid same-origin CSRF SHALL be denied.

#### Scenario: Mixed outcomes survive restart
- **WHEN** some selected requests are eligible and others have been claimed or cancelled
- **THEN** eligible operations commit durably, ineligible entries report their conflicts, and successful actions remain visible through a different server replica after restart
