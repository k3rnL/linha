# Distributed dataset results

Upgrade all server replicas to schema v6 before deploying SDKs that use datasets.
Job IDs and existing receipts are preserved. Old schema-v5 servers do not run
against v6; rollback requires a compatible build or restoring a coordinated backup.

The same Scala entrypoint works with local or S3 result policies:

```scala
import linha._
import linha.json._
import linha.spark._
import linha.spark.syntax._

final case class ExportRows(size: Long) extends SparkEntrypoint[DatasetResult] {
  def execute(ctx: LinhaJobContext[Spark]): DatasetResult = {
    val events = ctx.spark.range(size).selectExpr("id", "id % 2 AS group")
    ctx.results.parquet(events, "events", partitionBy = Seq("group"))
  }
}
object ExportRows {
  implicit val codec = EntrypointCodec.derived[ExportRows, DatasetResult]("export-rows", 1)
}
```

Register the codec in the worker. Parquet bytes are produced by distributed Spark
tasks; they are never collected into a driver DataFrame. The helper inventories
and hashes committed parts one at a time. It supports at most 100,000 parts and
bounds registration batches to 100 parts. Hashing reads each part on the driver;
publication verifies it again and creates an immutable copy owned by the server.
This costs additional I/O and storage; it is deliberate protection against late
writers. S3 snapshots use conditional uploads through one bounded temporary file at a time;
the API staging budget also covers these copies.

Only `complete` publishes a successful job. Part registration, a successful Spark
write, or sealing a manifest alone does not expose the dataset through the public
API. Retries have separate attempt namespaces. The manifest is JSON lines stored
in the selected provider; PostgreSQL contains its reference and a paginated part
index, not the Parquet payload.

## Client retrieval

```scala
val handle = client.job(savedJobId, ExportRows.codec)
val result = Await.result(handle.result(), 30.seconds)
val info = result.dataset.get
println(info.partCount)
val firstPage = Await.result(handle.parts(limit = 100), 30.seconds)
// Pass nextCursor to parts() until it is absent.
Await.result(handle.downloadPart("group=0/part-....parquet", destination), 30.seconds)
Await.result(handle.download(info.manifest.allocationId, manifestFile), 30.seconds)
```

Each download authenticates anew. Linha streams bytes through its API and does
not issue expiring public S3 URLs; a retained job ID remains sufficient to request
fresh access after credentials refresh, worker replacement or server restart.
Use the returned part paths verbatim. Status and results stay owner-scoped.
Missing storage returns an explicit error while retaining SUCCEEDED metadata.

## Shared local storage

Mount the same durable filesystem at the configured root in all server replicas,
drivers and executors. The root must be writable with group 10001 (Linha's server
and configured worker fsGroup); use a setgid group-writable directory. Linha does
not provision this storage. A server-created marker is checked by Spark tasks
before writing; missing or different mounts fail instead of silently publishing
worker-local data. Files and directories remain inside the allocated attempt path.

## S3 delegation and connectors

Ordinary JSON/file results keep using server-authorized uploads. Distributed
S3 output additionally requires an STS AssumeRole service that enforces session
policies. Add these destination fields to the existing S3 configuration Secret:

```json
{
  "analytics": {
    "bucket": "results",
    "prefix": "linha",
    "region": "eu-west-1",
    "datasetRoleArn": "arn:aws:iam::123456789012:role/linha-datasets"
  }
}
```

For a compatible service such as MinIO, also supply `endpoint`, `stsEndpoint` and
`pathStyle: true`. The Linha server's configured AWS credential chain must be
allowed to assume the role; its base policy must allow S3 operations under the
configured prefix. The session policy narrows write/read/list/abort privileges to
one allocation's staging prefix. It cannot write published copies or other
allocations. No broad bucket credentials are returned to workers. Providers
without this STS capability return DATASET_UNSUPPORTED; file results still work.

Delegations last at most one hour. A write exceeding credential validity fails
explicitly and follows the job's retry policy; transparent mid-write credential
renewal is not offered. Credentials live only in per-write Hadoop configuration;
filesystem caching is disabled to avoid reuse across requests. The Spark helper
uses the S3A magic committer and request-session commit settings, not a global
credential change. The result image must contain matching connector versions:

```sh
JAVA_HOME=/path/to/jdk17 scripts/prepare-spark-connectors.sh
mvn -B package
docker build -f examples/spark/Dockerfile -t linha-worker:example .
```

The example pins Spark 3.5.6, Scala 2.12.20, Hadoop AWS 3.3.4, AWS SDK bundle
1.12.262 and Spark Hadoop Cloud 3.5.6. Keep these aligned with Spark's Hadoop jars.
See the [Hadoop S3A committers](https://hadoop.apache.org/docs/r3.3.4/hadoop-aws/tools/hadoop-aws/committers.html)
and [STS session policies](https://docs.aws.amazon.com/STS/latest/APIReference/API_AssumeRole.html).

## Limits and cleanup

`maxBytes` bounds total dataset bytes; manifest bytes are separately subject to the
same limit. Default result size is 64 MiB, maximum 5 GiB. Concurrent server and
worker staging reservations default to 5 GiB and return STAGING_LIMIT_EXCEEDED
when exhausted. Configure `LINHA_MAX_STAGING_BYTES` (server Helm:
`results.maxStagingBytes`). Stream helpers enforce byte limits during writing;
seekable callbacks are checked after returning. For hard bounds on arbitrary
third-party seekable writes, also use a filesystem/container ephemeral-storage
quota. Linha cannot intercept all writes made through a raw filesystem path.

Enable cleanup explicitly in Helm:

```yaml
results:
  cleanup:
    enabled: true
  maxStagingBytes: 5368709120
```

The standalone equivalent is `LINHA_CLEANUP_ENABLED=true`. Cleanup waits for a
terminal attempt, delegated-write expiry and ten minutes of grace, then removes
only persisted allocations not referenced by a retained result. It aborts pending
S3 multipart uploads under those prefixes. It never scans unrelated buckets or
roots. Repeated sweeps tolerate failure and remove recreated obsolete data.
Result expiry is still enforced when cleanup is disabled. Job metadata and
completion receipts are retained indefinitely as tombstones; there is no
automatic job-record deletion. External lifecycle rules must not delete results
before their Linha retention period.
