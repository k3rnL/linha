# Scala client and job handles

Package `linha.client`, artifact `com.k3rnl:linha-client_2.12`. Requires Java 17,
Scala 2.12 and an implicit `scala.concurrent.ExecutionContext`. See
[dependencies](../../sdk-reference.md#dependency-declarations).

## LinhaClient {#linhaclient}

Constructor: `new LinhaClient(baseUrl: String, credentials: CredentialProvider)`.
The URL is the API HTTP(S) base, for example `https://linha.example`; credentials
are evaluated when sending each request. Use one client for the application
process lifetime. `close()` releases its attachments and stops lease scheduling.

| Member | Return type | Arguments/defaults and behavior |
| --- | --- | --- |
| `ensureBackend(name: String, spec: Json)` | `Future[LinhaContext]` | [BackendSpec](../backend/index.md#backendspec); acquires the derived version and client lease |
| `restoreContext(id: String)` | `Future[LinhaContext]` | Attaches to an existing immutable version; may reactivate its backend |
| `job[J,R](id: String, codec: EntrypointCodec[J,R])` | `JobHandle[R]` | Restores a typed handle; no context attachment or engine startup |
| `rawJob(id: String)` | `RawJobHandle` | Restores a raw handle; no network call during construction |
| `list(contextId: String = "", state: String = "", cursor: String = "", limit: Int = 50)` | `Future[Json]` | Owner-scoped [JobPage](../requests.md#jobpage), limit 1–200 |
| `close()` | `Unit` | Idempotent process shutdown; release failures are left to lease expiry |

Ensure resolves the image and validates effective settings; job submission only
waits for durable acceptance. References to the same version in one client share
an attachment. The SDK renews a 90-second client lease every 30 seconds and
reacquires it when the server explicitly reports expiry. Generic transport retries
are separate from job execution retries.

## LinhaContext {#linhacontext}

Returned by ensure/restore; constructed by the client. `id: String` is the context
version's persisted ID, and `version: String` is its configuration hash. Keep the
logical name unchanged during configuration/image rollouts.

| Member | Return type | Arguments/defaults and behavior |
| --- | --- | --- |
| `status()` | `Future[Json]` | [Context](../backend/index.md#context) status; GET does not itself renew leases |
| `submitRaw(request: RawRequest, idempotencyKey: String = UUID.randomUUID().toString, maxAttempts: Int = 1)` | `Future[RawJobHandle]` | Sends handler/version/payload, retry backoff 1 second, and the live client lease |
| `submit[J,R](entrypoint: J, idempotencyKey: String = UUID.randomUUID().toString, maxAttempts: Int = 1)(implicit codec: EntrypointCodec[J,R])` | `Future[JobHandle[R]]` | Encodes data through the registered codec; execution code stays in the worker image |
| `storage(destination: String)` | `Future[StorageSession]` | [Application storage](storage.md); requires an enabled alias captured in this context |
| `developmentWorker()` | `Future[Json]` | Requests local development identity; available only with security disabled |
| `close()` | `Unit` | Releases this handle; other handles retain a shared attachment until they close |

Submission completes with a durable handle before execution. Reuse an explicit
business idempotency key when retrying acceptance; the default generated key
creates a distinct business request on each application-level call. This SDK
overload does not expose every raw [SubmitRequest](../requests.md#submitrequest)
option, such as deadline or custom backoff; the HTTP API does.

## RawJobHandle {#rawjobhandle}

Prefer `client.rawJob(savedId)` for restoration. Constructor:
`new RawJobHandle(id: String, client: LinhaClient)`. Its public `id` is durable.

| Member | Return type | Arguments/defaults and behavior |
| --- | --- | --- |
| `status()` | `Future[Json]` | [Job](../requests.md#job) including current progress/failure |
| `attempts()` | `Future[Json]` | `items` of [Attempt](../requests.md#attempt) |
| `cancel()` | `Future[Json]` | Commits cancellation intent; terminal state may arrive later |
| `resultMetadata()` | `Future[Json]` | [Result](../results.md#result); requires published, unexpired output |
| `parts(cursor: String = "", limit: Int = 100)` | `Future[Json]` | [DatasetPartPage](../results.md#datasetpartpage), limit 1–200 |
| `download(allocationId: String, destination: Path)` | `Future[Path]` | Streams retained output through owner-authorized API |
| `downloadPart(part: String, destination: Path)` | `Future[Path]` | Uses a returned relative dataset part path verbatim |

Destination files must not already exist: downloads use `Files.copy` without
replacement. All asynchronous operations can fail their Future with
`LinhaException`, transport I/O errors or JSON decoding errors. See
[errors and transport](credentials.md#transport).

## JobHandle[R] {#jobhandle}

Extends RawJobHandle. Prefer `client.job(savedId, codec)`; explicit constructor is
`new JobHandle[R](id: String, expected: ResultDescriptor, decoder: Decoder[R], client: LinhaClient)`.

`result(): Future[R]` fetches available output; **it does not wait or poll for
execution**. Poll status until terminal before calling it. It compares the entire
result descriptor and throws `SCHEMA_MISMATCH` on mismatch. JSON decoding reads at
most 64 MiB even when the server policy permits larger results; use streaming
download for larger values. File/dataset results decode their descriptor lists;
status results decode from JSON null (Scala Unit). Expired payloads return
`RESULT_EXPIRED` even while the handle's job history remains accessible.
