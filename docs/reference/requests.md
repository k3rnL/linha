# Requests, jobs and failures

These are the JSON objects used to accept a job and read its durable state.
Ownership is derived from the authenticated API identity. A public ID is returned
only after acceptance commits to PostgreSQL; it does not mean execution started.

```json
{"handler":"count-rows","version":1,"payload":{"size":1000},"idempotencyKey":"request-42","retry":{"maxAttempts":1,"backoffSeconds":1}}
```

The same owner, logical context name and idempotency key return the original job
when the normalized request and result policy match. Different content conflicts.
An existing receipt remains replayable after its submitting version retires.
Retries are opt-in and do not make user side effects exactly-once.


Field paths below are relative to their containing model; output fields have no caller default.

## State and retry semantics {#job-states}

| State | Meaning |
| --- | --- |
| `QUEUED` | Accepted durably; waiting for a compatible worker |
| `RETRYING` | Previous attempt failed/lost and an accepted retry remains; may be in backoff |
| `RUNNING` | A worker owns the current attempt under a lease |
| `CANCELLING` | Cancellation intent committed; completion is fenced out |
| `SUCCEEDED` | Verified result publication committed |
| `FAILED` | Terminal failure, deadline or exhausted retry policy |
| `CANCELLED` | Terminal cancellation |

Queued/retrying cancellation is immediate. Running cancellation commits intent;
the worker observes loss of execution authority, interrupts its handler and
cancels the attempt's engine job group. Completion and cancellation serialize,
so whichever commits first determines the allowed final transition. Terminal
cancel requests return the retained state.

After lease/deadline loss, recovery marks an attempt lost and applies the
accepted retry policy. A driver replacement alone does not guarantee a retry.
Unsupported handler/version requests fail once a live ready backend establishes
that no matching capability exists. Progress is current-attempt data, while
attempt history remains available across server and worker restarts.

## Attempt {#attempt}

Output items from `GET /v1/jobs/{job}/attempts`. This endpoint returns
`{"items":[...]}` without pagination. It records executions, including lost and
failed attempts; retries retain the original job ID.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="attempt-id"></span>`id` | string | Output | Opaque attempt identity |
| <span id="attempt-number"></span>`number` | integer | Output | Attempt sequence, starting at 1 |
| <span id="attempt-state"></span>`state` | string | Output | RUNNING, SUCCEEDED, FAILED, LOST or CANCELLED |
| <span id="attempt-workerid"></span>`workerId` | string | Output | Worker that claimed this attempt |
| <span id="attempt-startedat"></span>`startedAt` | RFC 3339 string | Output | Claim/start time |
| <span id="attempt-finishedat"></span>`finishedAt` | RFC 3339 string | Optional output | Terminal attempt time |
| <span id="attempt-failure"></span>`failure` | [Failure](#failure) | Optional output | Persisted diagnostics for this attempt |

## SubmitRequest {#submitrequest}

Input to `POST /v1/contexts/{context}/jobs`; returned in Job.request. Requires `X-Linha-Client-Lease`.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="submitrequest-handler"></span>`handler` | string | Required; no default | Stable registered handler name; executable code is installed in the worker image. Minimum length: 1. Maximum length: 256. |
| <span id="submitrequest-version"></span>`version` | integer | Required; no default | Version of the associated protocol, handler or configuration. Minimum: 1. |
| <span id="submitrequest-payload"></span>`payload` | JSON value | Required; no default | Any valid JSON value understood by this handler; no remotely executable classes or closures. |
| <span id="submitrequest-idempotencykey"></span>`idempotencyKey` | string | Required; no default | Business acceptance key, 1–256 bytes, scoped to owner and logical context name. Minimum length: 1. Maximum length: 256. |
| <span id="submitrequest-retry"></span>`retry` | [RetryPolicy](requests.md#retrypolicy) | Optional; omitted | Bounded retry policy; omission disables retries beyond the initial attempt. |
| <span id="submitrequest-deadline"></span>`deadline` | string (date-time) | Optional; omitted | Optional absolute RFC 3339 execution deadline. Recovery also fails queued/retrying work after it passes. |

## RetryPolicy {#retrypolicy}

Input at SubmitRequest.retry. Defaults to a single total attempt.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="retrypolicy-maxattempts"></span>`maxAttempts` | integer | `1` (also for `0`) | Total attempts including the first; default 1, range 1–10. Zero selects default. Minimum: 1. Maximum: 10. |
| <span id="retrypolicy-backoffseconds"></span>`backoffSeconds` | integer | `1` (also for `0`) | Delay before eligible retry, default 1 second; range 1–3600. Zero selects default. Minimum: 1. Maximum: 3600. |

## Job {#job}

Owner-scoped output from acceptance, status, cancellation and listing.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="job-id"></span>`id` | string | Present; no caller default | Opaque server/resource identity; preserve it verbatim. |
| <span id="job-contextid"></span>`contextId` | string | Present; no caller default | Opaque immutable context-version ID, not the logical context name. |
| <span id="job-request"></span>`request` | [SubmitRequest](requests.md#submitrequest) | Present; no caller default | Original accepted SubmitRequest; payload and retry policy remain durable. |
| <span id="job-state"></span>`state` | string | Present; no caller default | QUEUED, RETRYING, RUNNING, CANCELLING, SUCCEEDED, FAILED or CANCELLED. See state semantics below. Values: `QUEUED`, `RUNNING`, `RETRYING`, `CANCELLING`, `CANCELLED`, `FAILED`, `SUCCEEDED`. |
| <span id="job-submittedat"></span>`submittedAt` | string (date-time) | Present; no caller default | Original server acceptance instant, retained across retry and queue movement (RFC 3339). |
| <span id="job-updatedat"></span>`updatedAt` | string (date-time) | Present; no caller default | Last durable job state/update instant (RFC 3339). |
| <span id="job-attemptcount"></span>`attemptCount` | integer | Present; no caller default | Number of persisted execution attempts, including lost attempts. |
| <span id="job-progress"></span>`progress` | [Progress](requests.md#progress) | Optional output; no caller default | Optional current-attempt progress. Cleared when a new attempt is claimed. |
| <span id="job-failure"></span>`failure` | [Failure](requests.md#failure) | Optional output; no caller default | Persisted execution diagnostics, when present. |
| <span id="job-result"></span>`result` | [Result](results.md#result) | Optional output; no caller default | Only successfully published metadata; absent before success. Expiry does not remove the public ID. |

## Progress {#progress}

Worker input on renew; output as Job.progress for the current attempt.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="progress-fraction"></span>`fraction` | number | Required; no default | Finite number between 0 and 1 inclusive. Minimum: 0. Maximum: 1. |
| <span id="progress-message"></span>`message` | string | Required; no default | Human-readable detail; progress messages are at most 4096 bytes. Maximum length: 4096. |

## Failure {#failure}

Worker input on failure; persisted output in Job.failure and attempt history.

Durable job or attempt failure. Diagnostic fields are optional for legacy workers or process loss; reporting remains fenced by the current attempt. Observed JVM OutOfMemoryError causes use OUT_OF_MEMORY.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="failure-code"></span>`code` | string | Required; no default | Machine-readable error or failure code; do not branch on message text. Maximum length: 256. |
| <span id="failure-message"></span>`message` | string | Required; no default | Safe failure summary, at most 8192 UTF-8 bytes. Maximum length: 8192. |
| <span id="failure-retryable"></span>`retryable` | boolean | Required; no default | Failure is eligible for the accepted bounded retry policy; true does not itself cause another attempt. |
| <span id="failure-exceptiontype"></span>`exceptionType` | string | Optional; omitted | Observed exception class/type; maximum 1024 UTF-8 bytes. Maximum length: 1024. |
| <span id="failure-stacktrace"></span>`stackTrace` | string | Optional; omitted | Plain text traceback including causes and suppressed exceptions, when available. Maximum 65536 UTF-8 bytes. Maximum length: 65536. |
| <span id="failure-stacktracetruncated"></span>`stackTraceTruncated` | boolean | Optional; omitted | True if the worker shortened the trace. Optional on legacy failures. |
| <span id="failure-phase"></span>`phase` | string | Optional; omitted | Execution phase: initialization, engine, handler, result or completion. Values: `initialization`, `engine`, `handler`, `result`, `completion`. |

## JobPage {#jobpage}

Output of `GET /v1/jobs`. Default limit 50; maximum 200.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="jobpage-items"></span>`items` | array of [Job](requests.md#job) | Present; no caller default | Items in this page/receipt; follow the linked item type. |
| <span id="jobpage-nextcursor"></span>`nextCursor` | string | Optional output; no caller default | Opaque continuation token. Omission/empty value means no further page. |
