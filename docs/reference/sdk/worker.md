# Scala entrypoints and worker runtime

Packages `linha` and `linha.worker`. See [Spark-specific APIs](spark.md) for
engine capabilities. Typed and raw handlers use the same JSON protocol.

## LinhaEntrypoint and EntrypointCodec {#entrypointcodec}

`trait LinhaEntrypoint[R,E]` defines `execute(context: LinhaJobContext[E]): R`.
Code is compiled into the worker image; only parameters cross the API.

`EntrypointCodec.derived[J: Encoder: Decoder, R: Encoder: Decoder: ClassTag](id: String, version: Int)`
creates `EntrypointCodec[J,R]`. Import `linha.json._` for Circe case-class
derivation. The generated descriptor uses schema `id + ".result"` and the same
version; return type chooses `status` for Unit, `file` for FileResult, `dataset`
for DatasetResult, and `json` otherwise.

The public case-class constructor also accepts `id`, `version`, `descriptor`,
`requestEncoder: Encoder[J]`, `requestDecoder: Decoder[J]`,
`resultEncoder: Encoder[R]` and `resultDecoder: Decoder[R]` explicitly.
`capability: Capability` returns its registration contract;
`encodeResult(value: R): ProducedResult` selects the runtime publication form.

## LinhaJobContext[E] {#linhajobcontext}

| Member | Return type | Meaning |
| --- | --- | --- |
| `jobId`, `attemptId` | `String` | Durable request and current execution attempt |
| `engine` | `E` | Request-scoped engine capability |
| `progress(fraction: Double, message: String)` | `Unit` | Fenced persistence; finite 0–1 fraction, message at most 4096 bytes |
| `isCancelled` | `Boolean` | Cancellation flag or local lease deadline exceeded |
| `checkCancelled()` | `Unit` | Throws InterruptedException when execution authority is lost |
| `storage(destination: String)` | `StorageSession` | Current-attempt application session; worker closes it during cleanup |
| `results` | `ResultWriter` | Writer governed by the accepted result policy |
| `onCleanup(action: () => Unit)` | `Unit` | Runs callbacks in reverse order after the attempt, before releasing the engine |

Handlers should cooperate with cancellation; interrupting a thread cannot undo
external side effects. Stale attempts cannot publish a replacement result.
The base trait's storage/dataset defaults can throw UnsupportedOperationException;
LinhaWorker provides these implementations when the server supports them.

## ResultWriter and produced values {#resultwriter}

| Signature | Return type | Contract |
| --- | --- | --- |
| `file(name: String, contentType: String)(write: Path => Unit)` | `FileResult` | Temporary seekable output, uploaded after write; bounded staging |
| `stream(name: String, contentType: String)(write: OutputStream => Unit)` | `FileResult` | Stages a bounded stream, then publishes via the same output allocation protocol |
| `dataset(name: String, format: String)(write: DatasetLocation => Iterator[DatasetPart])` | `DatasetResult` | Distributed write, part registration and sealing before successful completion; supported format is parquet |

Names follow [relative path rules](../backend/results.md#path-placeholders).
Worker/server staging reservation budgets default to 5 GiB; context maxBytes can
be smaller. JSON results use `result.json`, and status-only jobs allocate no file.

| Public type / constructor | Meaning |
| --- | --- |
| `RawRequest(handler: String, version: Int, payload: Json)` | Raw submission data |
| `Capability(handler: String, version: Int, result: ResultDescriptor)` | Worker handler contract |
| `ResultDescriptor(kind: String, schema: String, version: Int)` | [Result identity](../results.md#resultdescriptor) |
| `ResultFile(allocationId: String, name: String, size: Long, sha256: String, contentType: String)` | [Verified file](../results.md#resultfile) |
| `FileResult(files: Vector[ResultFile])` | Ordinary artifact return value |
| `DatasetPart(path: String, size: Long, sha256: String)` | [Distributed part](../results.md#datasetpart) |
| `DatasetManifest(allocationId: String, format: String, partCount: Long, totalBytes: Long, manifest: ResultFile)` | [Sealed manifest](../results.md#datasetmanifest) |
| `DatasetLocation(allocationId: String, uri: String, hadoopOptions: Map[String,String], expiresAt: Option[String], probePath: Option[String], probeValue: Option[String])` | [Worker write grant](../results.md#datasetaccess), not public output |
| `DatasetResult(files: Vector[ResultFile], dataset: Option[DatasetManifest] = None)` | Distributed artifact return value |
| `ProducedResult` | Sealed raw-handler return family below |
| `StatusResult` | Successful status-only raw handler |
| `JsonResult(value: Json)` | Raw JSON result |
| `ArtifactResult(files: Vector[ResultFile])` | Raw file result |
| `DatasetProduced(value: DatasetResult)` | Raw dataset result |

## LinhaWorker[E] {#linhaworker}

Constructor arguments:

| Argument | Type / default | Meaning |
| --- | --- | --- |
| `baseUrl` | `String`, required | Internal Linha server URL |
| `credential` | `WorkerCredential`, required | Managed identity and optional fixed token |
| `makeEngine` | `String => E`, required | Opens engine capability using the attempt-specific key |
| `cancelEngine` | `String => Unit`, no-op | Cancels only the attempt-specific engine work |
| `releaseEngine` | `E => Unit`, no-op | Releases request-scoped engine state |
| `capacity` | `Int`, `1` | 1–128 request slots; SparkRuntime supplies its chosen capacity |
| `credentials` | `Option[CredentialProvider]`, `None` | Renewable token source, preferred over fixed token when supplied |

| Member | Return type | Behavior |
| --- | --- | --- |
| `register[J <: LinhaEntrypoint[R,E],R](codec: EntrypointCodec[J,R])` | `this.type` | Registers a typed handler; duplicate handler/version is rejected |
| `handleRaw(capability: Capability)(handler: (Json,LinhaJobContext[E]) => ProducedResult)` | `this.type` | Registers an untyped handler |
| `run()` | `Unit` | Blocking claim/execute loop; requires at least one registration |
| `close()` | `Unit` | Stops acceptance, cancels active work and shuts down worker executors |
| `LinhaWorker.projectedIdentity()` | `WorkerCredential` | Reads injected context/Pod identity |
| `LinhaWorker.projectedCredentials()` | `CredentialProvider` | Rereads projected token, or anonymous credentials if master security is disabled |
| `LinhaWorker.local(baseUrl: String, credential: WorkerCredential, capacity: Int = 1)` | `LinhaWorker[Unit]` | Engine-free development runtime |

`WorkerIdentity(contextId: String, id: String, incarnation: String, expiresAt: Long)`
and `WorkerCredential(identity: WorkerIdentity, token: String)` carry runtime
identity; projected identity uses Pod UID for ID/incarnation and expiry `0L`.
Token validation and live-instance checks remain server-authoritative.

`RetryableJobException(message: String)` explicitly marks handler failure as
retry-eligible. Accepted maxAttempts still bounds execution. Ordinary handler
exceptions are not retried by default; worker shutdown and selected transport
errors can also produce retryable failures. Persisted diagnostics include phases,
causes/suppressed errors and bounded stack traces; see [Failure](../requests.md#failure).

## Managed worker environment {#managed-worker-environment}

Linha injects `LINHA_SERVER_URL`, `LINHA_CONTEXT_ID`, `LINHA_POD_UID`,
`LINHA_CAPACITY`, `LINHA_SECURITY_ENABLED` and `LINHA_WORKER_TOKEN_FILE` into
managed workers. These provide runtime URL, identity, concurrency and projected
credentials. Worker `LINHA_MAX_STAGING_BYTES` configures its positive staging
reservation budget (default 5 GiB). Do not replace managed LINHA_/SPARK_ variables
through the primary-container Pod template.
