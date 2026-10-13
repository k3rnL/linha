# Configuration reference

Configuration belongs to three surfaces: deployment settings in Helm, immutable
backend settings supplied by the client, and policy captured with each request.
Unknown or conflicting fields are rejected rather than silently ignored.

The complete chart contract is in
[values.yaml](https://github.com/k3rnL/linha/blob/main/deploy/helm/linha/values.yaml)
and [values.schema.json](https://github.com/k3rnL/linha/blob/main/deploy/helm/linha/values.schema.json).

## Helm deployment settings

| Setting | Type / default | Purpose |
| --- | --- | --- |
| `replicaCount` | integer / `2` | API replicas sharing the same database |
| `image.repository`, `image.tag` | strings / GHCR server, chart release | Published server image; use a matching source build for unreleased features |
| `imagePullSecrets` | array / `[]` | Server pull Secret references and worker defaults |
| `database.postgres.host` | string / `postgres` | PostgreSQL host |
| `database.postgres.port` | integer / `5432` | PostgreSQL port |
| `database.postgres.database` | string / `linha` | Existing database name |
| `database.postgres.sslMode` | string / `verify-full` | PostgreSQL TLS mode |
| `database.postgres.sslRootCert` | string / empty | Mounted PostgreSQL CA file |
| `database.postgres.secretRef` | string / `linha-postgres` | Caller-managed database credential Secret |
| `database.postgres.usernameKey`, `passwordKey` | strings / `username`, `password` | Selected Secret keys |
| `security.enabled` | boolean / `true` | Master authentication switch; false uses one anonymous owner |
| `security.api.enabled` | boolean / `true` | Public job API JWT validation |
| `security.api.jwt.issuer` | string / empty | Required API token issuer when API authentication is active |
| `security.api.jwt.audience` | string / `linha` | Intended API token audience |
| `security.api.jwt.tls.insecureSkipVerify` | boolean / `false` | Skip TLS verification only for API identity discovery/keys |
| `security.oidc.enabled` | boolean / `true` | Browser administrator OIDC login |
| `security.oidc.issuer` | string / empty | Browser issuer; required for secured UI |
| `security.oidc.tls.insecureSkipVerify` | boolean / `false` | Skip TLS verification only for browser identity operations |
| `security.admin.viewer`, `operator` | objects / empty rules | Subject or claim membership; inherit browser issuer |
| `rbac.namespaceOnly` | boolean / `true` | Namespaced management; false adds worker TokenReview permission |
| `allowedImages` | string array / `[]` | Authorized worker, sidecar and init-container repository prefixes |
| `local.enabled` | boolean / `false` | Enable local result destination |
| `local.root` | string / `/var/lib/linha/results` | Shared persistent result root |
| `local.existingClaim` | string / empty | Caller-managed PVC for server results |
| `s3.enabled` | boolean / `false` | Enable S3 destinations |
| `s3.secretRef`, `destinationsKey` | strings / `linha-s3`, `destinations` | Secret containing destination JSON |
| `workerPodTemplates` | object / empty templates | Default driver/executor Pod fragments |
| `registrySecrets.allowedNames` | string array / `[]` | Registry Secrets approved for per-context selection |
| `results.cleanup.enabled` | boolean / `false` | Physical deletion of eligible tracked output |
| `results.maxStagingBytes` | integer / `5368709120` | Server concurrent staging reservation budget |
| `ui.enabled` | boolean / `false` | Register embedded UI/admin/login routes |
| `ui.publicURL` | string / empty | Absolute URL ending in `/ui/`, no query or fragment |
| `ui.sessionSeconds` | integer / `3600` | Browser session lifetime, additionally capped by token expiry |
| `ui.oidc.clientId` | string / empty | Browser client ID and ID-token audience |
| `ui.oidc.scopes` | string array / `[openid, profile, email]` | Browser scopes; must include `openid` |
| `ui.oidc.secretRef`, `clientSecretKey` | strings / empty, `client-secret` | Optional confidential browser client credentials |
| `ui.ingress.enabled` | boolean / `false` | Same-origin `/ui` and `/v1/admin` ingress |
| `metrics.enabled` | boolean / `true` | Register `/metrics` |
| `metrics.maxContexts` | integer / `100` | Lifetime cap of named metric context dimensions, 0–10,000 |
| `metrics.serviceMonitor.enabled` | boolean / `false` | Requires Prometheus Operator CRD |
| `metrics.grafanaDashboard.enabled` | boolean / `false` | Requires Grafana Operator CRD |

`extraEnv`, `extraVolumes`, `extraVolumeMounts`, `resources`, `nodeSelector` and
`tolerations` configure server Pods. `workerVolumes` and `workerMounts` are
administrator mount defaults; prefer explicit driver/executor Pod templates.

The removed `existingSecret`, `deploymentMode`, top-level `oidc`, and
`security.oidc.audience` fields are not part of the current contract.

## Spark settings

These fields are under `spec.engine.settings`. The engine type is `spark`,
version `3.5.6`. The application uses `sparkoperator.k8s.io/v1beta2`.

| Field | Type / default | Contract |
| --- | --- | --- |
| `application.mainClass` | string / required | JVM main class installed in the image |
| `application.mainApplicationFile` | string / required | `local:///` path ending in `.jar` |
| `application.arguments` | string array / `[]` | At most 128 main-class arguments |
| `drivers.minDrivers` | integer / `1` | Minimum with demand, may be zero |
| `drivers.maxDrivers` | integer / `1` | 1–16, at least the minimum |
| `drivers.maxConcurrentRequestsPerDriver` | integer / `1` | 1–32 request slots per driver |
| `drivers.queueThreshold` | integer / `1` | Ready queue threshold for scale-up |
| `drivers.waitSeconds` | integer / `5` | Queue wait threshold for scale-up |
| `drivers.cooldownSeconds` | integer / `10` | Scale-up cooldown |
| `drivers.idleSeconds` | integer / `60` | Idle retirement above minimum |
| `drivers.cores`, `executors.cores` | integer / `1` | Spark parallelism, 1–32 |
| `drivers.coreRequest`, `executors.coreRequest` | quantity string / cores | Positive CPU request, up to 32 cores |
| `drivers.coreLimit`, `executors.coreLimit` | quantity string / cores | CPU limit, at least request, up to 32 cores |
| `drivers.memory`, `executors.memory` | string / `1024m` | JVM heap, 512m–128g, units m/g |
| `drivers.memoryOverhead`, `executors.memoryOverhead` | string / max(384m, 10% heap) | Added to heap for container request and limit |
| `drivers.javaOptions`, `executors.javaOptions` | string / empty | Additional JVM options; heap overrides rejected |
| `executors.dynamicAllocation` | boolean / `false` | Spark executor autoscaling |
| `executors.instances` | integer / `1` | Static executor count, 1–128 |
| `executors.minExecutors` | integer / `0` | Dynamic minimum |
| `executors.initialExecutors` | integer / `1` | Dynamic startup count, at least minimum |
| `executors.maxExecutors` | integer / `4` | Dynamic maximum, at least initial, 1–128 |
| `executors.executorIdleTimeout` | duration / `60s` | Idle timeout without retained data |
| `executors.cachedExecutorIdleTimeout` | duration / `infinity` | Cached-data retention timeout |
| `executors.shuffleTrackingTimeout` | duration / `infinity` | Shuffle-data retention timeout |
| `ui.enabled` | boolean / `false` | Start each driver's Spark UI |
| `ui.port` | integer / `4040` | 1024–65535 |
| `kubernetes.driverPodTemplate`, `executorPodTemplate` | objects / empty | Independent native fragments; each at most 64 KiB |

Durations accept positive integer s/m/h/d units up to 2,147,483,647 seconds.
Cache/shuffle timeouts also accept `infinity`. Equivalent durations normalize to
one configuration version. Deprecated `memoryMi` is a heap alias and cannot be
combined with `memory`; new configuration should use Spark units.

Example: `memory: "10g"` with `memoryOverhead: "2g"` creates a 10g heap inside a
12Gi container. See [manage Spark applications](versioned-spark-applications.md).

## Results and request policy

| Field | Default / bound | Meaning |
| --- | --- | --- |
| `results.type` | `local` or `s3` | Backend result provider |
| `results.root` | allowed configured root | Local result filesystem |
| `results.destination` | configured alias | S3 destination; no bucket credentials in requests |
| `results.path` | version 1 template | Relative output key policy |
| `retry.maxAttempts` | `1`, maximum `10` | Total attempts, including the first |
| `retry.backoffSeconds` | `1` | Delay between eligible retries |
| Result `maxBytes` | 64 MiB, maximum 5 GiB | Total accepted result bytes |
| Result retention | seven days by default | Availability duration; metadata remains after expiry |
| Attempt lease | 30 seconds | Current worker authority |
| Client lease | 90 seconds | Context demand; SDK renews every 30 seconds |
| Dataset parts | maximum 100,000 | Distributed output inventory cap |

The [OpenAPI contract](https://github.com/k3rnL/linha/blob/main/api/openapi.json)
is authoritative for complete request schema, policy placement and bounds.

## Result path templates

The default is `{contextId}/{requestId}/{attemptId}/{file}`. `{requestId}` and
`{attemptId}` must each occupy a complete segment, and `{file}` must appear once
at the end. `{contextId}` is optional. `{submittedAt}` expands the original
accepted time in UTC; formats allow yyyy/MM/dd/HH/mm/ss/SSS fields and `/ - _ T Z`
literals.

```text
exports/{submittedAt:yyyy/MM/dd}/{requestId}/{attemptId}/{file}
```

Templates are declarative data, not remote lambdas. Absolute paths, URI schemes,
traversal, encoded traversal and reserved `_linha` segments are rejected. The S3
bucket and base prefix remain administrator-controlled.

## Standalone server environment

| Helm field | Environment variable |
| --- | --- |
| Master security | `LINHA_SECURITY_ENABLED` |
| API JWT enabled / issuer / audience | `LINHA_API_AUTH_ENABLED`, `LINHA_API_JWT_ISSUER`, `LINHA_API_JWT_AUDIENCE` |
| API provider TLS bypass | `LINHA_API_JWT_TLS_INSECURE_SKIP_VERIFY` |
| Browser OIDC enabled / issuer | `LINHA_OIDC_ENABLED`, `LINHA_OIDC_ISSUER` |
| Browser provider TLS bypass | `LINHA_OIDC_TLS_INSECURE_SKIP_VERIFY` |
| Admin membership JSON | `LINHA_ADMIN_MAPPINGS` |
| Browser client / scopes | `LINHA_UI_OIDC_CLIENT_ID`, `LINHA_UI_OIDC_SCOPES` |
| Database fields | `LINHA_DATABASE_HOST`, `PORT`, `NAME`, `USERNAME`, `PASSWORD`, `SSL_MODE`, `SSL_ROOT_CERT`, each with `LINHA_DATABASE_` prefix |
| Cleanup / staging | `LINHA_CLEANUP_ENABLED`, `LINHA_MAX_STAGING_BYTES` |
| Worker templates / pull names / permitted registry names | `LINHA_WORKER_POD_TEMPLATES`, `LINHA_WORKER_IMAGE_PULL_SECRETS`, `LINHA_REGISTRY_SECRETS` |

A standalone `LINHA_DATABASE_URL` is mutually exclusive with separate database
fields. Managed backends also require `LINHA_SERVER_URL`,
`LINHA_WORKER_SERVICE_ACCOUNT` and `LINHA_ALLOWED_IMAGES`. Helm supplies these.
`LINHA_WORKER_AUTH` selects `projected-token` (default) or `token-review`.
Use JSON arrays/objects for structured environment settings.
