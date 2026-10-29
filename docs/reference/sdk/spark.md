# Scala Spark runtime

Package `linha.spark`, artifact `com.k3rnl:linha-spark_2.12`. Spark 3.5.6/Scala 2.12
is provided by the worker image. See [backend Spark settings](../backend/spark.md)
for every deployment/resource option.

## SparkRuntime {#sparkruntime}

`new SparkRuntime(conf: SparkConf, capacity: Int)` requires positive capacity.
It creates one shared SparkContext with FAIR scheduling and bounded scheduler
pool slots. Backend `drivers.maxConcurrentRequestsPerDriver` supplies the managed
worker capacity; use `sys.env("LINHA_CAPACITY").toInt` rather than a different
hard-coded slot count.

| Member | Return type | Contract |
| --- | --- | --- |
| `open(jobId: String)` | `Spark` | Takes a slot, opens a separate SparkSession, sets attempt job group and scheduler pool on the execution thread |
| `release(value: Spark)` | `Unit` | Clears thread-local group/session properties and returns the slot |
| `cancel(jobId: String)` | `Unit` | Cancels only that engine job group |
| `worker(baseUrl: String, credential: WorkerCredential, credentials: Option[CredentialProvider] = None)` | `LinhaWorker[Spark]` | Wires engine open/cancel/release and capacity automatically |
| `close()` | `Unit` | Stops the shared SparkSession/SparkContext |

When used through `worker`, the key passed as `jobId` to open/cancel is the unique
attempt ID. `Spark(session: SparkSession, pool: String)` exposes `session`;
the pool member is package-scoped. `SparkEntrypoint[R]` extends
`LinhaEntrypoint[R,Spark]` with the same execute contract.

## Spark syntax {#spark-syntax}

Import `linha.spark.syntax._` for these extensions:

| Extension | Return type | Contract |
| --- | --- | --- |
| `ctx.spark` | `SparkSession` | Request session from `LinhaJobContext[Spark]` |
| `ctx.cache(dataframe: DataFrame)` | `DataFrame` | Reference-counted request-owned cache; released during cleanup, leaves pre-existing user cache alone |
| `ctx.broadcast[T: ClassTag](value: T)` | `Broadcast[T]` | Broadcast destroyed during request cleanup |
| `ctx.results.parquet(dataframe: DataFrame, name: String, partitionBy: Seq[String] = Seq.empty)` | `DatasetResult` | Distributed output with immutable published parts; partition names must be distinct existing columns |

The public implicit extension classes are `syntax.SparkJobContext` and
`syntax.SparkResultWriter`. Parquet writes use error-if-exists mode. Local writes
probe shared mounts on executors; S3 writes require allocation-scoped STS grants
and matching Hadoop/S3A magic-committer connectors. The helper inventories and
hashes parts incrementally without collecting the DataFrame on the driver.
At most 100,000 parts are accepted. See [dataset prerequisites](../../dataset-results.md).

## Worker main and image contract {#worker-image}

The worker image needs the declared `application.mainClass` and an image-local
JAR at `application.mainApplicationFile`. Keep Spark's native container entrypoint.
Bootstrap with `new SparkConf()` to inherit submitted settings; do not create a
second independent SparkContext per request.

```scala
import linha.spark.SparkRuntime
import linha.worker.LinhaWorker
import org.apache.spark.SparkConf

object WorkerBootstrap {
  def runtime(): (SparkRuntime, linha.worker.LinhaWorker[linha.spark.Spark]) = {
    val spark = new SparkRuntime(new SparkConf(), sys.env("LINHA_CAPACITY").toInt)
    val worker = spark.worker(
      sys.env("LINHA_SERVER_URL"),
      LinhaWorker.projectedIdentity(),
      Some(LinhaWorker.projectedCredentials())
    )
    (spark, worker) // Register business codecs, run, then close worker and spark.
  }
}
```

Build/shade the application with matching SDK/Spark dependencies and the required
connectors. The [client/worker tutorial](../../walkthrough.md) gives a registered
entrypoint and runnable main sequence; [compatibility](../../compatibility.md)
records the supported image/toolchain choices.
