## 1. Durable queue recovery

- [x] 1.1 Add domain contracts, schema migration, repeatable move audit and current queue metric transfer.
- [x] 1.2 Implement bounded per-item cancel/move actions with context compatibility, expected-source and claim fencing checks; expose authenticated API and OpenAPI contracts.
- [x] 1.3 Verify moves, idempotency, partial outcomes, audit, metrics, source drain, destination execution and claim races against PostgreSQL.

## 2. Request filtering and selection

- [x] 2.1 Add searchable context suggestions and local date/time filtering with range validation.
- [x] 2.2 Add shared page selection, cancel/move confirmation, destination choices and per-item results to global/context request tables.
- [x] 2.3 Verify both tables, viewer restrictions, date conversion, destination selection, partial errors and selection reset in browser tests.

## 3. Renewable browser sessions

- [x] 3.1 Persist encrypted refresh state and separate authentication expiry; serialize refresh across replicas and preserve absolute lifetime/logout semantics.
- [x] 3.2 Validate refreshed identity/membership and UserInfo fallback; classify provider failures without leaking credentials or bypassing OIDC TLS policy.
- [x] 3.3 Verify rotation, replica races, expiry, revocation, membership changes, token bindings, fallback and logout using signed OIDC and PostgreSQL fixtures.

## 4. Documentation and delivery

- [x] 4.1 Document queue recovery, batch limits, session lifetime and provider refresh prerequisites using generic examples.
- [x] 4.2 Run formatting, Go/database/browser checks, contract and OpenSpec validation; verify implementation against all scenarios and rebuild the server image.
