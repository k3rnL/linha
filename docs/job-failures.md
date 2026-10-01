# Debugging job failures

`GET /v1/jobs/{jobId}` returns a `failure` with the original message and, for
updated JVM workers, diagnostic fields. This is an illustrative example, not a
reconstruction of a previously stored error:

```json
{
  "code": "OUT_OF_MEMORY",
  "message": "Java heap space",
  "retryable": false,
  "exceptionType": "java.lang.OutOfMemoryError",
  "phase": "handler",
  "stackTrace": "java.lang.OutOfMemoryError: Java heap space\n\tat example.TrackJob.execute(TrackJob.scala:42)\n...",
  "stackTraceTruncated": false
}
```

`exceptionType` is the exception actually caught by the worker. `stackTrace`
includes standard JVM `Caused by:` and `Suppressed:` sections. A wrapper such as
SparkException keeps its own type and trace, with its underlying exception in
the cause chain. An observed OutOfMemoryError in that chain uses the code
`OUT_OF_MEMORY`; memory exhaustion is not inferred from message text alone.

`phase` identifies `initialization`, `engine`, `handler`, `result`, or
`completion`. The handler phase includes result helpers called directly inside
business code; result/completion cover SDK processing after the handler returns.
Linha HTTP error codes remain intact. Generic initialization, engine, result and
publication failures get corresponding codes rather than all being handler errors.

Use `GET /v1/jobs/{jobId}/attempts` for **each attempt's** exception, especially
when a request retries or its final job summary reports a deadline. Job and
attempt diagnostics remain in PostgreSQL across server/worker restarts and
backend shutdown. Existing ownership checks apply to both endpoints. Stale
workers cannot overwrite newer attempts.

Worker stderr also prints the complete exception with `jobId`, `attemptId`,
`workerId`, handler/version and phase. Search by job ID to find it. Errors during
failure reporting or cleanup are logged with the same IDs; they do not replace
the original report. User payloads are not added to those log headers.

Wire limits are 8192 UTF-8 bytes for the message, 1024 for exceptionType and 65536
for stackTrace. The JVM SDK stops trace capture at 16384 UTF-16 code units to
bound allocation and wire size, sets `stackTraceTruncated: true`, and includes a
truncation marker. Full exception output remains in worker logs. Exceptions that
disable stack capture retain their original diagnostic text; Linha does not
invent missing original frames.

## Upgrade and limitations

Upgrade the server first, then rebuild worker images with the updated
`io.linha:linha-worker_2.12:0.1.0-SNAPSHOT` SDK (including through
`linha-spark_2.12`). This change needs no database migration. Old workers and
old failures containing only code/message/retryable remain supported by the new
server. Old strict servers reject the new worker's additional fields.

A server-only upgrade cannot recover stack traces that the old worker discarded.
Rerun the job with the rebuilt worker to capture its exception. The supplied
`Java heap space` message indicates a heap-allocation failure, but its old record
cannot identify the failing line or distinguish the original exception from a
wrapper. This change improves evidence; it does not change heap configuration or
fix the underlying allocation.

Reporting is best-effort if the JVM has exhausted memory. A process killed by
Kubernetes or an unrecoverable JVM crash cannot send a trace; its attempt still
follows existing lease-loss recovery. Inspect Pod termination details and retained
worker/executor logs for those cases. Retry policy and cancellation authority
remain unchanged; a captured error does not imply exactly-once business effects.
