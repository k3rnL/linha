# Access application files through Linha

Application storage is for input files, shared datasets and temporary processing files. It is separate from a job's published result. Linha result expiration never removes application files.

## Enable a destination

An operator configures a named S3 destination on the server with `allowWorkloadAccess: true`:

```json
{
  "application-data": {
    "bucket": "application-bucket",
    "prefix": "processing",
    "region": "eu-west-1",
    "endpoint": "http://object-storage.storage.svc:9000",
    "pathStyle": true,
    "allowWorkloadAccess": true
  }
}
```

The server obtains S3 credentials through its normal AWS credential chain. Give it read, list, put, copy and delete permissions on the configured prefix. Application access is disabled unless explicitly enabled. Use a dedicated destination/prefix for each trust boundary: an authorized context requesting a destination can access that entire prefix, including shared application files written by other contexts using it.

The client's backend configuration requests access by name:

```json
"storage": { "destinations": ["application-data"] }
```

Linha snapshots the physical destination fingerprint into the context version. Moving the destination to another bucket, prefix or endpoint prevents old sessions from accessing the new location.

## Upload an input from the client

```scala
val context = Await.result(client.ensureBackend("processing", backend), Duration.Inf)
val storage = Await.result(context.storage("application-data"), Duration.Inf)
try storage.upload("inputs/source.nc", Paths.get("source.nc"))
finally storage.close()
```

A client session depends on the context's live client attachment. Reacquire a session if that attachment expires; a session token alone cannot revive it. Session tokens are scoped bearer credentials. Keep them out of durable job payloads, logs and public URLs. Send the destination name and application paths in the request instead.

## Read and write in a worker

```scala
val storage = ctx.storage("application-data")
val input = storage.open("inputs/source.nc")
try process(input)
finally input.close()
```

A worker session is bound to the active job attempt, worker incarnation and fence. The server checks its current database lease on each operation. Completion, cancellation, worker replacement and lease expiry deny subsequent access. PostgreSQL stores token digests, allowing another server replica to serve the same session after a restart.

For Spark or another Hadoop client, add `com.k3rnl:linha-hadoop_2.12` and propagate the session's Hadoop options before creating a dataset or RDD:

```scala
storage.hadoopOptions.foreach { case (key, value) =>
  spark.sparkContext.hadoopConfiguration.set(key, value)
}
val path = storage.path("datasets/output") // linha://<session-id>/datasets/output
frame.write.parquet(path)
```

Executors use the same scoped session through the Linha HTTP gateway. They receive no S3 access keys and need no application Secret, ConfigMap or shared PVC. Credentials use session-specific option names, so concurrent sessions remain separate. Redact token configuration keys in Spark logs and UI. Only delegate sessions to trusted executors.

The filesystem supports seekable range reads, create/overwrite, listing, directories, rename and delete. Append is unsupported. Rename uses S3 copy/delete and is not atomic. Directory operations are limited to 10,000 objects, uploads to 5 GiB per object. Hadoop writes stage on the worker's temporary disk; server uploads reserve space against `maxStagingBytes`. Size both disks and server bandwidth for the expected file sizes and concurrency. Storage traffic passes through Linha rather than directly between executors and S3.

## Serialize shared metadata updates

```scala
val lock = storage.lock("datasets/example/index")
try updateSharedIndex(storage)
finally lock.close()
```

Locks are 30-second PostgreSQL leases renewed by the SDK. All sessions addressing the same physical destination and directory contend on the same key. SDK writes below the locked directory include a lock proof; Linha verifies it and holds the lock row during publication. A writer that loses the lock cannot begin another protected write.

Locks do not make arbitrary S3 side effects exactly once. A request already executing against S3 when cancellation arrives may finish. Keep output operations repeatable and choose retry/replay policies appropriate for your application.

## Return status without an artifact

A Scala entrypoint returning `Unit` registers a `status` result descriptor. On success the worker commits an empty file list; no allocation or result upload occurs. Raw handlers return `StatusResult` with a matching descriptor. Clients can still retrieve the durable job state, progress and failure, and typed `result()` returns `Unit` after success.

Application owners manage retention of staged inputs, scratch and datasets, for example through S3 lifecycle policies. Keep inputs if administrators must replay requests later, and clean per-attempt scratch before completion while its storage session remains valid.
