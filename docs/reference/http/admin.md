# Administrator request models

Administrator routes are a separate cross-owner surface under `/v1/admin/`.
Viewer membership permits inspection; mutations require operator membership.
Browser cookie mutations also require same-origin `Origin` and `X-Linha-CSRF`.
Application OAuth identity alone does not grant administrator membership.
See [console procedures](../../admin-ui.md) and [public routes](../../http-api.md).

For `POST /v1/admin/jobs/batch`, malformed input changes no jobs. A valid batch
returns HTTP 200 with independent per-item outcomes; failed items do not roll
back successful items. Moves retain IDs, requests, original result policy,
attempt counts and retry backoff. The destination must be active and match owner,
logical name, engine type/version and source workload-storage access.

```json
{"action":"move","targetContextId":"corrected-context-id","jobs":[{"id":"request-id","contextId":"old-context-id"}]}
```


Field paths below are relative to their containing model; output fields have no caller default.

## AdminSession {#adminsession}

Output of `GET /v1/admin/session`; current browser/admin identity and mutation token.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="adminsession-actor"></span>`actor` | string | Present; no caller default | Authenticated administrator identity recorded for this action/session. |
| <span id="adminsession-role"></span>`role` | string | Present; no caller default | Administrator role: viewer or operator. Values: `viewer`, `operator`. |
| <span id="adminsession-csrf"></span>`csrf` | string | Optional output; no caller default | Session-bound CSRF value required for browser cookie mutations. |
| <span id="adminsession-expiresat"></span>`expiresAt` | string (date-time) | Optional output; no caller default | Server-recorded expiry instant (RFC 3339); check before using retained access. |

## AdminJob {#adminjob}

Administrator job output with owner, context and replay metadata.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="adminjob-id"></span>`id` | string | Present; no caller default | Opaque server/resource identity; preserve it verbatim. |
| <span id="adminjob-contextid"></span>`contextId` | string | Present; no caller default | Opaque immutable context-version ID, not the logical context name. |
| <span id="adminjob-request"></span>`request` | [SubmitRequest](../requests.md#submitrequest) | Present; no caller default | Original accepted SubmitRequest; payload and retry policy remain durable. |
| <span id="adminjob-state"></span>`state` | string | Present; no caller default | Current persisted or observed lifecycle state; see the model notes. Values: `QUEUED`, `RUNNING`, `RETRYING`, `CANCELLING`, `CANCELLED`, `FAILED`, `SUCCEEDED`. |
| <span id="adminjob-submittedat"></span>`submittedAt` | string (date-time) | Present; no caller default | Original server acceptance instant, retained across retry and queue movement (RFC 3339). |
| <span id="adminjob-updatedat"></span>`updatedAt` | string (date-time) | Present; no caller default | Last durable job state/update instant (RFC 3339). |
| <span id="adminjob-attemptcount"></span>`attemptCount` | integer | Present; no caller default | Number of persisted execution attempts, including lost attempts. |
| <span id="adminjob-progress"></span>`progress` | [Progress](../requests.md#progress) | Optional output; no caller default | Optional current-attempt progress. Cleared when a new attempt is claimed. |
| <span id="adminjob-failure"></span>`failure` | [Failure](../requests.md#failure) | Optional output; no caller default | Persisted execution diagnostics, when present. |
| <span id="adminjob-result"></span>`result` | [Result](../results.md#result) | Optional output; no caller default | Published result or registered result descriptor as specified by the containing model. |
| <span id="adminjob-owner"></span>`owner` | string | Optional output; no caller default | Authenticated application owner; supplied by the server, not application request JSON. |
| <span id="adminjob-contextname"></span>`contextName` | string | Optional output; no caller default | Stable logical context name. |
| <span id="adminjob-contextversion"></span>`contextVersion` | string | Optional output; no caller default | Hash identifying the effective normalized backend configuration. |
| <span id="adminjob-metriccontext"></span>`metricContext` | string | Optional output; no caller default | Bounded metric alias for this owner/logical context. |
| <span id="adminjob-submittedby"></span>`submittedBy` | string | Optional output; no caller default | Administrator actor that accepted/replayed this request, if any. |
| <span id="adminjob-replayedfrom"></span>`replayedFrom` | string | Optional output; no caller default | Optional source public job ID recorded for an administrator replay. |

## AdminSubmitRequest {#adminsubmitrequest}

Operator input for a new or replayed request in an administrator-selected context.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="adminsubmitrequest-handler"></span>`handler` | string | Required; no default | Stable registered handler name; executable code is installed in the worker image. Minimum length: 1. Maximum length: 256. |
| <span id="adminsubmitrequest-version"></span>`version` | integer | Required; no default | Version of the associated protocol, handler or configuration. Minimum: 1. |
| <span id="adminsubmitrequest-payload"></span>`payload` | JSON value | Required; no default | Any valid JSON value understood by this handler; no remotely executable classes or closures. |
| <span id="adminsubmitrequest-idempotencykey"></span>`idempotencyKey` | string | Required; no default | Business acceptance key, 1–256 bytes, scoped to owner and logical context name. Minimum length: 1. Maximum length: 256. |
| <span id="adminsubmitrequest-retry"></span>`retry` | [RetryPolicy](../requests.md#retrypolicy) | Optional; omitted | Bounded retry policy; omission disables retries beyond the initial attempt. |
| <span id="adminsubmitrequest-deadline"></span>`deadline` | string (date-time) | Optional; omitted | Optional absolute RFC 3339 execution deadline. Recovery also fails queued/retrying work after it passes. |
| <span id="adminsubmitrequest-replayedfrom"></span>`replayedFrom` | string | Optional; omitted | Optional source public job ID recorded for an administrator replay. |
| <span id="adminsubmitrequest-activateifstopped"></span>`activateIfStopped` | boolean | Optional; omitted | Explicit operator consent to activate a stopped context for new work; default false. |

## AdminResult {#adminresult}

Administrator retained-result metadata; file details may include authorized physical locations.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="adminresult-descriptor"></span>`descriptor` | [ResultDescriptor](../results.md#resultdescriptor) | Present; no caller default | Registered result identity; kind/schema/version must match the claimed capability. |
| <span id="adminresult-expiresat"></span>`expiresAt` | string (date-time) | Present; no caller default | Server-recorded expiry instant (RFC 3339); check before using retained access. |
| <span id="adminresult-expired"></span>`expired` | boolean | Present; no caller default | Whether payload retention has ended; job IDs and status remain. |
| <span id="adminresult-files"></span>`files` | array of object ([fields](#adminresult-files-model)) | Present; no caller default | Verified output descriptors. JSON requires exactly one file; status requires none. |
| <span id="adminresult-dataset"></span>`dataset` | object ([fields](#adminresult-dataset-model)) | Optional output; no caller default | Optional sealed distributed dataset metadata, valid only for dataset results. |

### files[] {#adminresult-files-model}

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="adminresult-files-file"></span>`files[].file` | [ResultFile](../results.md#resultfile) | Present; no caller default | Retained ResultFile metadata for this administrator-visible artifact. |
| <span id="adminresult-files-location"></span>`files[].location` | string | Present; no caller default | Published physical path or S3 URI when safely available; staging locations are not exposed. |
| <span id="adminresult-files-unavailable"></span>`files[].unavailable` | string | Optional output; no caller default | Safe reason the published physical location cannot currently be resolved. |

### dataset {#adminresult-dataset-model}

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="adminresult-dataset-metadata"></span>`dataset.metadata` | [DatasetManifest](../results.md#datasetmanifest) | Optional output; no caller default | Only application labels and annotations; reserved runtime keys are protected. |
| <span id="adminresult-dataset-location"></span>`dataset.location` | string | Optional output; no caller default | Published physical path or S3 URI when safely available; staging locations are not exposed. |
| <span id="adminresult-dataset-available"></span>`dataset.available` | boolean | Optional output; no caller default | Whether current engine observation is available. |

## AdminPreview {#adminpreview}

Bounded JSON/text preview returned to an administrator; text must be displayed escaped.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="adminpreview-text"></span>`text` | string | Present; no caller default | Bounded UTF-8 result preview; display as escaped text. |
| <span id="adminpreview-truncated"></span>`truncated` | boolean | Present; no caller default | True when the preview reached its size bound. |
| <span id="adminpreview-validjson"></span>`validJSON` | boolean | Present; no caller default | Whether the bounded preview parses as JSON. |
| <span id="adminpreview-maxbytes"></span>`maxBytes` | integer | Present; no caller default | Maximum bytes used to produce this bounded preview. |

## AdminAudit {#adminaudit}

Output audit receipt for an administrator action, including queue moves.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="adminaudit-id"></span>`id` | string | Present; no caller default | Opaque server/resource identity; preserve it verbatim. |
| <span id="adminaudit-actor"></span>`actor` | string | Present; no caller default | Authenticated administrator identity recorded for this action/session. |
| <span id="adminaudit-action"></span>`action` | string | Present; no caller default | Requested administrator action or audit action name. |
| <span id="adminaudit-owner"></span>`owner` | string | Optional output; no caller default | Authenticated application owner; supplied by the server, not application request JSON. |
| <span id="adminaudit-contextid"></span>`contextId` | string | Optional output; no caller default | Opaque immutable context-version ID, not the logical context name. |
| <span id="adminaudit-jobid"></span>`jobId` | string | Present; no caller default | Public persisted job ID. |
| <span id="adminaudit-replayedfrom"></span>`replayedFrom` | string | Optional output; no caller default | Optional source public job ID recorded for an administrator replay. |
| <span id="adminaudit-outcome"></span>`outcome` | string | Present; no caller default | Per-action/per-item outcome; inspect the structured error for failures. |
| <span id="adminaudit-acceptedat"></span>`acceptedAt` | string (date-time) | Present; no caller default | Server-recorded administrator action acceptance time (RFC 3339). |
| <span id="adminaudit-sourcecontextid"></span>`sourceContextId` | string | Optional output; no caller default | Source context for a move. contextId identifies the destination. |

## AdminJobPage {#adminjobpage}

Paginated administrator output; items follow the named observation/receipt model.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="adminjobpage-items"></span>`items` | array of [AdminJob](admin.md#adminjob) | Present; no caller default | Items in this page/receipt; follow the linked item type. |
| <span id="adminjobpage-nextcursor"></span>`nextCursor` | string | Optional output; no caller default | Opaque continuation token. Omission/empty value means no further page. |

## AdminAuditPage {#adminauditpage}

Paginated administrator output; items follow the named observation/receipt model.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="adminauditpage-items"></span>`items` | array of [AdminAudit](admin.md#adminaudit) | Present; no caller default | Items in this page/receipt; follow the linked item type. |
| <span id="adminauditpage-nextcursor"></span>`nextCursor` | string | Optional output; no caller default | Opaque continuation token. Omission/empty value means no further page. |

## AdminJobSelection {#adminjobselection}

Input selection containing a public job ID and its expected source context version.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="adminjobselection-id"></span>`id` | string | Required; no default | Opaque server/resource identity; preserve it verbatim. Minimum length: 1. Maximum length: 256. |
| <span id="adminjobselection-contextid"></span>`contextId` | string | Required; no default | Opaque immutable context-version ID, not the logical context name. Minimum length: 1. Maximum length: 256. |

## AdminBatchRequest {#adminbatchrequest}

Operator input for batch cancel or move.

Move requires targetContextId; cancel forbids it. IDs must be unique. Each contextId is the expected source context.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="adminbatchrequest-action"></span>`action` | string | Required; no default | Requested administrator action or audit action name. Values: `cancel`, `move`. |
| <span id="adminbatchrequest-jobs"></span>`jobs` | array of [AdminJobSelection](admin.md#adminjobselection) | Required; no default | Selected public IDs with expected source context versions; 1–200 unique IDs. Minimum items: 1. Maximum items: 200. |
| <span id="adminbatchrequest-targetcontextid"></span>`targetContextId` | string | Optional; omitted | Required destination for move; forbidden for cancel. Minimum length: 1. Maximum length: 256. |

## AdminBatchItem {#adminbatchitem}

One output outcome in a valid batch; inspect error or job for the individual item.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="adminbatchitem-id"></span>`id` | string | Present; no caller default | Opaque server/resource identity; preserve it verbatim. |
| <span id="adminbatchitem-outcome"></span>`outcome` | string | Present; no caller default | Per-action/per-item outcome; inspect the structured error for failures. Values: `moved`, `cancelled`, `cancelling`, `unchanged`, `failed`. |
| <span id="adminbatchitem-job"></span>`job` | [AdminBatchJob](admin.md#adminbatchjob) | Optional output; no caller default | Job assignment or compact action result; follow its linked model. |
| <span id="adminbatchitem-error"></span>`error` | object ([fields](#adminbatchitem-error-model)) | Optional output; no caller default | Optional safe per-item code/message; no partial payload copy. |

### error {#adminbatchitem-error-model}

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="adminbatchitem-error-code"></span>`error.code` | string | Present; no caller default | Machine-readable error or failure code; do not branch on message text. |
| <span id="adminbatchitem-error-message"></span>`error.message` | string | Present; no caller default | Human-readable detail; progress messages are at most 4096 bytes. |

## AdminBatchResult {#adminbatchresult}

Output receipt for a valid batch action.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="adminbatchresult-items"></span>`items` | array of [AdminBatchItem](admin.md#adminbatchitem) | Present; no caller default | Items in this page/receipt; follow the linked item type. Maximum items: 200. |

## AdminBatchJob {#adminbatchjob}

Compact output job state after a batch action, without the payload.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="adminbatchjob-id"></span>`id` | string | Present; no caller default | Opaque server/resource identity; preserve it verbatim. |
| <span id="adminbatchjob-contextid"></span>`contextId` | string | Present; no caller default | Opaque immutable context-version ID, not the logical context name. |
| <span id="adminbatchjob-state"></span>`state` | string | Present; no caller default | Current persisted or observed lifecycle state; see the model notes. |
| <span id="adminbatchjob-attemptcount"></span>`attemptCount` | integer | Present; no caller default | Number of persisted execution attempts, including lost attempts. Minimum: 0. |
| <span id="adminbatchjob-updatedat"></span>`updatedAt` | string (date-time) | Present; no caller default | Last durable job state/update instant (RFC 3339). |
