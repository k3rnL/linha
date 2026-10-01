# Linha Helm chart

Deploys the Go server with two replicas by default, namespaced worker management,
services, health probes and disruption controls. PostgreSQL and result storage
are external persistent dependencies. See [operations](../../../docs/operations.md)
for database Secret keys, security/OIDC and namespace-only RBAC.

Use `workerPodTemplates.driverPodTemplate` and `executorPodTemplate` for native
Pod defaults. Client ensure requests can customize both; effective templates are
persisted for replacement. `registrySecrets.allowedNames` grants the server get
access to only selected registry Secrets. Top-level `imagePullSecrets` apply to
server Pods and become worker defaults. See the
[template guide](../../../docs/pod-templates.md) for examples and merge rules.

`workerVolumes`/`workerMounts` remain supported as legacy defaults. The new chart
adds namespaced ConfigMap get/create permissions for Spark executor templates;
upgrade the chart alongside the server. No ClusterRole is required when
`rbac.namespaceOnly: true`.

Spark cleanup also requires namespaced `deletecollection` on Pods and ConfigMaps.
The chart grants it only to the worker Role. Update the Helm release to correct
older installations; no server/worker image rebuild is needed. See
[operations](../../../docs/operations.md) for verification commands.
