# Design

## Context

The chart currently leaves pull policies to Kubernetes/Spark defaults. The
server receives administrator Pod templates as JSON, merges them with each
client's requested templates before accepting a context, and persists the
effective templates with the context version. Provisioning already applies
driver template fields to managed Pods and executor template fields to Spark
configuration or SparkApplication templates.

## Goals / Non-Goals

**Goals:**
- Make Helm-created new contexts use one shared worker image pull policy,
  defaulting to `Always`.
- Preserve existing per-context overrides and immutable context snapshots.
- Verify the rendered configuration and Spark runtime propagation.

**Non-Goals:**
- Change policies for the Linha server container or user-added sidecars.
- Modify existing context versions after a chart upgrade.

## Decisions

- Add `workerPodPullPolicy` in Helm values and pass it to the server through
  `LINHA_WORKER_POD_PULL_POLICY`. Restrict values to Kubernetes' `Always`,
  `IfNotPresent`, and `Never` options.
- Resolve the common policy after administrator and client templates have been
  merged. A policy explicitly set in either template overrides the deployment
  default for both roles; reject the request if both templates set different
  policies.
- Persist the resolved policy in both effective main-container templates. Set
  `SparkApplication.spec.imagePullPolicy` and the shared
  `spark.kubernetes.container.image.pullPolicy` configuration from that value,
  because Spark overwrites Pod-template policies with its shared setting. The
  SparkApplication field also controls Spark-managed init containers.
- Add chart-render and engine regression tests. Cover the rendered setting,
  resolved context and executor policies, and the SparkApplication shared field.
- Document the setting, valid values, precedence, and new-context scope.

## Risks / Trade-offs

- `Always` causes Kubernetes to check the registry whenever a worker Pod starts;
  startup can fail if the registry or pull credentials are unavailable. Existing
  registry Secret configuration and immutable image resolution remain in effect.
- Chart upgrades do not rewrite policies already snapshotted into context
  versions. This preserves restart behavior; users wanting the new policy for
  an existing context must create a new effective context version.

## Migration Plan

Upgrade the Helm release normally. New contexts use `workerPodPullPolicy`
unless administrator or client templates specify a common policy. Existing
contexts and replacements keep their persisted policies. To change the default
for future contexts, set `workerPodPullPolicy` in Helm values and upgrade again.
