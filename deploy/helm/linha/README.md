# Linha Helm chart

Deploys the Go server with two replicas by default, a Service, probes,
namespace-scoped engine management and worker permissions. PostgreSQL, result
storage, the Spark Operator and optional monitoring operators are external.

```sh
helm upgrade --install linha oci://ghcr.io/k3rnl/charts/linha \
  --version 0.1.4 --namespace linha --create-namespace --values values.yaml
```

The OCI chart requires Helm 3.8 or newer and no registry login. Download it with
`helm pull oci://ghcr.io/k3rnl/charts/linha --version 0.1.4`.
For a source checkout, use `deploy/helm/linha` as the chart argument instead and
omit `--version`.

Create credential Secrets before installation. Database settings use
`database.postgres.secretRef`, `usernameKey` and `passwordKey`; S3 uses
`s3.secretRef` and `destinationsKey`. Configure permitted worker repositories.

`security.api.jwt` validates application access tokens. `security.oidc` and
`ui.oidc` configure browser login; `security.admin` maps verified groups to
viewer/operator access. `security.enabled: false` disables all authentication.

`rbac.namespaceOnly: true` is the default and requires no Linha ClusterRole.
Driver/executor resources and custom mounts belong in backend Spark settings
and Pod templates. The UI, its ingress, ServiceMonitor and GrafanaDashboard are
independently optional.

- [Deploy Linha](../../../docs/operations.md)
- [Configuration reference](../../../docs/configuration.md)
- [Pod templates](../../../docs/pod-templates.md)
- [Administrator console](../../../docs/admin-ui.md)
- [Prometheus and Grafana](../../../docs/observability.md)

Use matching chart and server versions. This chart defaults to the published
`ghcr.io/k3rnl/linha/server:0.1.4` image.
