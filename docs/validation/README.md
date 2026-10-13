# Recorded fixture measurements

The September 30 platform run used one Kind host, two allowed drivers, one
request slot per driver and 0–2 one-core executors per driver. Both roles used
512m heap plus 384m overhead. Executor idle/cache/shuffle timeouts were 5s/8s/8s;
driver idle was 120s.

## Measurements

| Observation | Recorded value |
| --- | --- |
| Durable acceptance, seven submissions | 2.96–4.99 ms |
| Four load-job queue waits | 7.44s, 7.47s, 37.84s, 38.04s |
| Peak drivers / executors / simultaneous requests | 2 / 4 / 2 |
| Retained request RDDs after completion | 0 |
| Additional wait for executor retirement after the UI/cache check | 4.24s |
| Additional wait for idle driver retirement after later checks | 107.44s |

Polling began after other checks, so these retirement observations are not exact
time from the final Spark task. They are fixture measurements, not capacity or
latency promises.

## Checks

PostgreSQL/S3 restarts, API replica replacement and complete driver shutdown
preserved local/S3 result access. Signed-token tests checked cryptographic
identity rules separately. The fixture did not certify production IAM,
multi-node storage failure or load at result-size/part-count ceilings.

Executable groups include `e2e.py`, `platform-results-e2e.py`,
`platform-fault-e2e.py`, `platform-load-e2e.py`, `platform-final-smoke.py`,
`pod-template-e2e.py` and `test-helm.py`. See
[validation scope](../validation.md) and [acceptance coverage](../acceptance.md).

Generated request IDs, image digests and raw run logs stay outside Git. CI can
retain them as run artifacts when needed.
