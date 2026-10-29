# Server environment

Standalone server options. Boolean values use Go ParseBool syntax; use true/false
in deployment configuration. Structured values are JSON, not YAML. Helm-specific
defaults can differ from standalone defaults: for example the chart supplies its
namespace, worker Service URL, database hostname and listen address.

| Variable | Type / standalone default | Meaning / Helm equivalent |
| --- | --- | --- |
| <span id="linha-log-level"></span>`LINHA_LOG_LEVEL` | string / info | JSON log level, parsed as slog level; for example debug, info, warn, error. |
| <span id="linha-listen"></span>`LINHA_LISTEN` | string / 127.0.0.1:8080 | HTTP listen address; chart selects 0.0.0.0:8080. |
| <span id="linha-database-url"></span>`LINHA_DATABASE_URL` | string / unset | Standalone PostgreSQL DSN; mutually exclusive with all separate database fields. |
| <span id="linha-database-host"></span>`LINHA_DATABASE_HOST` | string / required | database.postgres.host; no standalone host default. |
| <span id="linha-database-port"></span>`LINHA_DATABASE_PORT` | integer / 5432 | database.postgres.port; 1–65535. |
| <span id="linha-database-name"></span>`LINHA_DATABASE_NAME` | string / required | database.postgres.database; no standalone database default. |
| <span id="linha-database-username"></span>`LINHA_DATABASE_USERNAME` | string / required | Selected database Secret username; server URI-encodes it. |
| <span id="linha-database-password"></span>`LINHA_DATABASE_PASSWORD` | string / required | Selected database Secret password; server URI-encodes it. |
| <span id="linha-database-ssl-mode"></span>`LINHA_DATABASE_SSL_MODE` | string / verify-full | database.postgres.sslMode; disable, allow, prefer, require, verify-ca or verify-full. |
| <span id="linha-database-ssl-root-cert"></span>`LINHA_DATABASE_SSL_ROOT_CERT` | path / unset | database.postgres.sslRootCert; mounted PostgreSQL CA. |
| <span id="linha-security-enabled"></span>`LINHA_SECURITY_ENABLED` | boolean / true | security.enabled; master authentication switch. |
| <span id="linha-api-auth-enabled"></span>`LINHA_API_AUTH_ENABLED` | boolean / true | security.api.enabled; API JWT validation when master security is enabled. |
| <span id="linha-api-jwt-issuer"></span>`LINHA_API_JWT_ISSUER` | URL / required with API auth | security.api.jwt.issuer. |
| <span id="linha-api-jwt-audience"></span>`LINHA_API_JWT_AUDIENCE` | string / linha | security.api.jwt.audience. |
| <span id="linha-api-jwt-tls-insecure-skip-verify"></span>`LINHA_API_JWT_TLS_INSECURE_SKIP_VERIFY` | boolean / false | security.api.jwt.tls.insecureSkipVerify; API identity-provider TLS only. |
| <span id="linha-oidc-enabled"></span>`LINHA_OIDC_ENABLED` | boolean / true | security.oidc.enabled; secured enabled UI requires it. |
| <span id="linha-oidc-issuer"></span>`LINHA_OIDC_ISSUER` | URL / required with secured UI | security.oidc.issuer; independent browser issuer. |
| <span id="linha-oidc-tls-insecure-skip-verify"></span>`LINHA_OIDC_TLS_INSECURE_SKIP_VERIFY` | boolean / false | security.oidc.tls.insecureSkipVerify; browser provider TLS only. |
| <span id="linha-worker-auth"></span>`LINHA_WORKER_AUTH` | enum / projected-token | projected-token or token-review; chart chooses from rbac.namespaceOnly. |
| <span id="linha-namespace"></span>`LINHA_NAMESPACE` | string / unset | Enables in-cluster managed Spark backends in this namespace; needed for secured managed-worker authentication. |
| <span id="linha-server-url"></span>`LINHA_SERVER_URL` | URL / required for managed backends | Internal API URL injected into managed workers; chart derives its Service URL. |
| <span id="linha-worker-service-account"></span>`LINHA_WORKER_SERVICE_ACCOUNT` | string / required for managed backends | Managed worker service-account name; chart supplies it. |
| <span id="linha-allowed-images"></span>`LINHA_ALLOWED_IMAGES` | comma-separated prefixes / required for managed backends | allowedImages, joined by commas. |
| <span id="linha-worker-volumes"></span>`LINHA_WORKER_VOLUMES` | JSON array / [] | workerVolumes legacy defaults. |
| <span id="linha-worker-mounts"></span>`LINHA_WORKER_MOUNTS` | JSON array / [] | workerMounts legacy defaults. |
| <span id="linha-worker-pod-templates"></span>`LINHA_WORKER_POD_TEMPLATES` | JSON object / empty templates | workerPodTemplates snapshot defaults. |
| <span id="linha-worker-image-pull-secrets"></span>`LINHA_WORKER_IMAGE_PULL_SECRETS` | JSON string array / [] | Worker default pull Secret names derived from chart imagePullSecrets. |
| <span id="linha-registry-secrets"></span>`LINHA_REGISTRY_SECRETS` | JSON string array / [] | registrySecrets.allowedNames permitted Secret names. |
| <span id="linha-enable-fake-engine"></span>`LINHA_ENABLE_FAKE_ENGINE` | boolean / false | Enable fake adapter version 1 for local SDK/controller development, not managed Spark. |
| <span id="linha-local-root"></span>`LINHA_LOCAL_ROOT` | path / unset | Configure local result provider; persistent shared mounts must be supplied. |
| <span id="linha-s3-destinations"></span>`LINHA_S3_DESTINATIONS` | JSON object / {} | Destination alias → S3Config. Chart loads it from s3.secretRef/destinationsKey. |
| <span id="linha-cleanup-enabled"></span>`LINHA_CLEANUP_ENABLED` | boolean / false | results.cleanup.enabled; delete eligible tracked output only. |
| <span id="linha-max-staging-bytes"></span>`LINHA_MAX_STAGING_BYTES` | positive integer / 5368709120 | results.maxStagingBytes concurrent server staging reservation limit. |
| <span id="linha-metrics-enabled"></span>`LINHA_METRICS_ENABLED` | boolean / true | metrics.enabled; register /metrics. |
| <span id="linha-metrics-max-contexts"></span>`LINHA_METRICS_MAX_CONTEXTS` | integer / 100 | metrics.maxContexts; 0–10000. |
| <span id="linha-metrics-context-aliases"></span>`LINHA_METRICS_CONTEXT_ALIASES` | JSON array / [] | metrics.contextAliases; owner/name/alias objects. |
| <span id="linha-ui-enabled"></span>`LINHA_UI_ENABLED` | boolean / false | ui.enabled; no browser/admin routes when disabled. |
| <span id="linha-ui-public-url"></span>`LINHA_UI_PUBLIC_URL` | URL / required with UI | ui.publicURL; absolute http(s) URL ending exactly /ui/. |
| <span id="linha-ui-session-seconds"></span>`LINHA_UI_SESSION_SECONDS` | integer / 3600 | ui.sessionSeconds absolute browser lifetime; 60–86400. |
| <span id="linha-ui-oidc-client-id"></span>`LINHA_UI_OIDC_CLIENT_ID` | string / required with secured UI | ui.oidc.clientId browser audience. |
| <span id="linha-ui-oidc-client-secret"></span>`LINHA_UI_OIDC_CLIENT_SECRET` | secret / unset | Optional confidential browser client secret; chart reads selected Secret. |
| <span id="linha-ui-oidc-scopes"></span>`LINHA_UI_OIDC_SCOPES` | JSON string array / [openid,profile,email] | ui.oidc.scopes replaces defaults; include openid. |
| <span id="linha-ui-grafana-url"></span>`LINHA_UI_GRAFANA_URL` | URL / unset | ui.grafanaURL optional console link. |
| <span id="linha-admin-mappings"></span>`LINHA_ADMIN_MAPPINGS` | JSON object / empty viewer/operator rules | security.admin membership models; see the provider/membership reference. |
| <span id="linha-mode"></span>`LINHA_MODE` | removed | Any nonempty value fails startup. Use the independent authentication settings. |
| <span id="linha-oidc-audience"></span>`LINHA_OIDC_AUDIENCE` | removed | Any nonempty value fails startup with master security enabled. Use LINHA_API_JWT_AUDIENCE and browser client ID. |

AWS credentials use the standard provider chain; AWS_REGION is not the per-destination
region default (S3Config defaults to us-east-1). SSL_CERT_DIR extends system CA lookup
for server provider HTTPS. These are upstream environment variables, not Linha
backend fields. Worker-injected identity settings are documented in the
[worker SDK reference](../sdk/worker.md#managed-worker-environment).
