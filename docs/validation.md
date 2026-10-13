# Validation scope

Linha has unit, database, JVM, browser, chart and disposable-cluster checks. This
page distinguishes tested behavior from deployment assumptions. Run logs and
environment-specific identifiers are local/CI artifacts, not public documentation.

## Test surfaces

| Surface | Covered behavior |
| --- | --- |
| Go + PostgreSQL | Durable acceptance, idempotency, ownership, leases, retries, cancellation, migrations and publication |
| JVM SDK | Codecs, transport, credential refresh, raw/typed dispatch, result helpers and diagnostics |
| Process restart smoke | Original job IDs and retained results after client/server/worker restarts |
| Spark/Kind | SparkApplications, concurrent requests, driver replacement, allocation, templates and result mounts |
| Browser | Login/session/role boundaries, navigation, replay, cancellation, previews and current/history views |
| Prometheus/Grafana | Metric contracts, deduplication, resets, outages, stale observations and dashboard expressions |
| Helm | Defaults, Secret references, namespace RBAC, UI ingress and optional monitoring resources |
| Release rehearsal | Metadata, signed SDK bundles, sources, API docs and checksum validation |

See [acceptance coverage](acceptance.md),
[admin/observability measurements](admin-observability-validation.md) and
[recorded fixture measurements](validation/README.md).

## Reproduce the routine checks

```sh
make check
mvn -B verify
make frontend-check
python3 scripts/e2e.py
python3 scripts/check-public-content.py
```

Set `LINHA_TEST_DATABASE` to a **disposable** PostgreSQL database for integration
checks; otherwise those scenarios are explicitly skipped. Tests create/remove
isolated schemas.

```sh
LINHA_TEST_DATABASE='postgres://user:password@localhost/linha_test?sslmode=disable'   go test -race -count=1 ./server/...
```

The CI workflow supplies its database fixture and runs the contract, chart,
publication, browser and restart suites. Cluster fixtures are separate; see
[run disposable Kubernetes validation](kubernetes-validation.md).

## Limits of the evidence

Single-host Kind tests establish application coordination and recovery, not
multi-node database, object-store or filesystem HA. Production IAM, admission
policy, storage failures and capacity need tests in the target environment.
Measured fixture latency is not an SLO or a maximum throughput guarantee.

The current database schema is 10. Older binaries can reject it; retaining IDs
during migration does not make arbitrary binary rollback safe. Job metadata is
retained indefinitely; payload retention and cleanup are separately configured.
