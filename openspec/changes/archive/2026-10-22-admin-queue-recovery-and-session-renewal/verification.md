# Verification

| Dimension | Result |
| --- | --- |
| Completeness | 11/11 tasks; 6/6 requirements implemented |
| Correctness | All 12 specified scenarios mapped to implementation and checks |
| Coherence | Request IDs, owner/name idempotency, attempt fencing, original result policy and HA session boundaries preserved |

## Evidence

- `server/internal/postgres/admin_batch_test.go`: durable cross-pool moves, SDK/admin idempotency, unchanged request/result policy, repeated move audit, partial outcomes, target compatibility, retry history/backoff, claim races and rollback on audit failure.
- `server/internal/controller/controller_test.go`: moving stranded work lets the old version reach STOPPED while the destination retains the same queued IDs and execution demand.
- `server/internal/httpapi/admin_test.go` and `admin_contract_test.go`: operator/viewer and same-origin CSRF checks, per-item response schemas and existing owner isolation.
- `web/tests/queue-recovery.spec.ts`: both tables, selection scope, cancellation, compatible destination choice, partial failure retention, styled context suggestions, mouse/keyboard selection, free-text filters, timezone conversion, invalid range and viewer controls. Move and cancel dialogs are centered and remain usable at desktop, portrait-phone and landscape sizes. The complete browser suite passes 20 tests, including all nine recovery and UI regression tests.
- `server/internal/auth/browser_refresh_test.go`: signed HTTPS OIDC responses, concurrent rotation across independent PostgreSQL pools, encrypted credential persistence, subject/issuer/audience/nonce/signature/access-token-hash/expiry/membership rejection, UserInfo fallback, transient provider failures, rotated-token retention and nonrenewable sessions and issuer/client reconfiguration before credential reuse.
- `server/internal/postgres/admin_test.go`: concurrent logout/refresh cannot resurrect a session. Existing session-expiry tests cover the absolute database expiry.
- Go race checks pass with PostgreSQL; `make check` passes with PostgreSQL. Frontend types/format/build, SQL layout, OpenAPI validation, 18 real HTTP response contracts, strict documentation/version checks and external public-content denylist checks pass.
- The statically linked server image starts as its non-root runtime user, migrates a disposable PostgreSQL database, and returns successful readiness/liveness responses. Docker still rejects the original incompatible dynamic binary.

## Limits

Validation uses disposable local containers and signed provider fixtures. It does not claim a rollout into an external Kubernetes cluster or a real provider account. Destination handler/payload compatibility remains an explicit operator decision; active execution and cross-owner/logical-context moves are rejected. Existing sessions need a new login to obtain refresh credentials, and provider revocation or the configured absolute session limit still ends access.

No critical issues or unresolved verification warnings remain. The change is ready for archive.
