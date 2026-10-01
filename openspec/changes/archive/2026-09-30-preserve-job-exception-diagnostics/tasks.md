## 1. Failure contract

- [x] 1.1 Extend validated failure JSON and durable job/attempt persistence, retaining old records and fencing.

## 2. Worker diagnostics

- [x] 2.1 Capture bounded exception traces and phases across initialization/execution/publication, classify observed OOM, and log correlated primary/secondary failures.

## 3. Verification and delivery

- [x] 3.1 Verify nested, suppressed, oversized, initialization and OOM exceptions in JVM worker tests; verify PostgreSQL persistence, ownership, retries and stale attempts.
- [x] 3.2 Verify diagnostics through the real HTTP/JVM restart smoke, update OpenAPI/docs, build JVM artifacts and the server image, and record evidence.
