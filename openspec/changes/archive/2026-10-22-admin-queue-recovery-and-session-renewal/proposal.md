## Why

Accepted requests can become stranded on a draining context version whose workers cannot start. Operators need a durable way to transfer pending work to a corrected version, practical request selection and filtering, and browser sessions that renew short-lived identity tokens.

## What Changes

- Add operator-only bulk cancellation and movement of queued/retrying requests between versions of the same owner's logical context, preserving public IDs, payloads, acceptance time, retry history and result policies.
- Record each move, reject races with claims and incompatible or retired destinations, and keep queue metrics accurate.
- Add row/page selection and explicit batch results to global and context-version request tables; provide searchable context choices and local date/time filters.
- Renew browser OIDC credentials on the server across HA replicas, with encrypted persisted refresh credentials, refreshed membership checks, and the configured absolute session limit.

## Capabilities

### New Capabilities

- `admin-queue-recovery`: Durable, audited movement and bounded bulk request actions.
- `admin-request-selection`: Searchable filters, date/time inputs and shared batch controls.
- `renewable-browser-sessions`: HA-safe OIDC refresh within a bounded browser session.

### Modified Capabilities

None; these capabilities extend the existing administrative and browser-session contracts.

## Impact

Go administrative HTTP APIs, PostgreSQL schema/read models/metrics, browser authentication, OpenAPI, React request tables and filters, Helm/session documentation and integration tests. No SDK or engine-specific migration API is required. Existing ordinary request ownership and attempt fencing remain enforced.
