# Client and worker walkthrough

Linha transports JSON data plus a handler name/version. Put business code in the
worker image; the client and worker can share the same entrypoint case classes.
A client process owns its `LinhaClient` and contexts for its lifetime. Keep the
logical name stable across releases: configuration and image digests determine
immutable versions, and client leases let old/new replicas overlap. Closing the
last context (or lease expiry) drains accepted jobs before stopping its drivers.

## Write and register a typed Spark job

```scala
import linha._
import linha.json._
import linha.spark._
import linha.spark.syntax._

final case class CountResult(rows: Long)
final case class CountEvents(path: String) extends SparkEntrypoint[CountResult] {
  def execute(ctx: LinhaJobContext[Spark]): CountResult = {
    ctx.progress(0.1, "Reading events")
    val events = ctx.spark.read.parquet(path)
    val count = events.count()
    ctx.progress(1.0, "Complete")
    CountResult(count)
  }
}
object CountEvents {
  implicit val codec = EntrypointCodec.derived[CountEvents, CountResult]("count-events", 1)
}
```

The worker constructs one `SparkRuntime`, creates its worker with projected
identity/credentials, registers `CountEvents.codec`, and calls `run()`. Follow
[`SparkMain.scala`](../examples/jvm/src/main/scala/linha/example/SparkMain.scala)
for startup/shutdown and the `LINHA_SPARK_CONF` bootstrap. `ctx.spark` is a
request session; cancellation uses an attempt-specific Spark job group. Use
`ctx.cache(frame)` and `ctx.broadcast(value)` for request-owned cleanup.

Manual dispatch uses the same worker and lifecycle:

```scala
worker.handleRaw(Capability("raw-count", 1,
  ResultDescriptor("json", "raw-count.result", 1))) { (payload, ctx) =>
  val size = payload.hcursor.get[Long]("size").fold(throw _, identity)
  JsonResult(io.circe.Json.obj("value" -> io.circe.Json.fromLong(ctx.spark.range(size).count())))
}
```

Do not register the same handler/version twice. Typed registration implements
this raw dispatch by decoding arguments, calling `execute`, and encoding results.

## Ensure and submit from an API

The example below uses the built-in `count-rows` job from the example image.
Use a registry/image your Linha deployment authorizes. Build/push your own image
with the matching SDK and connectors using the
[dataset image instructions](dataset-results.md#s3-delegation-and-connectors).

```scala
import linha._
import linha.client._
import linha.example.{CountRows, RowsCount, SparkMain}
import io.circe.Json
import io.circe.parser.parse
import scala.concurrent.{Await, ExecutionContext}
import scala.concurrent.duration._
implicit val ec: ExecutionContext = ExecutionContext.global
implicit val countCodec: EntrypointCodec[CountRows, RowsCount] = SparkMain.countCodec

// For an authenticated API, supply a CredentialProvider that refreshes its token.
val client = new LinhaClient("http://linha:8080", CredentialProvider.none)
val spec = parse("""{
  "image": "registry.example/team/worker:release",
  "engine": {"type":"spark","version":"3.5.6","settings":{
    "application":{"mainClass":"linha.example.SparkMain",
      "mainApplicationFile":"local:///opt/linha/linha-examples.jar"},
    "drivers":{"minDrivers":0,"maxDrivers":2,"maxConcurrentRequestsPerDriver":2,
      "memory":"1g","memoryOverhead":"512m","cores":1},
    "executors":{"dynamicAllocation":true,"minExecutors":0,"initialExecutors":0,
      "maxExecutors":4,"memory":"2g","memoryOverhead":"512m","cores":1,
      "executorIdleTimeout":"60s","cachedExecutorIdleTimeout":"120s",
      "shuffleTrackingTimeout":"120s"},
    "ui":{"enabled":true,"port":4040}
  }},
  "results":{"type":"s3","destination":"analytics",
    "path":{"version":1,"template":"exports/{submittedAt:yyyy/MM/dd}/{requestId}/{attemptId}/{file}"}}
}""").fold(throw _, identity)

val context = Await.result(client.ensureBackend("metoc-extract", spec), 30.seconds)
val handle = Await.result(context.submit(CountRows(1000), idempotencyKey = "external-request-42"), 30.seconds)
val publicId = handle.id // Persist this in your API response; Spark need not be ready.
val state = Await.result(handle.status(), 10.seconds)
// Poll until SUCCEEDED, FAILED, or CANCELLED before fetching the result.
val result = Await.result(handle.result(), 30.seconds)
```

`result()` fetches an available result; it does not wait for execution. Failed
status includes exception type, stack trace and phase; `attempts()` preserves
previous failures. A stable idempotency key retries acceptance safely; a changed
payload with the same key is rejected. Execution retry is separate and opt-in
(`maxAttempts = 2`); external business side effects must be idempotent.

For local output replace only `results` with
`ResultStorage.local("/var/lib/linha/results")`. Mount the same durable filesystem
in every server, driver and executor. For S3, administrators define destination
`analytics` in the configuration Secret; callers choose its path template, not
credentials or an arbitrary bucket. Templates expand the original accepted UTC
time and keep a separate attempt segment across retries.

The equivalent raw request is:

```json
{"handler":"count-rows","version":1,"payload":{"size":1000,"delayMillis":0,"cacheAndBroadcast":false},"idempotencyKey":"external-request-42"}
```

Send it to `POST /v1/contexts/{id}/jobs` with authentication and
`X-Linha-Client-Lease`, or use `context.submitRaw(RawRequest(...))`. The raw HTTP
client renews its context lease; the Scala client does so automatically.

## Values, streams, files and datasets

A plain case-class result is JSON stored in the configured provider. Return a
`FileResult` for byte output; the SDK uploads only after the callback succeeds:

```scala
// From an entrypoint returning FileResult:
ctx.results.stream("summary.csv", "text/csv") { output =>
  output.write("rows\n1000\n".getBytes("UTF-8"))
}
// Or use a seekable path for a library that requires one:
ctx.results.file("report.bin", "application/octet-stream") { path =>
  java.nio.file.Files.write(path, Array[Byte](1, 2, 3))
}
```

From a `SparkEntrypoint[DatasetResult]`, return
`ctx.results.parquet(frame, "events", partitionBy = Seq("day"))`. Executors write
parts; the SDK registers immutable parts and seals a manifest. Successful
completion publishes the result atomically. See [dataset retrieval and limits](dataset-results.md)
for incremental part enumeration, streaming download, STS and local mount setup.

## Retrieve after restarts or driver replacement

Construct a fresh client with the same authenticated principal, then use
`client.job(savedPublicId, countCodec)` and `status()` / `result()`. Retrieval
requires neither a context attachment nor a running worker. For files/datasets,
use `resultMetadata()`, `parts(cursor, limit)`, `download(allocationId, path)` and
`downloadPart(partPath, path)`. Each download is freshly authorized through the
API; no expiring S3 link is part of the permanent identity.

The server replaces lost required drivers. A lost running attempt fails or
retries according to its accepted retry policy; completed results remain at their
recorded locations. Close contexts and the client when the API process shuts
down; accepted jobs finish while the unused version drains.

## Reproduce development acceptance

`make check`, Java 17 `mvn -B install`, and `scripts/e2e.py` exercise the raw/typed
baseline. The platform scripts exercise datasets, real outages and the disposable
Spark Operator cluster. See [validation evidence](validation.md) and the
[acceptance matrix](acceptance.md) for prerequisites, measured limits and scope.
