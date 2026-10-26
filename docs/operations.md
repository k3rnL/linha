# Deploy Linha on Kubernetes

This guide installs an HA API service with managed Spark backends. It assumes you
already operate Kubernetes, Helm, PostgreSQL and either S3 or a shared persistent
filesystem.

## 1. Prepare the dependencies

Create a namespace for Linha and its workers. Install Kubeflow Spark Operator 2.4.0
separately, with `sparkoperator.k8s.io/v1beta2` SparkApplication support and the
operator watching this namespace. Linha's chart does not install the operator or
its CRDs. Namespace-only Linha RBAC does not change the operator's own permissions.

Provision a durable PostgreSQL service and result storage. All server replicas
must use the same database. For local results, every server, driver and executor
must see the same persistent filesystem at the configured root.

## 2. Create database and storage Secrets

Provide a Secret in the release namespace with separate `username` and `password`
keys. Set its name through `database.postgres.secretRef`. Credentials are URI-encoded
by the server; do not manually escape characters in the Secret values.

For S3, provide a Secret key containing destination JSON:

```json
{
  "analytics": {
    "bucket": "example-results",
    "prefix": "linha",
    "region": "eu-west-1"
  }
}
```

Optional `endpoint` and `pathStyle: true` support S3-compatible stores. Server
credentials use the AWS default credential chain. Callers select a destination
alias and path policy; storage credentials do not travel in job JSON.

## 3. Select a matching image

Use the server image matching your chart and documentation version.

<!-- development-only:start -->
The example below selects a source-built image. For a published release, select
`ghcr.io/k3rnl/linha/server` and its matching release tag instead.
<!-- development-only:end -->

## 4. Create a values file

```yaml
replicaCount: 2
image:
  repository: registry.example/linha/server
  tag: development
database:
  postgres:
    host: postgres.example.internal
    database: linha
    sslMode: verify-full
    secretRef: linha-postgres
    usernameKey: username
    passwordKey: password
security:
  enabled: true
  api:
    enabled: true
    jwt:
      issuer: https://identity.example/realms/jobs
      audience: linha
  oidc:
    enabled: false
rbac:
  namespaceOnly: true
allowedImages:
  - registry.example/workers
s3:
  enabled: true
  secretRef: linha-results
  destinationsKey: destinations
```

`allowedImages` must include each permitted worker repository prefix. Use a broad
allow-list only when all callers and images share the deployment's trust boundary.
Configure private registry Secret references through
[Pod templates](pod-templates.md).

The chart derives the internal worker server URL from its Service. Applications
use the Service address or a separately exposed API route. The UI ingress exposes
only UI/admin routes and is not a public job API ingress.

## 5. Install and verify

Install the published OCI chart with Helm 3.8 or newer. It is public and requires
no registry login or `helm repo add`:

```sh
helm upgrade --install linha oci://ghcr.io/k3rnl/charts/linha \
  --version 0.1.3 --namespace linha --create-namespace --values values.yaml
kubectl -n linha rollout status deployment/linha-linha
kubectl -n linha get pods
kubectl -n linha port-forward service/linha-linha 8080:8080
```

To download the archive without installing it:

```sh
helm pull oci://ghcr.io/k3rnl/charts/linha --version 0.1.3
```

From a source checkout, use `deploy/helm/linha` as the chart argument instead of
the OCI reference and omit `--version`. Keep chart and server image versions aligned.

In another terminal, check readiness:

```sh
curl --fail http://localhost:8080/readyz
```

Readiness checks PostgreSQL and configured local storage. Liveness checks the
process. Inspect server logs if readiness or startup fails.

## 6. Configure clients and Spark

Use [OAuth client credentials](client-authentication.md) for application clients.
Ensure a backend with your worker image and
[Spark application settings](versioned-spark-applications.md). Put handler code,
matching SDKs and any required connectors in that image.

Add [the administrator console](admin-ui.md) and
[Prometheus/Grafana integration](observability.md) when needed.

## Private certificates

For PostgreSQL, mount its CA and set `database.postgres.sslRootCert`.
For the identity provider, mount a CA directory and extend the system trust path:

```yaml
extraEnv:
  - name: SSL_CERT_DIR
    value: /etc/ssl/certs:/etc/linha/identity-ca
extraVolumes:
  - name: identity-ca
    configMap:
      name: linha-identity-ca
extraVolumeMounts:
  - name: identity-ca
    mountPath: /etc/linha/identity-ca
    readOnly: true
```

The ConfigMap must contain PEM certificate files. Roll the server after changing
its CA files. Keep `/etc/ssl/certs` in the path to retain public roots.

The explicit `security.api.jwt.tls.insecureSkipVerify` and
`security.oidc.tls.insecureSkipVerify` switches bypass provider certificate and
hostname verification independently. They retain JWT checks but permit provider
impersonation; trusting the CA preserves the full verification boundary. Browser
certificate trust is independent of these server options.

## Namespace-only worker permissions

The default `rbac.namespaceOnly: true` creates Roles and RoleBindings, with no
ClusterRole. Workers authenticate through Kubernetes issuer discovery and signing
keys plus live Pod/service-account checks. A cluster that removed the default
service-account discovery access must restore it or choose
`rbac.namespaceOnly: false` to use TokenReview. Resource management stays in the
release namespace in both cases.

Spark cleanup needs `deletecollection` on Pods and ConfigMaps. Verify the worker
Role if Spark reports a collection-delete permission error:

```sh
kubectl -n linha auth can-i deletecollection pods   --as=system:serviceaccount:linha:linha-linha-worker
kubectl -n linha auth can-i deletecollection configmaps   --as=system:serviceaccount:linha:linha-linha-worker
```

Both should return `yes`; the caller needs permission to impersonate the account.

## Upgrade and recover

Back up PostgreSQL before an upgrade. Deploy a matching server, chart and SDK set
when configuration or protocol changes require it. Database migrations are
serialized. Older binaries can reject a newer schema; do not start an older
binary against it without a compatible rollback procedure.

Retain the same result destination locations when upgrading. Changing a bucket,
prefix or endpoint under an existing destination alias can invalidate its stored
fingerprint; use a new alias for a new location. Credentials can rotate separately.

Process restarts preserve metadata, but recovery still depends on the durability
of PostgreSQL and result storage. See [architecture](architecture.md) and
[validation scope](validation.md) for the tested boundaries.
