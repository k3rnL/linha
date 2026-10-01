# Linha

Linha is a Go job server with Scala/JVM client and worker SDKs. Read `README.md`
and `openspec/config.yaml` before changing the project.

Use OpenSpec for feature work. Before implementing, read the active change's
proposal, design, capability specs, and tasks using the paths returned by the
OpenSpec CLI. Keep completed tasks aligned with actual implementation and checks.

Keep request management independent of engine adapters. Persist public job IDs
before acknowledgement. Preserve ownership, idempotency, attempt fencing, and
result durability across process restarts. Treat native engine IDs as internal.
Do not claim exactly-once execution for user side effects.

The repository is a pre-release implementation. See docs/validation.md and the
OpenSpec tasks for verified capabilities and remaining work. Use make check for
Go formatting, compilation and vet; mvn package with Java 17 tests the JVM SDKs.
Run PostgreSQL integration checks with LINHA_TEST_DATABASE and restart smoke tests
with scripts/e2e.py. Do not represent skipped integration tests as passed.
