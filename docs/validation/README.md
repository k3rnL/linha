# Validation summary

This directory keeps a curated summary of the development acceptance run.
Raw `.log` files and generated `.json` records are local, ignored artifacts.
The implementation and executable checks are versioned; run output can be retained
as CI artifacts when needed. The complete scenario mapping is in
[acceptance](../acceptance.md).

## Checks

The 2026-09-30 platform acceptance passed Go race tests with disposable PostgreSQL
17 and MinIO, JVM tests, Helm render/schema checks, OpenAPI validation and strict
OpenSpec validation. Tests exercised durable acceptance, retries and lease
fencing, ownership, source-independent retrieval, local/S3 datasets, immutability,
retention cleanup, staging bounds and shared-mount failures.

Reproducible checks:

- [Raw/typed JVM restart lifecycle](../../scripts/e2e.py).
- [Concurrent local/S3 dataset jobs, failed writes and restart retrieval](../../scripts/platform-results-e2e.py).
- [Database outage and expired-worker fencing](../../scripts/platform-fault-e2e.py).
- [Spark Operator scaling, UI, datasets and API replica rollout](../../scripts/platform-load-e2e.py).
- [Final image smoke test and retained results](../../scripts/platform-final-smoke.py).
- [Pod templates, driver replacement and leased context versions](../../scripts/pod-template-e2e.py).
- [Helm security/RBAC/configuration rendering](../../scripts/test-helm.py).

The source-JAR installation check on 2026-10-01 also passed all 16 JVM tests.

## Measurements

The September 30 load run used one Kind host, two permitted drivers, one request
slot per driver, and 0–2 one-core executors per driver. Both roles used a 512m heap
and 384m overhead. Executor/cache/shuffle timeouts were 5s/8s/8s; driver idle was
120s. These are fixture observations, not production capacity promises.

| Observation | Result |
| --- | --- |
| Durable acceptance, seven submissions | 2.96–4.99 ms |
| Queue wait for four load jobs | 7.44s, 7.47s, 37.84s, 38.04s |
| Peak drivers / executors / concurrent requests | 2 / 4 / 2 |
| Retained request RDDs after completion | 0 |
| Additional wait for all executors to retire after the UI/cache check | 4.24s |
| Additional wait for idle drivers to reach zero after later checks | 107.44s |

PostgreSQL and S3 container restarts, two API replica replacements and complete
driver shutdown preserved access to retained local/S3 results. Production IAM,
an external OIDC provider, multi-node filesystem failure and full-limit payload
load are outside this single-host validation scope. See the
[acceptance limits](../acceptance.md#measured-defaults-and-limits).
