# Helm values

Operator settings passed through a values file. Backend declarations use the
separate [backend reference](../backend/index.md). Default values below come from
the checked-in chart; nested item fields have no default unless stated. Native
Kubernetes/monitoring objects retain their upstream validation.

| Field | Type | Default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="helm-replicacount"></span>`replicaCount` | integer | `2` | API replicas sharing the database; default 2. External database/storage HA is operator-managed.  |
| <span id="helm-image"></span>`image` | object | Object; see nested fields | Server image selection. Keep chart, image and SDK versions compatible.  |
| <span id="helm-image-repository"></span>`image.repository` | string | `"ghcr.io/k3rnl/linha/server"` | Server image repository.  |
| <span id="helm-image-tag"></span>`image.tag` | string | `"0.1.4"` | Server image version; use the chart release version.  |
| <span id="helm-image-pullpolicy"></span>`image.pullPolicy` | string | `"IfNotPresent"` | Kubernetes image pull policy.  |
| <span id="helm-imagepullsecrets"></span>`imagePullSecrets` | array | `[]` | Server image pull Secret references, also permitted worker defaults.  |
| <span id="helm-database"></span>`database` | object | Object; see nested fields | Database connection configuration; database is provisioned externally.  |
| <span id="helm-database-postgres"></span>`database.postgres` | object | Object; see nested fields | PostgreSQL connection and caller-managed credential Secret.  |
| <span id="helm-database-postgres-host"></span>`database.postgres.host` | string | `"postgres"` | PostgreSQL DNS/IP hostname. Minimum length 1. |
| <span id="helm-database-postgres-port"></span>`database.postgres.port` | integer | `5432` | TCP port. Minimum 1. Maximum 65535. |
| <span id="helm-database-postgres-database"></span>`database.postgres.database` | string | `"linha"` | Existing PostgreSQL database name. Minimum length 1. |
| <span id="helm-database-postgres-sslmode"></span>`database.postgres.sslMode` | string | `"verify-full"` | TLS mode passed to pgx; verification defaults to verify-full.  Values: `disable`, `allow`, `prefer`, `require`, `verify-ca`, `verify-full`. |
| <span id="helm-database-postgres-sslrootcert"></span>`database.postgres.sslRootCert` | string | `""` | Optional mounted CA certificate path.  |
| <span id="helm-database-postgres-secretref"></span>`database.postgres.secretRef` | string | `"linha-postgres"` | Existing Secret containing database credentials. Minimum length 1. |
| <span id="helm-database-postgres-usernamekey"></span>`database.postgres.usernameKey` | string | `"username"` | Username key in that Secret. Minimum length 1. |
| <span id="helm-database-postgres-passwordkey"></span>`database.postgres.passwordKey` | string | `"password"` | Password key in that Secret. Minimum length 1. |
| <span id="helm-security"></span>`security` | object | Object; see nested fields | Independent application API, browser and worker authentication settings.  |
| <span id="helm-security-enabled"></span>`security.enabled` | boolean | `true` | Master authentication switch; false disables API/worker security and uses one shared owner.  |
| <span id="helm-security-api"></span>`security.api` | object | Object; see nested fields | Public application API identity configuration.  |
| <span id="helm-security-api-enabled"></span>`security.api.enabled` | boolean | `true` | Validate API JWTs when master security is enabled.  |
| <span id="helm-security-api-jwt"></span>`security.api.jwt` | object | Object; see nested fields | JWT discovery, signature and audience validation settings.  |
| <span id="helm-security-api-jwt-issuer"></span>`security.api.jwt.issuer` | string | `""` | Required issuer when API authentication is enabled.  |
| <span id="helm-security-api-jwt-audience"></span>`security.api.jwt.audience` | string | `"linha"` | Required intended API token audience. Minimum length 1. |
| <span id="helm-security-api-jwt-tls"></span>`security.api.jwt.tls` | object | Object; see nested fields | TLS trust settings for API identity discovery and signing-key requests.  |
| <span id="helm-security-api-jwt-tls-insecureskipverify"></span>`security.api.jwt.tls.insecureSkipVerify` | boolean | `false` | Skip API provider certificate/hostname verification only; JWT checks remain.  |
| <span id="helm-security-oidc"></span>`security.oidc` | object | Object; see nested fields | Browser administrator OIDC login; separate from application API JWTs.  |
| <span id="helm-security-oidc-enabled"></span>`security.oidc.enabled` | boolean | `true` | Enable OIDC; a secured enabled UI requires it.  |
| <span id="helm-security-oidc-issuer"></span>`security.oidc.issuer` | string | `""` | Required browser issuer for secured UI; inherited by membership rules.  |
| <span id="helm-security-oidc-tls"></span>`security.oidc.tls` | object | Object; see nested fields | TLS trust settings for browser identity operations.  |
| <span id="helm-security-oidc-tls-insecureskipverify"></span>`security.oidc.tls.insecureSkipVerify` | boolean | `false` | Skip browser provider certificate/hostname verification only; browser trust is separate.  |
| <span id="helm-security-admin"></span>`security.admin` | object | Object; see nested fields | Viewer/operator membership rules; see membership models in the storage/provider reference.  |
| <span id="helm-security-admin-viewer"></span>`security.admin.viewer` | object | Object; see nested fields | Viewer membership; rules inherit the browser issuer.  |
| <span id="helm-security-admin-viewer-subjects"></span>`security.admin.viewer.subjects` | array | `[]` | Explicit subject membership rules.  |
| <span id="helm-security-admin-viewer-subjects-issuer"></span>`security.admin.viewer.subjects[].issuer` | string | Omitted | Optional; if supplied it must equal the browser issuer.  |
| <span id="helm-security-admin-viewer-subjects-subject"></span>`security.admin.viewer.subjects[].subject` | string | Omitted | Required nonempty token subject. Minimum length 1. |
| <span id="helm-security-admin-viewer-claims"></span>`security.admin.viewer.claims` | array | `[]` | Membership by matching a nested claim.  |
| <span id="helm-security-admin-viewer-claims-issuer"></span>`security.admin.viewer.claims[].issuer` | string | Omitted | Optional; if supplied it must equal the browser issuer.  |
| <span id="helm-security-admin-viewer-claims-path"></span>`security.admin.viewer.claims[].path` | array | Omitted | 1–8 nested claim keys; each nonempty, at most 128 bytes, not __proto__.  |
| <span id="helm-security-admin-viewer-claims-values"></span>`security.admin.viewer.claims[].values` | array | Omitted | Nonempty list of allowed string values.  |
| <span id="helm-security-admin-operator"></span>`security.admin.operator` | object | Object; see nested fields | Operator membership; rules inherit the browser issuer.  |
| <span id="helm-security-admin-operator-subjects"></span>`security.admin.operator.subjects` | array | `[]` | Explicit subject membership rules.  |
| <span id="helm-security-admin-operator-subjects-issuer"></span>`security.admin.operator.subjects[].issuer` | string | Omitted | Optional; if supplied it must equal the browser issuer.  |
| <span id="helm-security-admin-operator-subjects-subject"></span>`security.admin.operator.subjects[].subject` | string | Omitted | Required nonempty token subject. Minimum length 1. |
| <span id="helm-security-admin-operator-claims"></span>`security.admin.operator.claims` | array | `[]` | Membership by matching a nested claim.  |
| <span id="helm-security-admin-operator-claims-issuer"></span>`security.admin.operator.claims[].issuer` | string | Omitted | Optional; if supplied it must equal the browser issuer.  |
| <span id="helm-security-admin-operator-claims-path"></span>`security.admin.operator.claims[].path` | array | Omitted | 1–8 nested claim keys; each nonempty, at most 128 bytes, not __proto__.  |
| <span id="helm-security-admin-operator-claims-values"></span>`security.admin.operator.claims[].values` | array | Omitted | Nonempty list of allowed string values.  |
| <span id="helm-rbac"></span>`rbac` | object | Object; see nested fields | Worker/resource permission configuration.  |
| <span id="helm-rbac-namespaceonly"></span>`rbac.namespaceOnly` | boolean | `true` | Namespaced management with projected-token verification; false adds cluster-scoped TokenReview permission.  |
| <span id="helm-allowedimages"></span>`allowedImages` | array | `[]` | Allowed worker, init-container and sidecar image repository prefixes. A nonempty allow-list is required for managed server startup.  |
| <span id="helm-local"></span>`local` | object | Object; see nested fields | Optional shared-filesystem result provider.  |
| <span id="helm-local-enabled"></span>`local.enabled` | boolean | `false` | Configure local result storage on servers.  |
| <span id="helm-local-root"></span>`local.root` | string | `"/var/lib/linha/results"` | Persistent shared root; client result root must match exactly.  |
| <span id="helm-local-existingclaim"></span>`local.existingClaim` | string | `""` | Existing server PVC; drivers/executors still need matching shared mounts.  |
| <span id="helm-s3"></span>`s3` | object | Object; see nested fields | Server S3 destination configuration.  |
| <span id="helm-s3-enabled"></span>`s3.enabled` | boolean | `false` | Load named destinations from the selected Secret.  |
| <span id="helm-s3-secretref"></span>`s3.secretRef` | string | `"linha-s3"` | Existing Secret with destination JSON; AWS credentials use the server default credential chain.  |
| <span id="helm-s3-destinationskey"></span>`s3.destinationsKey` | string | `"destinations"` | Secret key containing the destination-name → S3Config JSON object.  |
| <span id="helm-workervolumes"></span>`workerVolumes` | array | `[]` | Legacy administrator volumes snapshotted into new context templates; prefer explicit templates.  |
| <span id="helm-workermounts"></span>`workerMounts` | array | `[]` | Legacy worker mounts; PVC mounts also flow to executor templates.  |
| <span id="helm-workerpodtemplates"></span>`workerPodTemplates` | object | Object; see nested fields | Administrator Pod defaults merged with caller overrides for a new context.  |
| <span id="helm-workerpodtemplates-driverpodtemplate"></span>`workerPodTemplates.driverPodTemplate` | object | `{}` | Native Pod fragment; see the backend Pod-template reference for merge/protected-field rules.  |
| <span id="helm-workerpodtemplates-driverpodtemplate-metadata"></span>`workerPodTemplates.driverPodTemplate.metadata` | object | Omitted | Application labels/annotations only.  |
| <span id="helm-workerpodtemplates-driverpodtemplate-metadata-labels"></span>`workerPodTemplates.driverPodTemplate.metadata.labels` | object | Omitted | String label map; runtime prefixes are protected.  |
| <span id="helm-workerpodtemplates-driverpodtemplate-metadata-annotations"></span>`workerPodTemplates.driverPodTemplate.metadata.annotations` | object | Omitted | String annotation map; runtime prefixes are protected.  |
| <span id="helm-workerpodtemplates-driverpodtemplate-spec"></span>`workerPodTemplates.driverPodTemplate.spec` | object | Omitted | Kubernetes v1 PodSpec subject to Linha/Spark restrictions.  |
| <span id="helm-workerpodtemplates-executorpodtemplate"></span>`workerPodTemplates.executorPodTemplate` | object | `{}` | Native Pod fragment; see the backend Pod-template reference for merge/protected-field rules.  |
| <span id="helm-workerpodtemplates-executorpodtemplate-metadata"></span>`workerPodTemplates.executorPodTemplate.metadata` | object | Omitted | Application labels/annotations only.  |
| <span id="helm-workerpodtemplates-executorpodtemplate-metadata-labels"></span>`workerPodTemplates.executorPodTemplate.metadata.labels` | object | Omitted | String label map; runtime prefixes are protected.  |
| <span id="helm-workerpodtemplates-executorpodtemplate-metadata-annotations"></span>`workerPodTemplates.executorPodTemplate.metadata.annotations` | object | Omitted | String annotation map; runtime prefixes are protected.  |
| <span id="helm-workerpodtemplates-executorpodtemplate-spec"></span>`workerPodTemplates.executorPodTemplate.spec` | object | Omitted | Kubernetes v1 PodSpec subject to Linha/Spark restrictions.  |
| <span id="helm-registrysecrets"></span>`registrySecrets` | object | Object; see nested fields | Registry credential access policy for per-context private image resolution.  |
| <span id="helm-registrysecrets-allowednames"></span>`registrySecrets.allowedNames` | array | `[]` | Permitted Secret names; each gets namespaced get permission for primary image digest resolution.  |
| <span id="helm-results"></span>`results` | object | Object; see nested fields | Server result staging and physical-cleanup policy, not the per-backend ResultPolicy.  |
| <span id="helm-results-cleanup"></span>`results.cleanup` | object | Object; see nested fields | Physical deletion policy for tracked allocations only.  |
| <span id="helm-results-cleanup-enabled"></span>`results.cleanup.enabled` | boolean | `false` | Opt in to eligible terminal/expired allocation deletion after writer expiry and grace; job history remains.  |
| <span id="helm-results-maxstagingbytes"></span>`results.maxStagingBytes` | integer | `5368709120` | Concurrent server staging reservation budget in bytes; default 5 GiB. Minimum 1. |
| <span id="helm-extraenv"></span>`extraEnv` | array | `[]` | Additional native Kubernetes EnvVar entries for server Pods.  |
| <span id="helm-extravolumes"></span>`extraVolumes` | array | `[]` | Additional native Kubernetes volumes for server Pods.  |
| <span id="helm-extravolumemounts"></span>`extraVolumeMounts` | array | `[]` | Additional native Kubernetes volume mounts for server containers.  |
| <span id="helm-resources"></span>`resources` | object | Object; see nested fields | Server Pod requests and limits, separate from Spark worker resources.  |
| <span id="helm-resources-requests"></span>`resources.requests` | object | Object; see nested fields | Native server container resource requests limit.  |
| <span id="helm-resources-requests-cpu"></span>`resources.requests.cpu` | string | `"100m"` | Native server container resource cpu request.  |
| <span id="helm-resources-requests-memory"></span>`resources.requests.memory` | string | `"128Mi"` | Native server container resource memory request.  |
| <span id="helm-resources-limits"></span>`resources.limits` | object | Object; see nested fields | Native server container resource limits limit.  |
| <span id="helm-resources-limits-memory"></span>`resources.limits.memory` | string | `"512Mi"` | Native server container resource memory limit.  |
| <span id="helm-podannotations"></span>`podAnnotations` | object | `{}` | Server Pod annotation map.  |
| <span id="helm-nodeselector"></span>`nodeSelector` | object | `{}` | Server Pod node-label selector map.  |
| <span id="helm-tolerations"></span>`tolerations` | array | `[]` | Server Pod Kubernetes tolerations.  |
| <span id="helm-ui"></span>`ui` | object | Object; see nested fields | Embedded administrator console and browser sessions.  |
| <span id="helm-ui-enabled"></span>`ui.enabled` | boolean | `false` | Register UI, browser login and administrator routes.  |
| <span id="helm-ui-publicurl"></span>`ui.publicURL` | string | `""` | Required absolute HTTP(S) URL ending exactly /ui/, without credentials, query or fragment.  |
| <span id="helm-ui-ingress"></span>`ui.ingress` | object | Object; see nested fields | Same-origin UI/admin Ingress; does not expose the public job API or Spark UI.  |
| <span id="helm-ui-ingress-enabled"></span>`ui.ingress.enabled` | boolean | `false` | Render Ingress for /ui and /v1/admin.  |
| <span id="helm-ui-ingress-classname"></span>`ui.ingress.className` | string | `""` | Optional IngressClass name. Maximum length 253. |
| <span id="helm-ui-ingress-annotations"></span>`ui.ingress.annotations` | object | `{}` | Ingress annotations map.  |
| <span id="helm-ui-ingress-tls"></span>`ui.ingress.tls` | object | Object; see nested fields | Ingress TLS certificate selection.  |
| <span id="helm-ui-ingress-tls-enabled"></span>`ui.ingress.tls.enabled` | boolean | `false` | Enable TLS for the publicURL host.  |
| <span id="helm-ui-ingress-tls-secretname"></span>`ui.ingress.tls.secretName` | string | `""` | Existing TLS Secret for the host. Maximum length 253. |
| <span id="helm-ui-sessionseconds"></span>`ui.sessionSeconds` | integer | `3600` | Absolute browser session lifetime, 60–86400 seconds. Provider refresh renews authentication within this limit; without refresh, token expiry can end access sooner. Minimum 60. Maximum 86400. |
| <span id="helm-ui-grafanaurl"></span>`ui.grafanaURL` | string | `""` | Optional absolute Grafana link shown in the console.  |
| <span id="helm-ui-oidc"></span>`ui.oidc` | object | Object; see nested fields | Browser client configuration, independent of application OAuth credentials.  |
| <span id="helm-ui-oidc-clientid"></span>`ui.oidc.clientId` | string | `""` | Required browser client ID/audience for secured UI.  |
| <span id="helm-ui-oidc-scopes"></span>`ui.oidc.scopes` | array | `["openid","profile","email"]` | Replaces defaults; nonempty unique valid scope strings, including openid. Request groups when needed.  |
| <span id="helm-ui-oidc-secretref"></span>`ui.oidc.secretRef` | string | `""` | Optional existing browser confidential-client Secret.  |
| <span id="helm-ui-oidc-clientsecretkey"></span>`ui.oidc.clientSecretKey` | string | `"client-secret"` | Key holding browser client secret.  |
| <span id="helm-metrics"></span>`metrics` | object | Object; see nested fields | Prometheus endpoint, context dimensions and optional monitoring resources.  |
| <span id="helm-metrics-enabled"></span>`metrics.enabled` | boolean | `true` | Register /metrics.  |
| <span id="helm-metrics-maxcontexts"></span>`metrics.maxContexts` | integer | `100` | Lifetime cap on named context metric dimensions; 0–10000. Minimum 0. Maximum 10000. |
| <span id="helm-metrics-cluster"></span>`metrics.cluster` | string | `""` | Cluster label used by monitoring resources.  |
| <span id="helm-metrics-contextaliases"></span>`metrics.contextAliases` | array | `[]` | Persistent owner/logical-name alias overrides, applied before enrollment.  |
| <span id="helm-metrics-contextaliases-owner"></span>`metrics.contextAliases[].owner` | string | Omitted | Application owner.  |
| <span id="helm-metrics-contextaliases-name"></span>`metrics.contextAliases[].name` | string | Omitted | Logical context name.  |
| <span id="helm-metrics-contextaliases-alias"></span>`metrics.contextAliases[].alias` | string | Omitted | Bounded explicit metric alias.  |
| <span id="helm-metrics-servicemonitor"></span>`metrics.serviceMonitor` | object | Object; see nested fields | Optional Prometheus Operator ServiceMonitor; CRD must already exist.  |
| <span id="helm-metrics-servicemonitor-enabled"></span>`metrics.serviceMonitor.enabled` | boolean | `false` | Render ServiceMonitor; no operator/CRD installation.  |
| <span id="helm-metrics-servicemonitor-labels"></span>`metrics.serviceMonitor.labels` | object | `{}` | Labels used by the Prometheus ServiceMonitor selector.  |
| <span id="helm-metrics-servicemonitor-interval"></span>`metrics.serviceMonitor.interval` | string | `"30s"` | Prometheus scrape interval.  |
| <span id="helm-metrics-servicemonitor-scrapetimeout"></span>`metrics.serviceMonitor.scrapeTimeout` | string | `"10s"` | Prometheus scrape timeout.  |
| <span id="helm-metrics-servicemonitor-scheme"></span>`metrics.serviceMonitor.scheme` | string | `"http"` | Scrape HTTP or HTTPS.  Values: `http`, `https`. |
| <span id="helm-metrics-servicemonitor-samplelimit"></span>`metrics.serviceMonitor.sampleLimit` | integer | `100000` | Maximum samples accepted per scrape. Minimum 1. Maximum 10000000. |
| <span id="helm-metrics-servicemonitor-relabelings"></span>`metrics.serviceMonitor.relabelings` | array | `[]` | Prometheus target relabeling configurations.  |
| <span id="helm-metrics-servicemonitor-metricrelabelings"></span>`metrics.serviceMonitor.metricRelabelings` | array | `[]` | Prometheus metric relabeling configurations.  |
| <span id="helm-metrics-servicemonitor-tlsconfig"></span>`metrics.serviceMonitor.tlsConfig` | object | `{}` | Native ServiceMonitor TLS client configuration.  |
| <span id="helm-metrics-servicemonitor-authorization"></span>`metrics.serviceMonitor.authorization` | object | `{}` | Native ServiceMonitor scrape authorization.  |
| <span id="helm-metrics-grafanadashboard"></span>`metrics.grafanaDashboard` | object | Object; see nested fields | Optional Grafana Operator dashboard; CRD must already exist.  |
| <span id="helm-metrics-grafanadashboard-enabled"></span>`metrics.grafanaDashboard.enabled` | boolean | `false` | Render dashboard resource; no Grafana/CRD installation.  |
| <span id="helm-metrics-grafanadashboard-labels"></span>`metrics.grafanaDashboard.labels` | object | `{}` | Dashboard resource labels.  |
| <span id="helm-metrics-grafanadashboard-instanceselector"></span>`metrics.grafanaDashboard.instanceSelector` | object | `{}` | Selector for Grafana instances receiving this dashboard.  |
| <span id="helm-metrics-grafanadashboard-folder"></span>`metrics.grafanaDashboard.folder` | string | `"Linha"` | Grafana folder name.  |
| <span id="helm-metrics-grafanadashboard-datasourceuid"></span>`metrics.grafanaDashboard.datasourceUID` | string | `""` | Grafana Prometheus datasource UID.  |
| <span id="helm-metrics-grafanadashboard-allowcrossnamespaceimport"></span>`metrics.grafanaDashboard.allowCrossNamespaceImport` | boolean | `false` | Allow Grafana Operator cross-namespace dashboard import.  |
| <span id="helm-metrics-grafanadashboard-resyncperiod"></span>`metrics.grafanaDashboard.resyncPeriod` | string | `"10m"` | Grafana Operator resync interval.  |

See [environment equivalents](environment.md), [S3 destinations and membership](storage.md),
[deployment procedures](../../operations.md) and [monitoring](../../observability.md).
The removed existingSecret, deploymentMode, top-level oidc and security.oidc.audience
fields are not supported.
