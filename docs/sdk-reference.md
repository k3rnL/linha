# Scala/JVM SDK reference

All Scala artifacts use binary version 2.12 and Java 17. This guide uses SDK
version `0.1.3-SNAPSHOT`.

<!-- development-only:start -->
Build from source to use these development coordinates, or select the published
`0.1.3` version from Maven Central.
<!-- development-only:end -->

| Maven artifact | Responsibility |
| --- | --- |
| `com.k3rnl:linha-protocol_2.12` | Wire contracts, codecs, credentials and result paths |
| `com.k3rnl:linha-client_2.12` | Client, contexts, job handles and OAuth acquisition |
| `com.k3rnl:linha-worker_2.12` | Handler runtime, leases, progress and publication |
| `com.k3rnl:linha-hadoop_2.12` | Scoped application filesystem with range reads |
| `com.k3rnl:linha-spark_2.12` | SparkContext sessions and distributed result helpers |

## Dependency declarations

For an SBT application that includes both client and worker examples:

```scala
scalaVersion := "2.12.20"
val linhaVersion = "0.1.3-SNAPSHOT"
libraryDependencies ++= Seq(
  "com.k3rnl" %% "linha-client" % linhaVersion,
  "com.k3rnl" %% "linha-spark" % linhaVersion,
  "org.apache.spark" %% "spark-sql" % "3.5.6" % Provided
)
```

The Spark module brings the worker/protocol SDKs transitively. A client-only
application needs only `linha-client`; a shared entrypoint module also needs the
Spark types used by its code. Spark is provided at runtime by the worker image.
For Maven, use the explicit artifact names in the table, with group `com.k3rnl`.

## Client and context

| Method | Result / purpose |
| --- | --- |
| `new LinhaClient(url, credentials)` | Process-owned client; requires an ExecutionContext |
| `ensureBackend(name, spec)` | `Future[LinhaContext]`; attach to the derived configuration version |
| `restoreContext(id)` | `Future[LinhaContext]`; attach to an existing version |
| `job(id, codec)` | Typed handle; independent of engine lifetime |
| `rawJob(id)` | Untyped handle |
| `list(contextId, state, cursor, limit)` | Owner-scoped paginated jobs; limit defaults to 50 |
| `close()` | Release attachments and stop client lease scheduling |
| `context.submit(entrypoint, idempotencyKey, maxAttempts)` | Typed handle; defaults: generated key, one attempt |
| `context.submitRaw(RawRequest(...), idempotencyKey, maxAttempts)` | Same protocol with raw JSON parameters |
| `context.storage(destination)` | Live client session for an enabled application destination |
| `context.status()` | Current version state |
| `context.close()` | Release this context handle; shared handles retain their lease |

Submission futures finish after durable acceptance, not job execution.

## Job handles

| Method | Purpose |
| --- | --- |
| `status()` | Job state, progress and failure |
| `attempts()` | Persisted execution history and diagnostics |
| `cancel()` | Commit cancellation intent |
| `resultMetadata()` | Retained result descriptor and file/dataset metadata |
| `result()` | Typed result; available after success, schema checked before decode |
| `parts(cursor, limit)` | Dataset part page; defaults to 100 |
| `download(allocationId, destination)` | Stream a retained file through the API |
| `downloadPart(partPath, destination)` | Stream a retained dataset part |

`result()` does not poll until success. Download destinations must not already
exist because the implementation uses `Files.copy` without replacement.

## OAuth credentials

`CredentialProviders.oauthClientCredentials(tokenEndpoint, clientId,
clientSecret, scopes, options, clock)` returns a `CredentialProvider`.

| Argument / option | Default | Contract |
| --- | --- | --- |
| `tokenEndpoint` | required | Absolute HTTPS token endpoint; no embedded credentials or fragment |
| `clientId` | required | OAuth service client identifier |
| `clientSecret` | required | `() => String`, evaluated on acquisition |
| `scopes` | empty sequence | Optional OAuth scopes |
| `caFile` | none | PEM trust file for this identity HTTP client |
| `insecureSkipVerify` | false | Per-client certificate and hostname bypass |
| `allowHttp` | false | Permit HTTP in an isolated fixture |
| `connectTimeout` | 5 seconds | Connection deadline |
| `requestTimeout` | 10 seconds | Whole response deadline, including body |
| `maxResponseBytes` | 1 MiB | Bounded provider response |
| `clock` | UTC system clock | Expiry calculation; injectable for tests |

Token responses require `token_type: Bearer` and positive `expires_in`.
Redirects are rejected. Response bodies and credentials are not included in
`CredentialException`; the optional HTTP status is retained.

`CredentialProvider.fileSecret(path)` supplies a mounted **client secret** and
rereads it on acquisition. `CredentialProvider.none` is for explicitly anonymous
installations. A custom `CredentialProvider` can integrate application-owned token
acquisition; OAuth client credentials is the bundled acquisition flow.

## Entrypoints and worker context

`LinhaEntrypoint[R, E]` exposes `execute(LinhaJobContext[E]): R`.
`SparkEntrypoint[R]` specializes it for the Spark engine. Register an
`EntrypointCodec.derived[J, R](handler, version)` in the worker image.

| Context member | Purpose |
| --- | --- |
| `jobId`, `attemptId` | Durable job identity and current attempt |
| `engine` / Spark `spark` extension | Engine object / request SparkSession |
| `progress(fraction, message)` | Persist progress in the current attempt |
| `isCancelled`, `checkCancelled()` | Cooperative cancellation |
| `results` | Storage-policy result writer |
| `onCleanup(action)` | Request-scoped cleanup callback |
| Spark `cache(frame)` | Reference-counted request-owned cache |
| Spark `broadcast(value)` | Broadcast destroyed with request cleanup |

Worker bootstrap uses `LinhaWorker.projectedIdentity()` and
`projectedCredentials()` for managed instance authentication. These internal
worker credentials are separate from the application's OAuth credentials.

## Result types and paths

| Type/helper | Purpose |
| --- | --- |
| Ordinary case class | Encoded JSON result |
| `FileResult` | Registered immutable file descriptors |
| `DatasetResult` | Distributed parts and manifest |
| `results.stream(name, contentType)(write)` | Bounded byte stream |
| `results.file(name, contentType)(write)` | Temporary seekable Path |
| Spark `results.parquet(frame, name, partitionBy)` | Distributed Parquet |
| `ResultStorage.Local(root)` | Local backend result policy |
| `ResultStorage.S3(destination, path)` | S3 alias and declarative ResultPath |
| `ResultPath.template(text)` | Validated version-1 path template |

See [client/worker tutorial](walkthrough.md),
[result guide](dataset-results.md) and [path reference](configuration.md#result-path-templates)
for examples.

Application file access and `Unit`/`StatusResult` completion are described in [Application storage](application-storage.md).
