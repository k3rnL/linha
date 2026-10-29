# Managed worker protocol

These routes are used by managed worker runtimes, independently of application
OAuth and browser login. Workers authenticate with their bound Kubernetes
service-account token and `X-Linha-Context-Id`, `X-Linha-Worker-Id`, and
`X-Linha-Worker-Incarnation` headers. Linha checks the live managed instance.

| Operation | Route |
| --- | --- |
| Register / heartbeat / claim | `POST /v1/workers/register`, `/v1/workers/heartbeat`, `/v1/workers/claim` |
| Renew / fail / complete | `POST /v1/jobs/{job}/renew`, `/fail`, `/complete` |
| Allocate / upload | `POST /v1/jobs/{job}/outputs`, `PUT /v1/jobs/{job}/outputs/{output}` |
| Dataset access / register / seal | `POST /v1/jobs/{job}/outputs/{output}/access`, `/parts`, `/seal` |

Every attempt mutation carries the current worker ID, incarnation, attempt ID and
fence, and requires an unexpired lease. Late writers cannot replace a retained
result. Upload requests use `X-Linha-Attempt` and `X-Linha-Fence` headers instead
of a JSON body. Claims only match a registered handler/version within worker
capacity, with concurrent claims serialized by the durable queue. Attempt leases default to 30 seconds;
the assignment carries the actual duration. SDK workers cancel locally on lease
loss. See [runtime API](../sdk/worker.md).

Claim query `waitSeconds` is an integer from 0–20, default 0. No assignment
returns HTTP 204 with no body; the Scala transport represents this as Json.Null.
Other successful receipts are:

| Operation | Output |
| --- | --- |
| Registration | `{"registered":true}` |
| Heartbeat | `{"draining":false}` (or true when draining) |
| Renew | `{"leaseExpiresAt":"..."}` in RFC 3339 |
| Fail | `{"recorded":true}` |
| Allocate | [Allocation](#allocation) |
| Upload | [ResultFile](../results.md#resultfile) |
| Complete | [Result](../results.md#result) |

Registration does not let a worker choose its draining state; the server controls
retirement. A repeated registration must preserve identity, capacity and
capabilities. A retired incarnation cannot resume claiming by registering again.

Dataset part registration accepts 1–200 parts per raw request; the SDK uses
batches of at most 100. Repeating a part path requires identical size/digest;
sealing freezes the inventory. The seal format must be `parquet`.


Field paths below are relative to their containing model; output fields have no caller default.

## Capability {#capability}

Worker registration input: one supported handler/version and its result descriptor.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="capability-handler"></span>`handler` | string | Required; no default | Stable registered handler name; executable code is installed in the worker image. |
| <span id="capability-version"></span>`version` | integer | Required; no default | Version of the associated protocol, handler or configuration. |
| <span id="capability-result"></span>`result` | [ResultDescriptor](../results.md#resultdescriptor) | Required; no default | Published result or registered result descriptor as specified by the containing model. |

## Worker {#worker}

Registration input; runtime identity is bound to the managed instance, not supplied by an application client.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="worker-id"></span>`id` | string | Required; no default | Opaque server/resource identity; preserve it verbatim. |
| <span id="worker-contextid"></span>`contextId` | string | Required; no default | Opaque immutable context-version ID, not the logical context name. |
| <span id="worker-incarnation"></span>`incarnation` | string | Required; no default | Managed worker incarnation; prevents a replacement process from impersonating an old attempt. |
| <span id="worker-capacity"></span>`capacity` | integer | Required; no default | Concurrent request slots; worker registration accepts 1–128, Spark backend concurrency 1–32. Minimum: 1. Maximum: 128. |
| <span id="worker-capabilities"></span>`capabilities` | array of [Capability](workers.md#capability) | Required; no default | Nonempty list (at most 256) of unique handler/version/result contracts. |
| <span id="worker-draining"></span>`draining` | boolean | Optional; omitted | Whether the worker has stopped accepting new assignments. |

## Assignment {#assignment}

Output from a successful worker claim; HTTP 204 (SDK Json.Null) means no compatible eligible work was assigned.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="assignment-job"></span>`job` | [Job](../requests.md#job) | Present; no caller default | Job assignment or compact action result; follow its linked model. |
| <span id="assignment-attemptid"></span>`attemptId` | string | Present; no caller default | Opaque persisted attempt identity; belongs to one job execution attempt. |
| <span id="assignment-fence"></span>`fence` | integer | Present; no caller default | Monotonic attempt fence; must match the job current attempt on every mutation. |
| <span id="assignment-leaseexpiresat"></span>`leaseExpiresAt` | string (date-time) | Present; no caller default | Current attempt lease deadline from the server (RFC 3339). |
| <span id="assignment-leasedurationmillis"></span>`leaseDurationMillis` | integer | Present; no caller default | Authoritative attempt lease budget in milliseconds; SDK uses a local monotonic deadline. |
| <span id="assignment-results"></span>`results` | [ResultPolicy](../backend/results.md#resultpolicy) | Present; no caller default | Result publication/storage policy, separate from application file access. |
| <span id="assignment-resultdescriptor"></span>`resultDescriptor` | [ResultDescriptor](../results.md#resultdescriptor) | Present; no caller default | Result contract chosen from the matching registered capability. |

## AttemptUpdate {#attemptupdate}

Common worker mutation input. All fields must match the current attempt.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="attemptupdate-workerid"></span>`workerId` | string | Required; no default | Current registered worker identity. |
| <span id="attemptupdate-incarnation"></span>`incarnation` | string | Required; no default | Managed worker incarnation; prevents a replacement process from impersonating an old attempt. |
| <span id="attemptupdate-attemptid"></span>`attemptId` | string | Required; no default | Opaque persisted attempt identity; belongs to one job execution attempt. |
| <span id="attemptupdate-fence"></span>`fence` | integer | Required; no default | Monotonic attempt fence; must match the job current attempt on every mutation. |

## RenewRequest {#renewrequest}

Input to `POST /v1/jobs/{job}/renew`; extends worker authority and optionally reports progress.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="renewrequest-workerid"></span>`workerId` | string | Required; no default | Current registered worker identity. |
| <span id="renewrequest-incarnation"></span>`incarnation` | string | Required; no default | Managed worker incarnation; prevents a replacement process from impersonating an old attempt. |
| <span id="renewrequest-attemptid"></span>`attemptId` | string | Required; no default | Opaque persisted attempt identity; belongs to one job execution attempt. |
| <span id="renewrequest-fence"></span>`fence` | integer | Required; no default | Monotonic attempt fence; must match the job current attempt on every mutation. |
| <span id="renewrequest-progress"></span>`progress` | [Progress](../requests.md#progress) | Optional; omitted | Optional current-attempt progress. Cleared when a new attempt is claimed. |

## FailureRequest {#failurerequest}

Input to `POST /v1/jobs/{job}/fail`; reports diagnostics under attempt fencing.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="failurerequest-workerid"></span>`workerId` | string | Required; no default | Current registered worker identity. |
| <span id="failurerequest-incarnation"></span>`incarnation` | string | Required; no default | Managed worker incarnation; prevents a replacement process from impersonating an old attempt. |
| <span id="failurerequest-attemptid"></span>`attemptId` | string | Required; no default | Opaque persisted attempt identity; belongs to one job execution attempt. |
| <span id="failurerequest-fence"></span>`fence` | integer | Required; no default | Monotonic attempt fence; must match the job current attempt on every mutation. |
| <span id="failurerequest-failure"></span>`failure` | [Failure](../requests.md#failure) | Required; no default | Persisted execution diagnostics, when present. |

## OutputRequest {#outputrequest}

Input to `POST /v1/jobs/{job}/outputs`; persists an allocation before authorizing a write.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="outputrequest-workerid"></span>`workerId` | string | Required; no default | Current registered worker identity. |
| <span id="outputrequest-incarnation"></span>`incarnation` | string | Required; no default | Managed worker incarnation; prevents a replacement process from impersonating an old attempt. |
| <span id="outputrequest-attemptid"></span>`attemptId` | string | Required; no default | Opaque persisted attempt identity; belongs to one job execution attempt. |
| <span id="outputrequest-fence"></span>`fence` | integer | Required; no default | Monotonic attempt fence; must match the job current attempt on every mutation. |
| <span id="outputrequest-name"></span>`name` | string | Required; no default | Logical name within its containing model. |
| <span id="outputrequest-kind"></span>`kind` | string | Required; no default | Result/artifact kind; see the containing model for allowed kinds. Values: `json`, `file`, `dataset`. |
| <span id="outputrequest-contenttype"></span>`contentType` | string | Required; no default | MIME content type. Output allocation values are nonempty, at most 256 bytes, with no CR/LF. |

## Allocation {#allocation}

Worker-only output allocation. It is not proof that a result has been published.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="allocation-id"></span>`id` | string | Present; no caller default | Opaque server/resource identity; preserve it verbatim. |
| <span id="allocation-jobid"></span>`jobId` | string | Present; no caller default | Public persisted job ID. |
| <span id="allocation-attemptid"></span>`attemptId` | string | Present; no caller default | Opaque persisted attempt identity; belongs to one job execution attempt. |
| <span id="allocation-name"></span>`name` | string | Present; no caller default | Logical name within its containing model. |
| <span id="allocation-kind"></span>`kind` | string | Present; no caller default | Result/artifact kind; see the containing model for allowed kinds. |
| <span id="allocation-contenttype"></span>`contentType` | string | Present; no caller default | MIME content type. Output allocation values are nonempty, at most 256 bytes, with no CR/LF. |
| <span id="allocation-key"></span>`key` | string | Present; no caller default | Persisted relative allocation key; native storage identity remains internal to execution. |
| <span id="allocation-uri"></span>`uri` | string | Optional output; no caller default | Authorized allocation/session location. It is not a durable public download URL. |
| <span id="allocation-uploadurl"></span>`uploadUrl` | string | Optional output; no caller default | Worker-authorized API upload path for this allocation. |

## Completion {#completion}

Input to `POST /v1/jobs/{job}/complete`; commits successful publication. Identical committed retries return the receipt; different completion content conflicts.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="completion-workerid"></span>`workerId` | string | Required; no default | Current registered worker identity. |
| <span id="completion-incarnation"></span>`incarnation` | string | Required; no default | Managed worker incarnation; prevents a replacement process from impersonating an old attempt. |
| <span id="completion-attemptid"></span>`attemptId` | string | Required; no default | Opaque persisted attempt identity; belongs to one job execution attempt. |
| <span id="completion-fence"></span>`fence` | integer | Required; no default | Monotonic attempt fence; must match the job current attempt on every mutation. |
| <span id="completion-descriptor"></span>`descriptor` | [ResultDescriptor](../results.md#resultdescriptor) | Required; no default | Registered result identity; kind/schema/version must match the claimed capability. |
| <span id="completion-files"></span>`files` | array of [ResultFile](../results.md#resultfile) | Required; no default | Verified output descriptors. JSON requires exactly one file; status requires none. |
| <span id="completion-dataset"></span>`dataset` | [DatasetManifest](../results.md#datasetmanifest) | Optional; omitted | Optional sealed distributed dataset metadata, valid only for dataset results. |

## DatasetPartRegistration {#datasetpartregistration}

Input to register parts under the current attempt. SDK batches contain at most 100 parts.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="datasetpartregistration-workerid"></span>`workerId` | string | Required; no default | Current registered worker identity. |
| <span id="datasetpartregistration-incarnation"></span>`incarnation` | string | Required; no default | Managed worker incarnation; prevents a replacement process from impersonating an old attempt. |
| <span id="datasetpartregistration-attemptid"></span>`attemptId` | string | Required; no default | Opaque persisted attempt identity; belongs to one job execution attempt. |
| <span id="datasetpartregistration-fence"></span>`fence` | integer | Required; no default | Monotonic attempt fence; must match the job current attempt on every mutation. |
| <span id="datasetpartregistration-parts"></span>`parts` | array of [DatasetPart](../results.md#datasetpart) | Required; no default | Dataset parts to register; at most 100 per SDK batch, 100000 per allocation. Minimum items: 1. Maximum items: 200. |

## DatasetSeal {#datasetseal}

Input to seal the registered dataset before completing the attempt.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="datasetseal-workerid"></span>`workerId` | string | Required; no default | Current registered worker identity. |
| <span id="datasetseal-incarnation"></span>`incarnation` | string | Required; no default | Managed worker incarnation; prevents a replacement process from impersonating an old attempt. |
| <span id="datasetseal-attemptid"></span>`attemptId` | string | Required; no default | Opaque persisted attempt identity; belongs to one job execution attempt. |
| <span id="datasetseal-fence"></span>`fence` | integer | Required; no default | Monotonic attempt fence; must match the job current attempt on every mutation. |
| <span id="datasetseal-format"></span>`format` | string | Required; no default | Dataset serialization format; currently parquet is supported. Values: `parquet`. |
