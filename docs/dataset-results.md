# Return files and Spark datasets

The context selects local or S3 storage. Business code returns JSON, `FileResult`
or `DatasetResult`; the SDK allocates output and publishes it after successful
completion. The return type does not contain a bucket or storage-specific marker.

This guide assumes a registered typed Spark entrypoint and a result destination.
See [the client/worker tutorial](walkthrough.md) for that setup.

## Return a stream or file

From an entrypoint returning `FileResult`:

```scala
ctx.results.stream("summary.csv", "text/csv") { output =>
  output.write("rows\n1000\n".getBytes("UTF-8"))
}
```

Use a temporary seekable path when a library needs one:

```scala
ctx.results.file("report.bin", "application/octet-stream") { path =>
  java.nio.file.Files.write(path, Array[Byte](1, 2, 3))
}
```

A failing callback does not publish its output. Stream helpers bound bytes while
writing; seekable callbacks are checked after returning. Use an ephemeral-storage
quota when third-party writes need a hard disk bound.

## Return distributed Parquet

```scala
import linha._
import linha.json._
import linha.spark._
import linha.spark.syntax._

final case class ExportRows(size: Long) extends SparkEntrypoint[DatasetResult] {
  def execute(ctx: LinhaJobContext[Spark]): DatasetResult = {
    val frame = ctx.spark.range(size).selectExpr("id", "id % 2 AS group")
    ctx.results.parquet(frame, "events", partitionBy = Seq("group"))
  }
}
object ExportRows {
  implicit val codec = EntrypointCodec.derived[ExportRows, DatasetResult]("export-rows", 1)
}
```

Executors write parts without collecting the DataFrame to the driver. The helper
inventories and hashes each committed part. Publication verifies and copies the
parts into immutable allocations. This requires extra I/O and storage but protects
retained results from late writers. Registration or sealing alone does not publish.

## Configure local storage

Mount the same durable filesystem at the configured result root in every server,
driver and executor. Use group-writable directories compatible with group 10001.
A server-created marker is checked by Spark tasks; a missing or different mount
fails the write instead of producing inaccessible worker-local data.

```scala
val storage = linha.client.ResultStorage.Local("/var/lib/linha/results")
```

Linha does not provision the filesystem. See [deployment](operations.md).

## Configure S3 dataset delegation

JSON and ordinary files use server-authorized uploads. Distributed S3 datasets
also require STS AssumeRole with enforced session policies:

```json
{
  "analytics": {
    "bucket": "example-results",
    "prefix": "linha",
    "region": "eu-west-1",
    "datasetRoleArn": "arn:aws:iam::123456789012:role/linha-datasets"
  }
}
```

The server's AWS credentials must be allowed to assume that role. Its base policy
allows the configured result prefix; the delegated session policy narrows writes
to one staging allocation. For compatible stores, add `endpoint`, `stsEndpoint`
and `pathStyle`. A provider without the necessary STS support returns
`DATASET_UNSUPPORTED`, while file output remains available.

Grants last at most one hour, with no transparent renewal during a write.
Credentials stay in per-write Hadoop configuration, with filesystem caching
disabled. The helper uses S3A magic committers. Include matching connector JARs
in the worker image:

```sh
scripts/prepare-spark-connectors.sh
mvn -B package
docker build -f examples/spark/Dockerfile -t registry.example/workers/example:1 .
```

The pinned example uses Hadoop AWS 3.3.4, AWS SDK bundle 1.12.262 and Spark Hadoop
Cloud 3.5.6. See [compatibility](compatibility.md),
[Hadoop committers](https://hadoop.apache.org/docs/r3.3.4/hadoop-aws/tools/hadoop-aws/committers.html)
and [STS session policies](https://docs.aws.amazon.com/STS/latest/APIReference/API_AssumeRole.html).

## Retrieve retained output

```scala
val handle = client.job(savedJobId, ExportRows.codec)
val result = Await.result(handle.result(), 30.seconds)
val manifest = result.dataset.get
val page = Await.result(handle.parts(limit = 100), 30.seconds)
Await.result(handle.download(manifest.manifest.allocationId, manifestFile), 30.seconds)
Await.result(handle.downloadPart("group=0/part-....parquet", partFile), 30.seconds)
```

This continues the client setup from [the tutorial](walkthrough.md).
Use the returned `nextCursor` to enumerate further pages. Use part paths verbatim,
rather than constructing S3 keys. All downloads pass through Linha and authorize
the current owner anew; the job ID is not an expiring S3 URL.

## Bound storage and enable cleanup

Default result size is 64 MiB, with a 5 GiB maximum. The dataset cap is 100,000
parts; SDK registration batches contain at most 100 parts. Manifest bytes are
also bounded. Server and worker staging reservations default to 5 GiB.

```yaml
results:
  maxStagingBytes: 5368709120
  cleanup:
    enabled: true
```

Physical cleanup is disabled by default. When enabled it waits for terminal
attempts, delegated-write expiry and ten minutes of grace, and removes only
tracked allocations that are not protected by retained results. It aborts
eligible abandoned multipart uploads under those prefixes. Result expiry applies
even when physical cleanup is disabled. Job IDs, receipts and idempotency records
remain as metadata. External lifecycle rules must respect Linha's retention period.
