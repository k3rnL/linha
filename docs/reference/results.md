# Published results and datasets

A completed upload is not yet a public result. The current fenced attempt must
successfully publish verified output. Retained results can be retrieved by the
same owner and job ID without attaching a context or starting an engine.

The descriptor kind is `json`, `file`, `dataset` or `status`. A status-only result
has an empty `files` array and no dataset; Scala `Unit` entrypoints use this kind.
JSON requires exactly one file; ordinary file results require at least one.
Summed file bytes are bounded by the accepted maxBytes. Datasets retain a sealed
manifest plus an immutable part inventory; total part bytes and manifest bytes
are bounded separately. The supported dataset format is currently `parquet`.
After payload retention expires, status/history remain and retrieval returns
`RESULT_EXPIRED`. Physical deletion is separately opt-in.


Field paths below are relative to their containing model; output fields have no caller default.

## ResultDescriptor {#resultdescriptor}

Registered worker capability and retained result identity. Typed clients compare all three members before decoding.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="resultdescriptor-kind"></span>`kind` | string | Required; no default | Result/artifact kind; see the containing model for allowed kinds. Values: `json`, `file`, `dataset`, `status`. |
| <span id="resultdescriptor-schema"></span>`schema` | string | Required; no default | Stable application result schema identity, not an embedded JSON Schema document. |
| <span id="resultdescriptor-version"></span>`version` | integer | Required; no default | Version of the associated protocol, handler or configuration. Minimum: 1. |

## ResultFile {#resultfile}

Verified output metadata returned by upload and retained in a successful result.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="resultfile-allocationid"></span>`allocationId` | string | Present; no caller default | Opaque persisted output allocation ID. |
| <span id="resultfile-name"></span>`name` | string | Present; no caller default | Allocated relative output name, not a local client filename. |
| <span id="resultfile-size"></span>`size` | integer | Present; no caller default | Byte count, nonnegative. Minimum: 0. |
| <span id="resultfile-sha256"></span>`sha256` | string | Present; no caller default | SHA-256 hexadecimal digest of the exact bytes. Pattern: `^[0-9a-f]{64}$`. |
| <span id="resultfile-contenttype"></span>`contentType` | string | Present; no caller default | MIME content type. Output allocation values are nonempty, at most 256 bytes, with no CR/LF. |

## Result {#result}

Output of `GET /v1/jobs/{job}/result`, also embedded in a successful Job.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="result-descriptor"></span>`descriptor` | [ResultDescriptor](results.md#resultdescriptor) | Present; no caller default | Registered result identity; kind/schema/version must match the claimed capability. |
| <span id="result-files"></span>`files` | array of [ResultFile](results.md#resultfile) | Present; no caller default | Verified output descriptors. JSON requires exactly one file; status requires none. |
| <span id="result-expiresat"></span>`expiresAt` | string (date-time) | Present; no caller default | Server-recorded expiry instant (RFC 3339); check before using retained access. |
| <span id="result-expired"></span>`expired` | boolean | Present; no caller default | Whether payload retention has ended; job IDs and status remain. |
| <span id="result-dataset"></span>`dataset` | [DatasetManifest](results.md#datasetmanifest) | Optional output; no caller default | Optional sealed distributed dataset metadata, valid only for dataset results. |

## DatasetPart {#datasetpart}

Input when registering a distributed part; output in part listings.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="datasetpart-path"></span>`path` | string | Required; no default | Relative part path under the allocated dataset; use returned paths verbatim. |
| <span id="datasetpart-size"></span>`size` | integer (int64) | Required; no default | Byte count, nonnegative. Minimum: 0. |
| <span id="datasetpart-sha256"></span>`sha256` | string | Required; no default | SHA-256 hexadecimal digest of the exact bytes. Pattern: `^[0-9a-f]{64}$`. |

## DatasetManifest {#datasetmanifest}

Sealed dataset metadata returned on publication; the manifest itself is a retained file.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="datasetmanifest-allocationid"></span>`allocationId` | string | Present; no caller default | Opaque persisted output allocation ID. |
| <span id="datasetmanifest-format"></span>`format` | string | Present; no caller default | Dataset serialization format; currently parquet is supported. Values: `parquet`. |
| <span id="datasetmanifest-partcount"></span>`partCount` | integer (int64) | Present; no caller default | Number of registered published parts, at most 100000. Minimum: 0. Maximum: 100000. |
| <span id="datasetmanifest-totalbytes"></span>`totalBytes` | integer (int64) | Present; no caller default | Total dataset part bytes; bounded by the accepted result policy. Minimum: 0. |
| <span id="datasetmanifest-manifest"></span>`manifest` | [ResultFile](results.md#resultfile) | Present; no caller default | Retained JSON manifest ResultFile; must match the dataset result file. |

## DatasetPartPage {#datasetpartpage}

Output of `GET /v1/jobs/{job}/parts`; default limit 100, maximum 200.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="datasetpartpage-items"></span>`items` | array of [DatasetPart](results.md#datasetpart) | Present; no caller default | Items in this page/receipt; follow the linked item type. Maximum items: 200. |
| <span id="datasetpartpage-nextcursor"></span>`nextCursor` | string | Optional output; no caller default | Opaque continuation token. Omission/empty value means no further page. |

## DatasetAccess {#datasetaccess}

Worker-only write access returned for a dataset allocation. Never an owner-facing retained result.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="datasetaccess-allocationid"></span>`allocationId` | string | Present; no caller default | Opaque persisted output allocation ID. |
| <span id="datasetaccess-uri"></span>`uri` | string | Present; no caller default | Authorized allocation/session location. It is not a durable public download URL. |
| <span id="datasetaccess-hadoopoptions"></span>`hadoopOptions` | map of string | Present; no caller default | Ephemeral per-write settings including prefix-scoped STS credentials. Do not log or persist. |
| <span id="datasetaccess-expiresat"></span>`expiresAt` | string (date-time) | Optional output; no caller default | Delegated writer expiry when present; at most one hour with no transparent renewal mid-write. |
| <span id="datasetaccess-probepath"></span>`probePath` | string | Optional output; no caller default | Local shared-mount marker path used to verify all writers see the same filesystem. |
| <span id="datasetaccess-probevalue"></span>`probeValue` | string | Optional output; no caller default | Expected shared-mount marker contents. |
