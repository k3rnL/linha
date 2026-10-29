# HTTP errors

Errors use an HTTP status and this JSON envelope:

```json
{"error":{"code":"CLIENT_LEASE_EXPIRED","message":"a live client lease is required; attach to this context again"}}
```

| Code / HTTP status | Meaning |
| --- | --- |
| `INVALID_ARGUMENT` / 400 | Invalid fields, bounds or conflicting options |
| `UNAUTHORIZED` / 401 | Credentials missing or invalid |
| `FORBIDDEN` / 403 | Administrator role/action denied |
| `NOT_FOUND` / 404 | Missing or foreign owner-scoped resource |
| `CLIENT_LEASE_EXPIRED` / 409 | Attach again before accepting new work |
| `STALE_ATTEMPT` / 409 | Attempt or storage lock no longer owns its work |
| `CONFLICT` / 409 | Idempotency, receipt or resource conflict |
| `RESULT_NOT_READY` / 409 | Successful publication has not happened |
| `RESULT_EXPIRED` / 410 | Payload retention ended |
| `RESULT_TOO_LARGE`, `STORAGE_TOO_LARGE` / 413 | Configured result/object byte bound exceeded |
| `STAGING_LIMIT_EXCEEDED` / 429 | Concurrent staging reservation budget exhausted |
| `DATASET_UNSUPPORTED` / 400 or 503 | Provider lacks scoped delegation or dataset repository is unavailable |
| `DESTINATION_CHANGED` / 503 | Retained destination fingerprint differs |
| `SCHEMA_MISMATCH` (SDK) | Typed result descriptor does not match; thrown locally |

Other adapter/storage errors can add codes. Inspect the structured code and HTTP
status, not message text. [Failure](../requests.md#failure) is persisted execution
diagnostics; it is different from a transport error envelope.


Field paths below are relative to their containing model; output fields have no caller default.

## Error {#error}

Output error envelope for failed HTTP operations; SDKs expose its code and message in LinhaException.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="error-error"></span>`error` | object ([fields](#error-error-model)) | Present; no caller default | Structured per-request or per-item error code and safe message. |

### error {#error-error-model}

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="error-error-code"></span>`error.code` | string | Present; no caller default | Machine-readable error or failure code; do not branch on message text. |
| <span id="error-error-message"></span>`error.message` | string | Present; no caller default | Human-readable detail; progress messages are at most 4096 bytes. |
