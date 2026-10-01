## Context

See proposal.md. Linha creates drivers directly; Spark 3.5.6 creates executors
from the `LINHA_SPARK_CONF` supplied by the adapter. The existing driver has a
memory request/limit but only a CPU request. Spark executor cores determine task
parallelism and default CPU requests, while the Kubernetes CPU limit is separate.

## Goals / Non-Goals

Goals: consistently enforce the existing integer core budget with minimal changes.
Non-goals: independently configurable request/limit fields, new memory overhead
settings, automatic disruption of live drivers or changes to generic request handling.

## Decisions

- Set driver resources.limits.cpu directly because Spark runs in client mode and
  does not construct the Linha driver Pod. Use the same configured count as requests.
- Explicitly set both spark.kubernetes.executor.request.cores and
  spark.kubernetes.executor.limit.cores alongside spark.executor.cores. Explicit
  settings work with both native executor templates and dynamic allocation.
- Keep memory mapping: driver heap plus 512 MiB overhead in its Pod; executor heap
  plus Spark's existing overhead accounting. Memory already has requests and limits.
- Preserve input normalization and stored specs. A server upgrade changes how new
  Pods are rendered, without making existing context names conflict.

## Risks / Trade-offs

- CPU limits cap previous opportunistic CPU bursts; this enforces the client's
  chosen budget. Separate bursting/request controls can be designed independently.
- Existing Pods keep old resources until replaced. Document this and do not delete
  or restart production workloads during the implementation.

## Migration Plan

Deploy the rebuilt server image. New contexts and newly provisioned drivers use
the corrected limits. Existing running contexts adopt them when their drivers are
replaced; submit work on a new context for an isolated rollout. No schema, Helm
permission, SDK or worker image changes are required. Rollback changes future
provisioning only and does not rewrite existing Pods.
