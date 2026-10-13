# Platform acceptance coverage

This reference summarizes the completed platform checks recorded during the
2026-09-30 disposable acceptance run. The complete requirements remain in the
versioned specification; this page maps operational behavior to executable tests.

| Behavior | Regression group |
| --- | --- |
| Raw JSON and typed Scala use the same envelope | Go/JVM fixtures and `scripts/e2e.py` |
| Acceptance commits before returning an ID | PostgreSQL store/idempotency tests and restart smoke |
| Owners cannot retrieve foreign jobs/results | HTTP authorization and PostgreSQL ownership tests |
| Retries, cancellation and stale attempts are fenced | Store/recovery tests and `platform-fault-e2e.py` |
| Client expiry drains accepted work and stops unused versions | Lease/version tests and `pod-template-e2e.py` |
| Driver replacement preserves the effective spec | Controller/template tests and Spark cluster checks |
| CPU/memory requests and limits include heap overhead | Spark resource tests and real Pod inspection |
| Concurrent Spark requests cancel independently | SparkRuntime tests and `platform-load-e2e.py` |
| Pod files, Secrets, scheduling and pull references survive replacement | PodTemplate fixtures and `pod-template-e2e.py` |
| Local/S3 JSON, files and datasets remain retrievable | `e2e.py`, `platform-results-e2e.py`, `platform-final-smoke.py` |
| Distributed results publish immutable parts | Dataset allocation/store/S3 tests |
| Missing shared mounts fail explicitly | Dataset probes and platform load checks |
| STS grants are allocation-scoped | Storage/delegation tests and MinIO fixture |
| Cleanup protects retained results and active writers | Dataset cleanup/multipart tests |
| Lost acceptance responses return one ID on replay | Idempotency tests and accepting-replica failure fixture |

## Measured defaults and limits

| Contract | Value |
| --- | --- |
| Attempt / client lease | 30 / 90 seconds |
| SDK client heartbeat | 30 seconds |
| Default / maximum attempts | 1 / 10 |
| Default / maximum result bytes | 64 MiB / 5 GiB |
| Server/worker staging default | 5 GiB |
| Dataset parts | At most 100,000 |
| SDK registration batch | At most 100 parts |
| Result retention default | Seven days |
| Delegated dataset credentials | At most one hour, no mid-write renewal |
| Physical cleanup | Opt-in; terminal/writer expiry plus ten-minute grace |

Recorded timing and scale observations are in
[the fixture summary](validation/README.md#measurements). They do not establish
production capacity, maximum-size payload support under load or infrastructure HA.
