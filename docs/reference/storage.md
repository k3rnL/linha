# Application storage models

Application storage exposes input files, shared data and temporary processing
files through an explicitly permitted S3 destination. It is independent of
[retained job results](results.md): result expiry never deletes application files.
An authorized context can access the entire configured destination prefix.

## WorkloadStoragePolicy {#workloadstoragepolicy}

Input at `BackendSpec.storage`. An omitted storage object grants no application
access. The server snapshots physical destination fingerprints into the context
version. Configure operator access through [S3Config](deployment/storage.md#s3config).

```json
{"storage":{"destinations":["application-data"]}}
```

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="workloadstoragepolicy-destinations"></span>`destinations` | array of strings | Required when storage is present | 1–16 unique operator-enabled aliases; each requires `allowWorkloadAccess: true` |
| <span id="workloadstoragepolicy-fingerprints"></span>`fingerprints` | map of strings | Server output | Alias → physical destination fingerprint; server replaces any caller value |

## StorageSessionInput {#storagesessioninput}

Input to `POST /v1/contexts/{context}/storage-sessions` for clients, or
`POST /v1/jobs/{job}/storage-sessions` for workers. The requested alias must be
included in the backend's captured storage policy.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="storagesessioninput-destination"></span>`destination` | string | Required | Allowed application-storage alias |
| <span id="storagesessioninput-clientlease"></span>`clientLease` | string | Required for clients | Live attachment on the named context, owned by the caller |
| <span id="storagesessioninput-workerid"></span>`workerId` | string | Required for workers | Current worker identity |
| <span id="storagesessioninput-incarnation"></span>`incarnation` | string | Required for workers | Current managed incarnation |
| <span id="storagesessioninput-attemptid"></span>`attemptId` | string | Required for workers | Current live attempt |
| <span id="storagesessioninput-fence"></span>`fence` | integer | Required for workers | Current attempt fence |

## StorageSession {#storagesession}

Output of successful session creation (HTTP 201). Clients are bound to a live
client lease; workers are bound to their live attempt. A bearer token alone
cannot revive expired authority. Destination changes invalidate old sessions.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="storagesession-id"></span>`id` | string | Output | Opaque session identity |
| <span id="storagesession-token"></span>`token` | string | Output credential | Scoped bearer token used for session I/O, not the application's OAuth token |
| <span id="storagesession-destination"></span>`destination` | string | Output | Captured destination alias |
| <span id="storagesession-root"></span>`root` | string | Output | Physical `s3://bucket/prefix` root |
| <span id="storagesession-uri"></span>`uri` | string | Output | `linha://{session-id}/` virtual root; credentials are separate |

Use the [Scala session API](sdk/storage.md#storagesession) or the routes below.
Session tokens and Hadoop options are scoped credentials: keep them out of
durable job payloads and logs. Pass destination names and relative paths instead.

## Session I/O {#session-io}

Base: `/v1/storage/sessions/{session}`. Send the session token in
`Authorization: Bearer ...`. Query `path` is relative to the destination root.
Names are at most 1024 UTF-8 bytes; traversal, empty interior segments, control
characters, encoded paths and reserved `_linha` segments are rejected.

| Method / suffix | Inputs and output |
| --- | --- |
| `GET /stat?path=...` | [StorageObject](#storageobject) |
| `GET /list?path=...` | `{"items":[StorageObject,...]}`, at most 10,000 entries |
| `GET /data?path=...` | Byte stream; optional `Range` supports seek/range reads |
| `PUT /data?path=...` | Requires accurate Content-Length, at most 5 GiB; replaces the object |
| `POST /mkdir?path=...` | Directory marker; empty root is a no-op |
| `DELETE /delete?path=...&recursive=false` | Nonempty directories need recursive=true; root cannot be removed |
| `POST /rename?path=...&target=...` | Copy/delete; destination must be absent and outside the source subtree |
| `POST /lock?path=...` | Acquire 30-second directory lease; `renew=true` renews it |
| `DELETE /lock?path=...` | Release directory lease |
| `DELETE /close` | Revoke session; HTTP 204 |

Mutations return `{"ok":true}` except lock operations, which return
`{"leaseSeconds":30}`. Recursive delete/rename inventory is capped at 10,000
objects. Copy/delete rename is not an atomic filesystem rename.

`X-Linha-Storage-Lock` identifies the locked directory for a protected write.
Both source and target must stay within it. The SDK renews held locks every eight
seconds and checks them before protected writes. Locks coordinate cooperating
Linha writers; arbitrary external S3 writers are outside this protocol.

## StorageObject {#storageobject}

Output of stat/list. Root and synthetic directories can have zero-valued size and
timestamp; they do not represent a retained result descriptor.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="storageobject-path"></span>`path` | string | Output | Destination-relative path; empty for the root |
| <span id="storageobject-directory"></span>`directory` | boolean | Output | Directory/prefix versus object |
| <span id="storageobject-size"></span>`size` | integer | Output | Object length in bytes |
| <span id="storageobject-modifiedat"></span>`modifiedAt` | RFC 3339 string | Output | Provider modification time when known |

For procedures, see [access application files](../application-storage.md).
