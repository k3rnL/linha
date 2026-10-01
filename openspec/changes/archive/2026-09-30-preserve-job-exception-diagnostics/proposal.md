## Why
Job failures currently retain only a short message, losing the exception type, stack frames and cause chain. The reported `HANDLER_FAILED: Java heap space` cannot locate the failing code, and initialization/reporting errors can disappear from both job history and useful logs.

## What Changes
- Add optional exceptionType, stackTrace, stackTraceTruncated and phase to durable job/attempt failures.
- Capture JVM exceptions with causes and suppressed exceptions, bounded serialization, and correlated worker logs.
- Cover initialization through result publication; report secondary cleanup/reporting errors without overwriting the original failure.
- Classify observed OutOfMemoryError causes as OUT_OF_MEMORY without changing configured retry/cancellation policy.
- Preserve old failure payload compatibility, ownership checks and attempt fencing; document worker/server upgrade requirements.

## Capabilities
### New Capabilities
- `job-exception-diagnostics`: Durable, bounded and attributable exception diagnostics in failures and worker logs.
### Modified Capabilities
None; existing platform specs remain in unarchived changes.

## Impact
JVM worker SDK, Go failure wire validation/persistence, OpenAPI, worker and PostgreSQL integration tests, docs and local server image. Existing JSONB columns need no migration. Rebuild worker images to collect future traces; past missing traces cannot be reconstructed. No changes to METOC or production deployment.
