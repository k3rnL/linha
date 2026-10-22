## Context

See proposal.md. Storage currently serves retained output allocations and delegated Spark result datasets. Application dataset writes have no corresponding access boundary. Client leases and worker attempt fences already live in PostgreSQL.

## Goals / Non-Goals

Goals: reuse server storage credentials; support S3-compatible stores without STS configuration; provide HA authorization and reusable locking; support status-only handlers.

Non-goals: exactly-once business side effects, automatic application-file retention, credentials in queued requests, and additional processing engines.

## Decisions

- Add an optional application-storage policy to backend specifications. Named S3 destinations require an explicit administrator `allowWorkloadAccess` flag. Snapshot destination fingerprints during ensure for immutable context versioning. This is distinct from job result storage and its GC.
- Use an HTTP storage gateway with opaque, hashed PostgreSQL sessions. Client sessions bind context ownership and client lease; worker sessions bind job, attempt, worker incarnation and fence. Every operation validates the current lease and destination configuration. This supports common S3-compatible stores without their own STS service. STS-only credentials were considered but introduce provider requirements and executor refresh limits.
- Stream bounded object IO through the configured backend. Use a server staging reservation for bounded writes, and range reads for random-access data formats. Return no reusable S3 credentials. This trades additional server bandwidth/staging IO for a simple, provider-independent access boundary.
- Persist renewable locks by physical destination fingerprint and canonical object key. Validate protected writes under a locked database row to serialize metadata commits. Locks are released on close and recover after expiry; they do not claim exactly-once execution.
- Add a session-specific Hadoop filesystem in a dedicated JVM binding. Every grant has a distinct URI authority so simultaneous requests cannot replace each other's configuration. Spark passes those options to executors. SDK sessions carry only scoped opaque Linha tokens.
- Add `status` result descriptors and unit codec support. Worker publication skips allocations; normal persisted completion and fencing remain authoritative.

## Risks / Trade-offs

- Gateway bandwidth and temporary server disk → bounded streams, staging reservations, existing HA replicas, documented capacity requirements.
- In-flight external writes during worker loss → do not claim exactly-once; reject new stale-session IO and enforce metadata lock ownership.
- Enabling a shared application prefix exposes that prefix to authorized contexts → opt-in destination setting and dedicated application prefixes, never implicit access to retained-result prefixes.
- Lock-holder process loss → renewable leases and expiry; protected writes require a current lock.

## Migration Plan

Deploy the matching server and SDKs, apply the additive PostgreSQL schema, and explicitly enable an application destination. Old backends without application storage stay unchanged. Consumers update to self-contained per-item requests and remove their worker mounts. Disable destination application access to revoke sessions if rollback is required; retain application objects and request history.
