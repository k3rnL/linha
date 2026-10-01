## Why

Applications need their own configuration files, credentials, scheduling settings and
supporting containers in reusable Spark backends. Today Linha creates fixed Pods and
only deployment-wide mounts can reach them, which prevents client-specific setup.

## What Changes

- Accept native Kubernetes Pod template fragments for drivers and executors in Spark engine settings.
- Merge deployment defaults with client settings and persist both requested identity and effective configuration for repeat ensure and replacement.
- Apply driver templates directly and provide durable executor templates to Spark's native template mechanism.
- Preserve Linha/Spark lifecycle fields with precise validation; support application ConfigMap/Secret/PVC references, pull secrets, environment, scheduling, init containers and sidecars.
- Use authorized namespace-local registry Secrets for image digest resolution as well as Pod pulls.
- Add Scala JSON/YAML loading and immutable configuration helpers over the same JSON protocol.
- Extend namespace-scoped Helm permissions, configuration, examples and validation.

## Capabilities

### New Capabilities

- `client-pod-customization`: Durable per-context Pod templates, managed-field validation, registry credentials, deployment defaults and Scala helpers.

### Modified Capabilities

None in the main spec store. This extends the in-progress establish-linha-platform
foundation without completing its unrelated outstanding tasks.

## Impact

Spark adapter and controller image-resolution interface, context persistence migration,
Go HTTP normalization, JVM client SDK, OpenAPI, Helm Role permissions and documentation.
Request scheduling and job/result ownership remain engine-independent. Referenced
application Secrets/ConfigMaps/PVCs remain caller-managed. No METOC files are changed.
