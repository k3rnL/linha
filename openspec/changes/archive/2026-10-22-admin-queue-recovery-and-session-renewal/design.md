## Context

The existing PostgreSQL queue scopes idempotency by owner and logical context name, and claims lock individual jobs. Admin cancellation already has an audit trail. Browser login persists only claims and caps the cookie at ID-token expiry, discarding refresh credentials. See proposal.md for motivation.

## Goals / Non-Goals

Goals: recover pending work without changing its public identity; preserve fencing, accepted-work lifetime and HA behavior; improve selection and filters; renew verified browser identities.

Non-goals: moving active execution, cross-owner/name migration, rewriting opaque business payloads, automatic migration without operator intent, or unlimited browser sessions.

## Decisions

- Add a bounded batch endpoint with request IDs and expected source IDs, action and optional target context ID. Each item commits independently and returns an explicit outcome and compact job summary, keeping large request payloads out of batch responses. Moves lock source/destination contexts in ID order before locking the job, reject nonpending states and retired/incompatible targets, and preserve the immutable request/result policy. Existing cancellation logic is shared. A retry already on the requested target is a no-op.
- Preserve logical names and idempotency keys. The existing owner/name key remains stable; original submissions continue to return the same job. Storage aliases permitted by the source must remain permitted by the destination. Handler compatibility remains the operator's responsibility, with normal worker capability matching still enforced.
- Add source-context audit metadata and permit repeated move events while retaining once-only acceptance/cancel records. A separate context-change trigger transfers current job gauges without transferring historical counters. Context reconciliation is nudged on both versions.
- Reuse one table for page-local row selection, confirmation, compatible destination selection and per-item feedback. Load paginated context choices, including stopped versions, for searchable suggestions. Native local datetime controls convert to ISO UTC at the API boundary.
- Persist encrypted OAuth refresh state and separate authentication expiry from the absolute session expiry. Derive an AES-GCM key using domain-separated hashing of the unpersisted random session cookie; the database contains only its independent lookup hash and ciphertext. This permits HA renewal without another deployment secret or plaintext provider credentials.
- Serialize refresh under a PostgreSQL session-row lock, rechecking authentication expiry after lock acquisition. Validate fresh ID-token bindings and membership; use fresh subject-checked UserInfo if the provider omits an ID token, as permitted by OpenID Connect Core. The original login nonce and browser client binding travel inside encrypted state; the original issuer is checked before any refresh request. Cached identities are also bound to the current issuer and client. Refresh never extends the configured absolute lifetime. Invalid grants/identity revoke; transient errors preserve the refresh credential and cannot authorize beyond authentication expiry. Logout uses the same row ordering to prevent resurrection.

## Risks / Trade-offs

- Opaque business contracts can change between versions → require explicit destination confirmation, constrain logical owner/name/engine/storage, and retain normal worker matching.
- A batch can have mixed outcomes → per-request transactions and visible item results allow targeted recovery without pretending the batch is atomic.
- Provider refresh rotation can race across replicas → session row serialization and bounded HTTP calls.
- Providers may require extra scopes to issue refresh credentials or expose groups in UserInfo → document provider requirements and preserve sign-in when refresh is unavailable.
- Old binaries reject the new schema version → deploy matching replicas together; rollback requires a database backup or a forward fix.

## Migration Plan

Add an atomic schema migration for session authentication/credential columns, move audit source metadata and current-queue metric transfer. Existing nonrenewable sessions remain valid only until their existing expiry; new logins obtain renewable sessions when the provider supports them. Deploy the rebuilt server with embedded UI, verify both replicas, then use the batch move dialog for stranded queues. Retain IDs/results across restart and test cross-replica refresh against disposable fixtures.
