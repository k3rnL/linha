# Build a Scala client and worker

This tutorial creates a Spark job, registers it in a worker, submits it through
the client SDK, and restores its result using the saved job ID.

## Before you start

Use Java 17, Scala 2.12 and the matching `com.k3rnl` client, worker and Spark
artifacts listed in [the SDK reference](sdk-reference.md).

<!-- development-only:start -->
Build the development SDKs with Java 17 `mvn -B install`.
<!-- development-only:end -->
 You need a Linha deployment,
Kubeflow Spark Operator and a worker registry that the deployment permits. Configure an S3 result destination named `analytics` and working server AWS
credentials. Set up [OAuth credentials](client-authentication.md) for your application first.

The example uses a JSON result. [File and dataset output](dataset-results.md)
uses the same submission flow.

## 1. Define the shared entrypoint

Put this code in a module shared by your client and worker:

```scala
package example

import linha._
import linha.json._
import linha.spark._
import linha.spark.syntax._

final case class RowsCount(value: Long)
final case class CountRows(size: Long) extends SparkEntrypoint[RowsCount] {
  def execute(ctx: LinhaJobContext[Spark]): RowsCount = {
    require(size >= 0)
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

The codec gives this business operation a stable handler name and wire version.
The worker executes the class from its image; no class or closure is sent by the
client. Compile this module before continuing.

## 2. Register the worker

```scala
package example

import linha.spark.SparkRuntime
import linha.worker.LinhaWorker
import org.apache.spark.SparkConf

object WorkerMain {
  def main(args: Array[String]): Unit = {
    val runtime = new SparkRuntime(new SparkConf(), sys.env("LINHA_CAPACITY").toInt)
    val worker = runtime.worker(
      sys.env("LINHA_SERVER_URL"),
      LinhaWorker.projectedIdentity(),
      Some(LinhaWorker.projectedCredentials())
    ).register(CountRows.codec)
    sys.addShutdownHook { worker.close(); runtime.close() }
    try worker.run()
    finally { worker.close(); runtime.close() }
  }
}
```

Package this main class and its dependencies into your worker JAR. The image must
contain Spark 3.5.6 with Scala 2.12 and retain Spark's native container entrypoint.
Place the JAR at `/opt/linha/worker.jar`, or update the application path below.
The repository's [example worker](https://github.com/k3rnL/linha/tree/main/examples)
provides a runnable assembly and Dockerfile.

Push your image and verify that its repository is in Linha's `allowedImages`.
Use compatible dependency shading: see [compatibility](compatibility.md).

## 3. Create the application client

```scala
import linha.CredentialProvider
import linha.client.LinhaClient
import linha.client.auth.CredentialProviders
import java.nio.file.Paths
import scala.concurrent.ExecutionContext.Implicits.global

val credentials = CredentialProviders.oauthClientCredentials(
  "https://identity.example/realms/jobs/protocol/openid-connect/token",
  "reporting-api",
  CredentialProvider.fileSecret(Paths.get("/credentials/client-secret"))
)
val client = new LinhaClient("https://linha.example", credentials)
```

Keep this client for the application's process lifetime. The mounted file contains
an OAuth client secret; the SDK acquires the access token.

## 4. Ensure a context and submit

The example below selects a few settings. The [backend reference](reference/backend/index.md)
lists the complete model; [Spark settings](reference/backend/spark.md) lists every
application, driver, executor and UI option with defaults and constraints.

```scala
import example.{CountRows, RowsCount}
import io.circe.parser.parse
import scala.concurrent.Await
import scala.concurrent.duration._

val spec = parse("""{
  "image": "registry.example/workers/reporting:1",
  "engine": {"type":"spark","version":"3.5.6","settings":{
    "application":{"mainClass":"example.WorkerMain",
      "mainApplicationFile":"local:///opt/linha/worker.jar"},
    "drivers":{"minDrivers":1,"maxDrivers":2,
      "maxConcurrentRequestsPerDriver":2,
      "memory":"1g","memoryOverhead":"512m","cores":1},
    "executors":{"dynamicAllocation":true,"minExecutors":0,
      "initialExecutors":1,"maxExecutors":4,
      "memory":"2g","memoryOverhead":"512m","cores":1,
      "executorIdleTimeout":"60s","cachedExecutorIdleTimeout":"120s",
      "shuffleTrackingTimeout":"120s"}
  }},
  "results":{"type":"s3","destination":"analytics"}
}""").fold(throw _, identity)

val context = Await.result(client.ensureBackend("reporting", spec), 30.seconds)
val handle = Await.result(
  context.submit(CountRows(1000), idempotencyKey = "request-42"),
  30.seconds
)
println(handle.id)
```

Return `handle.id` to your API caller. The ID is durable; Spark processing may
start later. The ensure operation can resolve an image and validate settings,
whereas job acceptance waits only for its database transaction.

Keep the context while this process serves requests. Reuse the same name after
configuration changes; Linha derives a version. Use a fresh business idempotency
key for each new request, and reuse it when retrying the same acceptance.

## 5. Poll and retrieve

```scala
val state = Await.result(handle.status(), 10.seconds)
println(state)
```

Poll at your chosen interval until the state is `SUCCEEDED`, `FAILED` or
`CANCELLED`. After success:

```scala
val result: RowsCount = Await.result(handle.result(), 30.seconds)
println(result.value) // 1000
```

`result()` retrieves an available result; it does not wait for execution.
Use `handle.attempts()` to inspect persisted failures and `handle.cancel()` to
request cancellation.

## 6. Restore the handle later

Create a new client with the same application identity and restore by ID:

```scala
val restored = client.job(savedJobId, CountRows.codec)
val retained = Await.result(restored.result(), 30.seconds)
```

This operation needs no active context or running driver. On application shutdown,
close the context and client after stopping acceptance of new application requests:

```scala
context.close()
client.close()
```

Accepted work drains before Linha stops an unused context version.
For the equivalent raw JSON route, see [HTTP API](http-api.md).
