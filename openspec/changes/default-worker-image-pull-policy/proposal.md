# Proposal

## Why

Spark on Kubernetes applies one shared image pull policy to both driver and executor containers, overriding values supplied only through Pod templates. The deployment needs a separate setting that reaches Spark's effective configuration so the requested policy is used by actual Pods.

## What Changes

- Add one Helm setting, `workerPodPullPolicy`, defaulting to `Always` for both
  worker roles.
- Pass the policy through Spark's shared pull-policy configuration and
  SparkApplication field so Spark does not replace it with `IfNotPresent`.
- Preserve a common explicit policy from administrator or client templates;
  reject conflicting driver/executor policies and retain the resolved value in
  accepted context snapshots.
- Add regression coverage and document the setting and precedence.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `client-pod-customization`: specify the shared worker policy, template
  override rules, and context snapshot behavior.

## Impact

The Helm chart, server configuration, and SparkApplication creation gain the
shared setting. New contexts persist the resolved policy; existing contexts
retain their saved templates. No public job API or dependency changes are
required.
