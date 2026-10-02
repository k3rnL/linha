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

The image embeds the [admin console](../../../docs/admin-ui.md); `ui.enabled`
defaults to false. Configure `ui.publicURL`, a browser OIDC client and explicit
viewer/operator mappings before enabling it. Database and browser credentials
use caller-managed `secretRef` plus selected key names.

Enable `ui.ingress.enabled` for chart-managed UI/admin routing. Its host is derived
from `ui.publicURL`; configure `ui.ingress.className`, `annotations` and optional
`tls.enabled`/`tls.secretName`. TLS at the ingress requires an HTTPS public URL
and a Secret in the release namespace. The fixed `/ui` and `/v1/admin` Prefix
routes use the existing Service without path rewriting. See the
[HTTPS values example](../../../docs/admin-ui.md#enable-with-oidc). This adds no
RBAC permissions, controller installation or Spark UI ingress.

[Observability](../../../docs/observability.md) documents `/metrics`, the packaged
Grafana dashboard and `metrics.serviceMonitor`/`metrics.grafanaDashboard`. Both
operator resources default to disabled and can be enabled independently. They
require their respective installed CRDs and add no Linha ClusterRole. UI and
metric export can be disabled independently.
