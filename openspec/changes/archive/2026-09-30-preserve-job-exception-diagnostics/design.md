## Context
Failures are JSONB on jobs and attempts and use strict HTTP decoding. The worker currently keeps only getMessage and logs only a class name. Several initialization steps are outside its reporting try/catch. See proposal.md for the reported production symptom.

## Goals / Non-Goals
Capture actionable future failures across initialization, engine creation, handler execution and result publication. Do not diagnose the business-code allocation from the old message, change memory settings, manufacture missing traces, or alter result publication/attempt ownership.

## Decisions
- Extend Failure with optional exceptionType, stackTrace, stackTraceTruncated and phase. Keep the existing code/message/retryable shape for old workers. String traces preserve standard JVM cause/suppressed formatting without adding engine-specific types or reconstructing executable exception objects.
- Server validates UTF-8 byte bounds: message 8192, exceptionType 1024, stackTrace 65536; phases are initialization, engine, handler, result and completion. SDK capture stops after 16384 UTF-16 code units (always below the wire byte cap), marks truncation and never builds an unbounded intermediate trace. Message truncation respects UTF-8 bytes.
- Include full exception output in worker stderr with job/attempt/worker/handler and phase. Secondary failure-report and cleanup errors are logged with correlation, while cleanup cannot replace the primary report. Do not dump job payloads or credentials into the diagnostic envelope.
- Move attempt initialization inside the authoritative reporting boundary. Detect OutOfMemoryError in a bounded, cycle-safe cause chain as OUT_OF_MEMORY; preserve retry opt-in and cancellation/deadline authority. Fatal JVM failures remain best-effort and are rethrown after reporting.
- Existing failure JSONB persistence stores all fields on the attempt and current job. Deadline classification can replace the job's summary, but retains the actual worker exception on attempt history. Stale workers cannot overwrite newer failures; ownership checks apply to both endpoints. No acknowledgement or result publication semantics change.

## Risks / Trade-offs
- A process killed by Kubernetes or a JVM unable to allocate diagnostic data cannot report a stack trace; preserve lease-loss behavior and document this limit.
- Exception text may contain application details; retain existing owner isolation and do not add payload logging.
- Older strict servers reject added fields; upgrade server before rebuilding worker images. Previously stored failures remain readable but cannot gain historical frames.

## Migration Plan
No schema migration. Build updated server and JVM artifacts; deploy the server before adopting the worker SDK. Verify nested/suppressed errors, initialization failures, bounded Unicode messages, OOM classification, stale attempts, and retrieval after server restart. Production deployment is outside this task.
