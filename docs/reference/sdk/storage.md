# Scala application storage and Hadoop

Packages `linha` and `linha.hadoop`. Application sessions are obtained from
`LinhaContext.storage(destination): Future[StorageSession]` or
`LinhaJobContext.storage(destination): StorageSession`. See the
[wire models](../storage.md) and [application-file guide](../../application-storage.md).

## StorageSession {#storagesession}

Construction is controlled by the SDK. Public values are `serverURL`, `id`,
`destination`, `root`, and `uri` (all String). The session token is private;
`toString` contains only destination and ID. Hadoop configuration necessarily
contains its scoped bearer token and should not be logged or saved in payloads.

| Member | Return type | Defaults and behavior |
| --- | --- | --- |
| `relative(value: String)` | `String` | Resolves relative, same-session linha://, or in-destination s3:// / s3a:// paths; rejects another session/bucket/prefix |
| `path(value: String)` | `String` | Returns a credential-free linha:// session URI |
| `hadoopOptions` | `Map[String,String]` | Filesystem class/cache flags plus session server/token/root/destination keys |
| `stat(value: String)` | `StorageObject` | Object/prefix metadata |
| `list(value: String)` | `Vector[StorageObject]` | At most 10,000 immediate entries |
| `open(value: String, offset: Long = 0L)` | `InputStream` | Nonnegative offset; Range read when offset > 0; caller closes stream |
| `upload(value: String, source: Path)` | `Unit` | Replaces object from local file, at most 5 GiB, through server staging |
| `mkdir(value: String)` | `Unit` | Creates directory marker |
| `delete(value: String, recursive: Boolean = false)` | `Unit` | Missing target is harmless; nonempty directories require recursive=true |
| `rename(source: String, target: String)` | `Unit` | Copy/delete, target must be absent; not atomic |
| `lock(value: String, waitSeconds: Long = 120)` | `session.StorageLock` | Directory lease; not destination root or duplicate local lock; retries conflicts every 250 ms until wait bound |
| `close()` | `Unit` | Releases held locks, revokes session and removes local cache; cleanup errors can be thrown with suppressed secondary errors |

`StorageObject(path: String, directory: Boolean, size: Long, modifiedAt: String)`
is the public immutable metadata value; its fields match
[StorageObject JSON](../storage.md#storageobject). Session calls are synchronous
and blocking; API errors use LinhaException, path errors use
IllegalArgumentException. Session validity depends on the live attachment or
attempt. Reconstructing a session does not extend its authority.

## StorageLock {#storagelock}

Nested class `StorageSession.StorageLock` returned by `lock`. `check(): Unit`
throws STALE_ATTEMPT when its locally tracked lease is lost. `close(): Unit`
stops renewal and releases the server lock. The lease lasts 30 seconds and the
SDK renews every eight seconds; a renewal error marks it invalid locally.
Protected writes send the longest matching held directory lock. A stale lock
cannot begin a protected publication; uncoordinated external storage writers
are outside this guarantee.

## Session factories {#session-factories}

| Static method | Return type | Contract |
| --- | --- | --- |
| `StorageSession.fromJson(server: String, json: Json)` | `StorageSession` | Restores from the server session-creation response |
| `StorageSession.restore(server: String, id: String, token: String, destination: String, root: String)` | `StorageSession` | Reuses local session by ID; existing server/token/root must match |

Use normal context/job-context session acquisition in application code. These
factories support Hadoop integration; they cannot revive an expired server lease.

## LinhaFileSystem {#linhafilesystem}

Artifact `com.k3rnl:linha-hadoop_2.12`, class
`linha.hadoop.LinhaFileSystem extends org.apache.hadoop.fs.FileSystem`.
The URI scheme is linha, with session ID as authority. `session.hadoopOptions`
sets these configuration keys:

| Key | Value |
| --- | --- |
| `fs.linha.impl` | `linha.hadoop.LinhaFileSystem` |
| `fs.linha.impl.disable.cache` | `true` |
| `fs.linha.session.<id>.server` | Linha API base |
| `fs.linha.session.<id>.token` | Scoped session bearer token |
| `fs.linha.session.<id>.root` | Physical destination root |
| `fs.linha.session.<id>.destination` | Destination alias |

| Public override/helper | Behavior |
| --- | --- |
| `initialize(uri: URI, config: Configuration): Unit` | Requires linha://session-id/ and the session keys above |
| `getUri: URI`, `getScheme: String` | Initialized virtual root and linha scheme |
| `getWorkingDirectory: Path`, `setWorkingDirectory(path: Path): Unit` | Session-relative working directory |
| `getFileStatus(path: Path): FileStatus` | Metadata; synthetic block size 64 MiB, replication 1 |
| `listStatus(path: Path): Array[FileStatus]` | Directory entries, or the single file status |
| `open(path: Path, bufferSize: Int): FSDataInputStream` | Seek/range reads; negative seeks rejected |
| `create(path: Path, permission: FsPermission, overwrite: Boolean, bufferSize: Int, replication: Short, blockSize: Long, progress: Progressable): FSDataOutputStream` | Stages a local temporary file, uploads on close, maximum 5 GiB |
| `append(path: Path, bufferSize: Int, progress: Progressable): FSDataOutputStream` | Unsupported; throws IOException |
| `mkdirs(path: Path, permission: FsPermission): Boolean` | Creates marker via session |
| `delete(path: Path, recursive: Boolean): Boolean` | False for absent path; recursive behavior follows session |
| `rename(source: Path, destination: Path): Boolean` | Copy/delete; existing directory destination appends source basename, existing target returns false |
| `LinhaFileSystem.lookup(uri: URI): LinhaFileSystem` | Retrieves an already initialized filesystem for libraries without Configuration injection |

HTTP 404 becomes FileNotFoundException; other API failures become IOException.
Permissions, replication and block size arguments do not turn S3 into POSIX
storage. Use a dedicated per-operation Hadoop configuration and close filesystem
instances when done. Spark readers/writers use the same scoped session path;
application files are not automatically retained as job results.
