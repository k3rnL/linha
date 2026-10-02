# Admin and observability acceptance — 2026-10-01

The two changes add 59 tasks: 28 for the admin console and 31 for observability.
The shared operational read model is schema 7, durable accounting is schema 8,
and admin sessions/audit/provenance are schema 9. Ownership, job IDs, completion
receipts, context configuration hashes and result paths remain unchanged.

## Verification mapping

| Change/tasks | Verified behavior | Evidence |
| --- | --- | --- |
| UI 1.1–1.4; observability 1.1–1.2 | Shared DTOs, sequential migrations, owner-derived reads, indexed bounded pagination and stable filter-bound cursors | domain operations/admin contracts; postgres operations/admin tests; actual OpenAPI response conformance |
| Observability 1.3–1.6 | Current owned executor pods, allocation completeness, epoch/application/pod fencing, server/client timestamps and unchanged accepted-work lifetime | engine observations tests; controller tests; real Spark replacement/retirement; full restart |
| UI 2.1–2.4 | Issuer-bound roles, deny-by-default access, viewer mutation rejection, signed code/PKCE/nonce/state/audience/expiry, secure cookie/CSRF/logout and shared sessions | auth admin tests with independent PostgreSQL pools; HTTP admin authorization tests; Helm matrix |
| UI 3.1–3.6 | Atomic administrative acceptance/provenance/audit, lost-response retry, owner preservation, edited replay, explicit activation, cancellation fencing and published-only results | postgres admin/store/dataset tests; HTTP contract tests; real accepting-pod loss and concurrent Spark cancellation |
| UI 4.1–4.9 | Embedded assets, scoped fallback/cache, inspection screens, edited replay, cancellation, escaped preview, stale links, polling and large integer preservation | webui tests; eight Playwright flows; actual served image and operator-created ingress |
| UI 5.1–5.5 | Default-off UI, Secret references, namespace-only RBAC, CI/docs, rollout and restart recovery | Helm tests; signed OIDC fixture; fresh two-replica cluster; build and strict OpenSpec validation |
| Observability 2.1–2.7 | Persistent capped aliases and accounting baseline, duplicate-safe events/histograms, current gauges, manifest bytes and lease generations | postgres metric tests, concurrent completion/recovery/expiry tests and durable restart ledger comparison |
| Observability 3.1–3.7 | Typed HELP/TYPE, runtime/HTTP/pool/loop/Kubernetes/storage metrics, cached bounded collection, compatibility, cardinality and concurrent scrapers | telemetry/HTTP/storage tests; live Prometheus exposition; retained-history fixture and outages |
| Observability 4.1–4.6 | Eight-row dashboard, correct HA/reset/outage/stale PromQL, independent monitoring resources and actual imports/discovery | check-observability.py; Helm render combinations; live Prometheus/Grafana/operators |
| Observability 5.1–5.5 | Replica replacement/full restart, database/Kubernetes/result failures, real Spark scale-down/replacement, recovery and documentation | admin-spark-e2e.py; admin-observability-e2e.py; Go/DB race, JVM and restart suites; strict OpenSpec validation |

## Measured fixtures

The retained-history fixture used 25,000 jobs and 120 logical contexts with the
default 100 named labels plus one overflow dimension. It emitted **3,945 series**,
collected the coherent snapshot in **23.3 ms**, and read a 50-context admin page
in **1.3 ms**. Eight concurrent callers completed 80 cached scrapes in **48.1 ms**
without acquiring any database connection. These are measurements on one host,
not maximum-capacity guarantees or a worst-case population of every histogram.

HTTP tests used 1,000 arbitrary paths and methods and retained two normalized
request-counter series. Completion fixtures sent 12 concurrent identical receipts
and recorded one job/attempt completion and one histogram observation. Eight
recovery/expiry callers recorded each retry/expired generation once.

The disposable Kind fixture ran two API replicas, PostgreSQL 17, Spark 3.5.6 with
Operator 2.4.0, two drivers with two request slots each, and 0–2 executors per
driver. Heap was 512m plus 384m overhead. Executor idle/cached/shuffle timeouts
were 5s/8s/8s. Current observed executors grew to four and returned to zero.
Lost applications were replaced; retired instances lost their UI URLs. Edited
replay succeeded, cancellation preserved unrelated work, and explicit activation
completed and drained without a browser/client lease.

Prometheus 3.7.1 / Prometheus Operator 0.94.1 discovered two independent pods,
including both unready replicas during database failure. Grafana 13.2.3 /
Grafana Operator 5.25.0 reconciled the dashboard: eight rows, 51 data panels and
70 evaluated expressions. Traefik 3.7.13 (chart 41.6.1) routed an operator-owned
Spark ingress using the existing host. Linha created no Spark ingress or proxy.

All server replicas were stopped with QUEUED, RUNNING and CANCELLING requests
present. The durable accounting ledger was identical before worker resumption;
work recovered, cancelled work stayed cancelled and the other five jobs finished.
Dropping the accepting pod after committed response headers, then retrying the
same envelope through another pod, returned one ID and one accepted audit record.
Shared Prometheus job totals matched PostgreSQL after replica replacement/restart.

A missing published local file returned 503, retained its metadata and recovered
when restored; server storage error metrics recorded it. Pausing PostgreSQL made
both servers unready while `/metrics` returned 200 with local metrics and omitted
shared families. Revoking namespaced Kubernetes reads produced forbidden metrics
and stale observations; restoring access recovered fresh snapshots. Tests wait
for registered workers **and** fresh READY instance observations before injecting
this fault, because these states can appear at different times.

## UI ingress follow-up

`add-ui-ingress` adds optional Helm-managed UI/admin routing. The 12-test Helm
suite checks default-off compatibility, canonical host/port handling, fixed Prefix
routes to the existing two-replica Service, OIDC/TLS/class/annotations, invalid
hosts/settings and namespace-only RBAC. Strict Helm lint passed for disabled and
secured ingress configurations, along with formatting, `make check` and strict
OpenSpec validation. No database integration or live UI ingress-controller rollout
was repeated for this chart-only addition; previous runtime/HA evidence above is
unchanged. [UI setup](admin-ui.md) includes the HTTPS values example.

## Admin refinements — 2026-10-02

`refine-admin-experience` adds schema 10, the Overview landing page (including
post-login routing), client hostname reporting, independently paginated current
instances/history, and dashboard version 2 under the existing UID.

- PostgreSQL 16 integration/race checks passed for populated 9→10 and legacy 3→10
  migrations, unchanged context hashes/lease generations, old-client compatibility,
  complete counts beyond list size, completion-based 24-hour windows, retry-aware
  processing averages, bounded details, timeout handling, owner isolation and
  history pagination. Retired records cannot hide an older running instance.
- Overview and Prometheus server counts use the newest heartbeat per pod, preventing
  an old process record from inflating availability after a restart.
- The retained-history fixture covered 25,000 requests and 120 contexts. Overview
  returned complete counts and five context details in **13.2 ms** under the race
  build on this host. This is a fixture measurement, not a production guarantee.
- **11 Playwright flows** passed, including the new landing page, complete totals,
  loading/error/empty states, viewer controls, current/historical allocations,
  lazy history and hostname fallback. Overview/context screenshots were inspected.
- **17 actual admin responses** passed OpenAPI validation. Viewer/ordinary-user
  authorization and signed OIDC callback routing passed.
- **40 PromQL expressions** passed Promtool. Fixtures execute generated queries
  for HA replica loss, selected-range outcomes, queue maximums and whole-total
  masking when another selected context has missing resources. The dashboard has
  17 primary panels and four collapsed diagnostic panels. **13 Helm tests** and
  strict lint passed, including Overview/Contexts/Requests links.
- All **16 JVM tests** passed on Java 17. Tested jars and sources were installed
  locally; the ten SDK source files match their installed source archives.
- Formatting, Go compilation/vet, PostgreSQL race checks, embedded frontend build,
  Go/PostgreSQL/JVM restart smoke and strict OpenSpec validation passed. The local
  image served the embedded console and preserved overview/hostname data across
  container restart. The image is tagged `linha/server:admin-refinements` and
  `linha/server:observability-ui`; no registry push was performed.

The new layout was checked through generated JSON, Helm and Promtool, not a new
live Grafana Operator deployment. The earlier real-Spark/operator/HA measurements
above describe the initial implementation and were not repeated for this UI and
read-model refinement. Resource charts show allocations, not measured consumption.
Upgrade the server before clients that send hostnames. Existing clients work
without that field and display “Hostname not reported” until upgraded/re-attached.

## Repeatable checks

Use the commands in README and Java 17. Database checks require a disposable
PostgreSQL DSN; skipped cases are not acceptance evidence:

```sh
make frontend-check
make check
make format-check
LINHA_TEST_DATABASE=YOUR_TEST_DSN go test -race ./server/...
LINHA_TEST_DATABASE=YOUR_TEST_DSN LINHA_METRIC_LOAD=1 \
  go test -count=1 -v ./server/internal/postgres -run TestMetricRetainedHistoryAndConcurrentScrapers
LINHA_TEST_DATABASE=YOUR_TEST_DSN LINHA_ADMIN_FIXTURE_OUTPUT=/tmp/linha-admin-fixtures.json \
  go test -count=1 ./server/internal/httpapi -run TestAdminOpenAPIResponseFixtures
uv run --no-project --with jsonschema==4.25.1 \
  python scripts/check-observability.py --fixtures /tmp/linha-admin-fixtures.json
python3 scripts/test-helm.py
mvn -B package
make build
python3 scripts/e2e.py
openspec validate --all --strict
```

`frontend-check` needs installed Playwright Chromium/system libraries; CI uses
`npx playwright install --with-deps chromium` inside `web/`. Browser flows cover
read-only views, viewer controls, edited replay/lost responses, HTTP key generation,
cancellation, stale Spark links, escaped active content and JVM-sized integers.
The API check validates 17 actual PostgreSQL-backed responses against OpenAPI.

For cluster acceptance, build the server image and the repository's Spark example
image first (see the Kubernetes/example guides). Docker, Kind, kubectl, Helm,
Git and network access to pinned operator charts/images are required:

```sh
docker build -f server/Dockerfile -t linha/server:observability-ui .
python3 scripts/admin-observability-fixtures.py up --state /tmp/linha-ui-state.json \
  --worker-image linha/spark-example:platform-complete
python3 scripts/admin-spark-e2e.py --state /tmp/linha-ui-state.json
python3 scripts/admin-observability-e2e.py --state /tmp/linha-ui-state.json
python3 scripts/admin-observability-fixtures.py clean --state /tmp/linha-ui-state.json
```

Fixture creation generates a unique Kind context and uses it explicitly for all
cluster calls. Cleanup checks its ownership marker/container label and removes
only that fixture, including its PostgreSQL and result volume. Raw logs, resource
IDs, image digests and generated evidence stay outside Git. `--outages-only` can
repeat the failure section after a successful Spark seed without repeating HA load.

## Limits and migration

This is application recovery on one Kind host, with persistent fixture PostgreSQL
and POSIX storage. It does not prove infrastructure HA, multi-node storage failure,
maximum payload/dataset/history capacity or production IAM. OIDC tests use a signed
local provider and independent database-backed admin instances; a production
external identity provider was not deployed in the cluster. Existing local/S3
compatibility and restart tests remain passing; this new live storage fault used
local published content, not an injected remote S3 outage.

An older binary rejects the current schema 10. Exposure rollback disables the UI/monitoring;
there is no automatic schema downgrade. Back up/rehearse upgrades in the target
environment. No production rollout, image push, commit or METOC modification was
part of this implementation. See [admin setup](admin-ui.md),
[observability](observability.md) and the [metric catalogue](metrics.md).
