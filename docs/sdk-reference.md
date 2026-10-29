# Scala/JVM SDK reference

All Scala artifacts use binary version 2.12 and Java 17. This guide uses SDK
version `0.1.4-SNAPSHOT`.

<!-- development-only:start -->
Build from source to use these development coordinates, or select the published
`0.1.4` version from Maven Central.
<!-- development-only:end -->

| Maven artifact | Responsibility |
| --- | --- |
| `com.k3rnl:linha-protocol_2.12` | Wire contracts, codecs, credentials and result paths |
| `com.k3rnl:linha-client_2.12` | Client, contexts, job handles and OAuth acquisition |
| `com.k3rnl:linha-worker_2.12` | Handler runtime, leases, progress and publication |
| `com.k3rnl:linha-hadoop_2.12` | Scoped application filesystem with range reads |
| `com.k3rnl:linha-spark_2.12` | SparkContext sessions and distributed result helpers |

## Dependency declarations {#dependency-declarations}

For an SBT application that includes both client and worker examples:

```scala
scalaVersion := "2.12.20"
val linhaVersion = "0.1.4-SNAPSHOT"
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

## Client and context {#client-and-context}

Complete constructors, arguments, defaults, return types and lifecycle behavior:
[Scala client, contexts and handles](reference/sdk/client.md). Backend declarations
are Json values described by [BackendSpec](reference/backend/index.md#backendspec).

## Job handles {#job-handles}

[RawJobHandle](reference/sdk/client.md#rawjobhandle) and
[JobHandle](reference/sdk/client.md#jobhandle) cover restored IDs, status,
attempts, cancellation and retained results. Submission futures acknowledge
durable acceptance; result() retrieves available output without polling.

## OAuth credentials {#oauth-credentials}

See [CredentialProvider](reference/sdk/credentials.md#credentialprovider),
[OAuth factory arguments](reference/sdk/credentials.md#oauth-client-credentials),
[IdentityHttpOptions](reference/sdk/credentials.md#identityhttpoptions), and
[transport/errors](reference/sdk/credentials.md#transport).

## Entrypoints and worker context {#entrypoints-and-worker-context}

See [entrypoints/codecs](reference/sdk/worker.md#entrypointcodec),
[LinhaJobContext](reference/sdk/worker.md#linhajobcontext),
[LinhaWorker](reference/sdk/worker.md#linhaworker), and
[Spark runtime and syntax](reference/sdk/spark.md).

## Result types and paths {#result-types-and-paths}

[ResultWriter and produced types](reference/sdk/worker.md#resultwriter),
[result paths/storage helpers](reference/sdk/helpers.md#resultpath), and
[PodTemplate/SparkPodTemplates](reference/sdk/helpers.md) list their public APIs.
[StorageSession and LinhaFileSystem](reference/sdk/storage.md) cover application
files. Use the [type index](reference/index.md#scala-type-index) for exact names.
