# Administrator and observability validation

This reference records the 2026-10-01 acceptance and 2026-10-02 interface/dashboard
refinement fixtures. It describes measured checks, not a deployment guarantee.

## Verified contracts

| Area | Evidence |
| --- | --- |
| Admin authorization | Signed OIDC state/PKCE/nonce/audience/expiry, deny-by-default roles, viewer mutation rejection and CSRF tests |
| Shared sessions | Independent PostgreSQL pools and replica restart with retained sessions |
| Cross-owner inspection/actions | Bounded cursors, operator acceptance, audit/provenance and cancellation fencing |
| UI | Overview, client hostnames, lazy current/history pagination, replay, previews, viewer controls and 11 Playwright flows |
| Current engine observations | Epoch/application/Pod fencing, executor retirement, replacement and stale UI links |
| Durable metrics | Duplicate-safe transitions, retries/lease events and accounting preserved after all-server restart |
| Collection | Cached bounded snapshots, normalized route labels, overflow labels and failure omissions |
| Dashboard | 40 generated PromQL expressions, replica loss/reset/outage fixtures and allocation masking |
| Helm | Optional UI ingress, namespace RBAC and independent operator resources |

## Measured fixtures

A fixture with 25,000 retained jobs and 120 logical contexts produced 3,945
series under the default 100 named dimensions plus overflow. Snapshot collection
took 23.3 ms and a 50-context admin page took 1.3 ms. Eight concurrent callers
performed 80 cached scrapes in 48.1 ms without acquiring a database connection.

The Overview query returned complete counts and five context details in 13.2 ms
under the race build on that host. These are observed fixture values; histogram
population, hardware and production data distributions can change them.

The disposable Spark fixture used two API replicas, two drivers with two request
slots each and 0–2 executors per driver. Observed executors grew to four and
returned to zero. Lost applications were replaced and retired UI URLs disappeared.
Edited replay succeeded; cancellation preserved unrelated requests.

## Failure behavior

- Pausing PostgreSQL made both servers unready. `/metrics` still returned local
  data and omitted unavailable shared families.
- Revoking Kubernetes reads produced forbidden API metrics and stale observations;
  restoration recovered fresh totals.
- Removing a published local file returned an explicit storage failure without
  deleting its metadata; restoring the file recovered retrieval.
- Losing an accepting replica after commit and retrying the same request returned
  one job ID and one acceptance audit entry.
- Full server restart preserved the accounting ledger; queued/running work recovered
  and cancellation remained authoritative.

Run [the repository scripts](https://github.com/k3rnL/linha/tree/main/scripts),
including `check-observability.py`, `admin-spark-e2e.py` and
`admin-observability-e2e.py`, for the corresponding fixtures. Raw output and exact
environment identifiers are omitted from the public record.
