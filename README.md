# Linha

Linha is a durable job service with a Go server and Scala/JVM SDKs. Submit a job,
receive a persisted ID, and fetch its status and results later—even after the
client, server, or worker restarts.

Business code runs in your worker image. JSON connects clients to registered
handlers; typed Scala entrypoints wrap that same protocol. PostgreSQL stores the
queue and execution history. Results live in a shared filesystem or S3. Spark is
the first managed engine, with an abstraction for additional engines.

**[Read the documentation](https://k3rnl.github.io/linha/)** ·
[Run your first job](docs/quickstart.md) ·
[Write client and worker code](docs/walkthrough.md) ·
[Deploy on Kubernetes](docs/operations.md)

## What Linha manages

- Durable acceptance, owner-scoped retrieval, idempotency and persisted attempts.
- Context versions inferred from configuration and image digest, with client leases
  and graceful draining during rolling updates.
- SparkApplication provisioning, driver replacement, bounded concurrent requests,
  driver scaling and Spark executor dynamic allocation.
- Progress, cancellation, opt-in retries, exception diagnostics and retained results.
- An administrator console, Prometheus metrics and an optional Grafana dashboard.

The server Helm chart runs two replicas by default. PostgreSQL, object storage,
shared volumes and the Spark Operator are external dependencies; their availability
must match your deployment requirements. Business side effects are not exactly-once.

## Get started

For a local development checkout, use Go 1.27.1, Java 17, Maven 3.9.9, Node.js 22.12+
with npm, Python 3 and Docker:

```sh
make check
mvn -B install
make build
python3 scripts/e2e.py
```

The last command runs a disposable PostgreSQL fixture, submits raw and typed JVM
jobs, restarts processes and checks that the original results remain available.
It cleans up its temporary resources. See the [tutorial](docs/quickstart.md) for
prerequisites and expected output.

Published SDKs use version `0.1.3`, including OAuth 2.0 client credentials and
application storage. Source builds use `0.1.3-SNAPSHOT`; Maven installs matching
source JARs alongside each SDK.

## Find your way around

| Directory | Contents |
| --- | --- |
| `server/` | Go API, PostgreSQL queue, storage and engine controllers |
| `sdk/client-jvm/` | Scala client, contexts and durable job handles |
| `sdk/worker-jvm/` | Handler registration, execution and publication |
| `sdk/protocol-jvm/` | Shared JSON contracts and codecs |
| `sdk/spark-jvm/` | Spark sessions, cancellation and dataset helpers |
| `web/` | Embedded React administrator console |
| `api/` | OpenAPI contract and cross-language fixtures |
| `examples/` | Executable JVM examples and Spark worker image |
| `deploy/helm/linha/` | Server chart and namespaced worker permissions |

See [contributing](CONTRIBUTING.md) for development checks and
[documentation maintenance](docs/docs-contributing.md) for the versioned site.
Linha is licensed under [Apache 2.0](LICENSE).
