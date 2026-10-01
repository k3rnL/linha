## Context

The worker Role grants delete on Pods, Services, ConfigMaps and PVCs. Kubernetes
requires a separate deletecollection verb for Spark's label-selector DELETEs.

## Goals / Non-Goals

Goals: permit executor Pod/ConfigMap cleanup within the existing trusted namespace.
Non-goals: cluster-wide permissions, extra server permissions, new cleanup jobs,
result deletion or changing existing worker management verbs.

## Decisions

Add a separate worker Role rule with resources [pods, configmaps] and only
[deletecollection]. Adding the verb to the combined rule would also grant it on
Services and PVCs, which this fix does not require. Keep namespace-only behavior
and existing RoleBinding identity. Helm updates apply to running service accounts.

## Risks / Trade-offs

RBAC does not constrain this grant to a label selector. Retain the documented
trusted-image/namespace boundary; Spark supplies its application/executor labels.
Validate allowed and denied collection operations against an isolated API server.

## Migration Plan

Upgrade the existing Helm release using the existing values. No image changes or
forced Pod restart. Helm rollback removes the added permission. Do not execute
manual cleanup against production as part of this change.
