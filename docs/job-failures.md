# Diagnose failed jobs and authentication

Use the saved job ID to inspect status and every persisted attempt. These records
survive server and worker restarts. A final job summary may differ from an earlier
failed attempt when retries or deadlines are involved.

## Inspect the failure

Fetch `GET /v1/jobs/{job}` and `GET /v1/jobs/{job}/attempts`, or call
`handle.status()` and `handle.attempts()`. An illustrative JVM failure is:

```json
{
  "code": "OUT_OF_MEMORY",
  "message": "Java heap space",
  "retryable": false,
  "exceptionType": "java.lang.OutOfMemoryError",
  "phase": "handler",
  "stackTrace": "java.lang.OutOfMemoryError: Java heap space\n\tat example.ReportJob.execute(ReportJob.scala:42)\n...",
  "stackTraceTruncated": false
}
```

`exceptionType` is the caught exception. The trace includes ordinary `Caused by`
and `Suppressed` sections; a Spark wrapper retains its underlying cause chain.
Phases are `initialization`, `engine`, `handler`, `result` and `completion`.
The code `OUT_OF_MEMORY` is based on an observed error type, not message matching.

## Find the worker evidence

Search worker logs by job ID. Error headers include job, attempt, worker,
handler/version and phase; the full exception follows. Failure-reporting and
cleanup errors are correlated separately and do not replace the original failure.

If a Pod was killed, inspect its termination state and executor logs. Kubernetes
OOM kills and JVM crashes may prevent any trace from being sent. The attempt still
follows lease-loss recovery. Verify JVM heap plus overhead in
[Spark resource settings](reference/backend/spark.md) rather than setting
container memory equal to the heap.

## Decide whether to retry

Execution retries default to one total attempt. Set `maxAttempts` when submitting
only if repeating the business operation is safe. A driver replacement does not
by itself opt a request into retry. Cancellation takes precedence when its intent
is committed before completion. Idempotency protects duplicate acceptance, not
all external business effects.

## Diagnose OAuth and access failures

| Symptom | Check |
| --- | --- |
| SDK credential acquisition fails | Exact token endpoint, client authentication/service account enabled, current client secret |
| `401` from Linha | JWT signature, issuer, audience `linha`, expiry and nonempty subject |
| `403` on admin routes | Browser group/role claim and `security.admin` rules; API service credentials do not grant admin |
| TLS failure | Provider CA trust for the SDK and server independently |
| Status/result disappears after identity change | Same issuer and subject as the original owner |

The OAuth SDK reports operation/status without provider bodies or credentials.
Keep the original cause in application logs only when it is already sanitized;
do not log tokens, client secrets or full system-property dumps.

## Diagnostic limits

The wire bounds messages to 8192 UTF-8 bytes, exception types to 1024 and stack
traces to 65536. The JVM captures at most 16384 UTF-16 code units and marks
truncation. Full traces remain in worker stderr when available. Older stored
failures without diagnostic fields cannot be reconstructed retroactively.
