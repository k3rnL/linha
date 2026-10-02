# Linha

Linha is a durable job service with a Go server and Scala/JVM client and worker
SDKs. The name means *ficelle* (string/thread) in French.

[Admin console](docs/admin-ui.md) covers contexts, Spark UI links, request replay,
cancellation and retained results. [Observability](docs/observability.md) covers
Prometheus, the Grafana dashboard and optional Helm monitoring resources.

[Job failure diagnostics](docs/job-failures.md) explain persisted exception types,
stack traces, attempt history and correlated worker logs.

Submit JSON, receive a persisted job ID immediately, and retrieve status and
results later. Typed Scala entrypoints wrap that JSON protocol. The worker image
contains your executable business code; classes and closures are not sent over
the wire. PostgreSQL owns queue state and execution attempts. Result bytes live
in a persistent local filesystem or S3.

The pre-release implementation includes managed Kubernetes Spark drivers,
bounded concurrency, cancellation, attempt fencing, opt-in retry, driver
replacement, driver scaling, and a two-replica Helm deployment. Real tests cover
complete process restarts with queued/running/cancelling requests and retrieval
of earlier local/S3 results. See [validation evidence](docs/validation.md)
and the [OpenSpec tasks](openspec/changes/archive/2026-09-30-establish-linha-platform/tasks.md).
Distributed Parquet helpers support local/S3 output, paginated manifests and
immutable published parts. [Dataset setup and limits](docs/dataset-results.md)
cover STS delegation, staging quotas and opt-in retention cleanup. Start with the
[client and worker walkthrough](docs/walkthrough.md).

[Release setup](docs/releases.md) covers GitHub CI, GHCR images and signed
`com.k3rnl` SDK publication to Maven Central.

## Layout

- `web/`: embedded React/TypeScript administrator console, disabled by default.
- `server/`: Go API, PostgreSQL queue, storage providers and engine controllers.
- `sdk/client-jvm/`: JSON and typed Scala clients, context/job handles and retrieval.
- `sdk/worker-jvm/`: handler registration, dispatch, leases, progress and results.
- `sdk/protocol-jvm/`: shared wire contracts/codecs; `sdk/spark-jvm/`: Spark integration.
- `api/openapi.json`: versioned HTTP contracts; `api/fixtures/`: cross-language fixtures.
- `examples/jvm/`: executable client, raw/typed worker and Spark job examples.
- `deploy/helm/linha/`: HA API deployment and worker RBAC.

## Build and run the restart example

Use Go 1.27.1, Java 17, Maven, Node.js 22.12+ with npm, Python 3 and Docker. See the
[compatibility matrix](docs/compatibility.md).

```sh
make check
mvn -B package
make build
python3 scripts/e2e.py
```

To install the JVM artifacts for another local project, run `mvn -B install`
with Java 17. Each SDK and the example module installs its binary and matching
`-sources.jar` under `com.k3rnl`, version `0.1.0-SNAPSHOT`, in the local Maven
repository. Reload your IDE's Maven project to pick up the sources.

The smoke test creates its own temporary PostgreSQL container and result root,
submits through the Scala client, starts the worker, runs raw and typed jobs,
checks file/callback-failure behavior, restarts processes, and retrieves the
original results. It removes its temporary resources afterward.

For PostgreSQL integration tests, point `LINHA_TEST_DATABASE` at a disposable
PostgreSQL database. Tests create isolated schemas and remove those schemas.

```sh
LINHA_TEST_DATABASE='postgres://user:password@localhost/linha_test?sslmode=disable' \
  go test -race ./server/...
uv run --no-project --with openapi-spec-validator==0.7.2 \
  python -m openapi_spec_validator api/openapi.json
openspec validate --all --strict
```

Without the database variable, Go tests explicitly skip PostgreSQL scenarios.
The CI workflow supplies it. Cluster tests are separate; see
[the Kubernetes walkthrough](docs/kubernetes-validation.md).

See [contributing](CONTRIBUTING.md) for formatting, local configuration and commit hygiene.

## Write the business code

A typed job receives its engine plus Linha progress, cancellation and result
helpers. Return an ordinary value to let the SDK encode and persist JSON:

```scala
import linha._
import linha.json._
import linha.spark._
import linha.spark.syntax._

final case class RowsCount(value: Long)
final case class CountRows(size: Long) extends SparkEntrypoint[RowsCount] {
  def execute(ctx: LinhaJobContext[Spark]): RowsCount = {
    ctx.progress(0.1, "Counting rows")
    val count = ctx.spark.range(size).count()
    ctx.progress(1.0, "Finished")
    RowsCount(count)
  }
}
object CountRows {
  implicit val codec: EntrypointCodec[CountRows, RowsCount] =
    EntrypointCodec.derived("count-rows", 1)
}
```

Install that code in the worker image and register its codec. The complete
bootstrap is in `examples/jvm/src/main/scala/linha/example/SparkMain.scala`:

```scala
val worker = runtime.worker(
  sys.env("LINHA_SERVER_URL"),
  LinhaWorker.projectedIdentity(),
  Some(LinhaWorker.projectedCredentials())
).register(CountRows.codec)
worker.run()
```

The runtime creates one classic SparkContext per driver, a separate SparkSession
per request, FAIR scheduler pools and request job groups. The server and worker
enforce bounded slots. Cancelling one request does not stop the shared context.

For files, return `FileResult`; the configured context determines local versus
S3 storage, so the business return type does not need `S3Result[...]`:

```scala
def execute(ctx: LinhaJobContext[Spark]): FileResult =
  ctx.results.stream("summary.txt", "text/plain") { output =>
    output.write("finished\n".getBytes("UTF-8"))
  }
```

`ctx.results.file(name, contentType)(path => ...)` provides a seekable temporary
path instead. Only a successfully returned result is eligible for publication.

## Submit and retrieve

```scala
import linha.client._
import io.circe.Json
import io.circe.syntax._
import scala.concurrent.ExecutionContext.Implicits.global

val client = new LinhaClient(
  "https://linha.example",
  CredentialProvider(() => obtainFreshAccessToken())
)
val storage = ResultStorage.S3(
  "analytics",
  ResultPath.template("exports/{submittedAt:yyyy/MM/dd}/{requestId}/{attemptId}/{file}")
)
val spec = Json.obj(
  "image" -> "registry.example/my-spark-worker:1".asJson,
  "engine" -> Json.obj(
    "type" -> "spark".asJson,
    "version" -> "3.5.6".asJson,
    "settings" -> Json.obj(
      "application" -> Json.obj("mainClass" -> "example.WorkerMain".asJson,
        "mainApplicationFile" -> "local:///opt/linha/worker.jar".asJson),
      "ui" -> Json.obj("enabled" -> true.asJson),
      "drivers" -> Json.obj("minDrivers" -> 1.asJson, "maxDrivers" -> 2.asJson,
        "maxConcurrentRequestsPerDriver" -> 2.asJson),
      "executors" -> Json.obj("dynamicAllocation" -> true.asJson,
        "minExecutors" -> 0.asJson, "initialExecutors" -> 1.asJson,
        "maxExecutors" -> 4.asJson,
        "executorIdleTimeout" -> "60s".asJson,
        "cachedExecutorIdleTimeout" -> "120s".asJson,
        "shuffleTrackingTimeout" -> "120s".asJson)
    )
  ),
  "results" -> storage
)
val accepted = for {
  context <- client.ensureBackend("analytics", spec)
  job <- context.submit(CountRows(1000000), idempotencyKey = "business-request-123")
} yield job.id
```

Keep the client/context for your API process lifetime and close them on shutdown.
The SDK renews a lease; old configuration versions finish accepted jobs and stop
after their last client leaves. See [SparkApplication setup and migration](docs/versioned-spark-applications.md).

Return that ID to your API caller. Submission does not wait for the driver or for
Spark processing. Store/reuse the same idempotency key if your own API call is
retried. After a restart, build a fresh client and restore a handle from the ID:

```scala
val handle = client.job(savedJobId, CountRows.codec)
val state = handle.status()        // Future[Json]; poll at your chosen interval
val attempts = handle.attempts()  // Persisted execution history
val result = handle.result()      // Future[RowsCount], once status is SUCCEEDED
```

Call `handle.cancel()` for cancellation. `client.list(...)` lists only the
authenticated owner's jobs. File results contain descriptors; use
`handle.download(allocationId, destination)` to stream one file.

Local storage uses `ResultStorage.Local("/var/lib/linha/results")` instead.
The caller is responsible for durable shared mounts and permissions.

## Customize driver and executor Pods

Client-provided Pod templates support ConfigMap/Secret/PVC mounts, environment
variables, private registry pull Secrets, scheduling, init containers and sidecars.
Use `PodTemplate` and `SparkPodTemplates` from the JVM client, or send the native
JSON fragments directly. See [the template guide and examples](docs/pod-templates.md).

## Use the base JSON protocol directly

Typed submission sends only the handler identity and encoded parameters:

```json
{"handler":"count-rows","version":1,"payload":{"size":1000000},
 "idempotencyKey":"business-request-123","retry":{"maxAttempts":1,"backoffSeconds":1}}
```

POST this body to `/v1/contexts/{contextId}/jobs`. A manual worker registers a
matching raw handler and decides how the payload maps to business code:

```scala
worker.handleRaw(Capability("count-rows", 1,
  ResultDescriptor("json", "count-rows.result", 1))) { (payload, ctx) =>
  val size = payload.hcursor.get[Long]("size").fold(throw _, identity)
  JsonResult(Json.obj("value" -> ctx.engine.session.range(size).count().asJson))
}
```

The raw and typed methods share durable acceptance, leases, progress, cancellation
and result publication. Retries are disabled by default; opt in with
`maxAttempts` when external business side effects are idempotent.

## Deployment

Build the server image after `CGO_ENABLED=0 make build`; build the example worker
after preparing the pinned connectors and running `mvn package`:

```sh
scripts/prepare-spark-connectors.sh
docker build -f server/Dockerfile -t your-registry/linha-server:0.1.0 .
docker build -f examples/spark/Dockerfile -t your-registry/linha-worker:0.1.0 .
helm lint deploy/helm/linha --set security.enabled=false
```

Configure `database.postgres` with connection settings and `secretRef` / `usernameKey` /
`passwordKey`. Set `security.oidc.enabled`, issuer and audience, or disable all
authentication with `security.enabled: false`. Namespace-only RBAC is the default
(`rbac.namespaceOnly: true`); it creates no ClusterRole or ClusterRoleBinding.
Configure authorized image repositories and local/S3 destinations before installing
the chart. Anonymous clients use `CredentialProvider.none`. Read [operations](docs/operations.md)
for identity, mounts, retention, resource settings, upgrade constraints and known
limits. No production deployment or METOC migration has been performed.
