# Running Linha

The API returns a public job ID only after committing acceptance to PostgreSQL.
Engine startup does not delay acceptance. A successful response does not mean the
job started or will succeed. Retrying the same owner/logical-name/idempotency key and
request returns the original ID; changing the request returns a conflict.

Deploy at least two server replicas using `deploy/helm/linha`. Supply an external
HA PostgreSQL service and durable result storage. The chart does not make a
single database instance or a single-node disk highly available. The Kind test
uses one node and is a failure-recovery test, not proof of infrastructure HA.

Configure PostgreSQL connection details separately from credentials. The chart reads
only the two selected keys from a caller-managed Secret in the release namespace:

```yaml
database:
  postgres:
    host: postgres.example.internal
    port: 5432
    database: linha
    sslMode: verify-full
    secretRef: my-postgres-credentials
    usernameKey: username
    passwordKey: password
security:
  enabled: true
  oidc:
    enabled: true
    issuer: https://identity.example/realms/my-realm
    audience: linha
rbac:
  namespaceOnly: true
```

Passwords and usernames are URI-encoded by the server, so special characters do
not need manual escaping in Secret values. `sslMode` defaults to `verify-full`;
use `sslRootCert` for a CA file mounted with `extraVolumes`/`extraVolumeMounts`.
For a local PostgreSQL fixture without TLS, explicitly choose `sslMode: disable`.
Linha does not create the database, Secret, or certificate mount.

`security.enabled` is the master authentication switch. With `false`, public and
worker authentication are disabled, no OIDC provider is contacted, and no Linha
worker token is projected. Every client uses the stable owner `anonymous`.
The worker SDK sends context/worker/incarnation routing headers; these are not
credentials. Durable claims, lease fencing, and result publication still apply.
This setting provides no isolation between clients or protection against worker
impersonation. Kubernetes authentication remains necessary for provisioning
Pods and for Spark's executor management.

With `security.enabled: true` and `security.oidc.enabled: false`, only the public
API is anonymous; workers must still authenticate. With both switches `true`,
owner identity is a stable hash of validated OIDC issuer and subject; request JSON
cannot choose ownership. Missing issuer/audience fails chart rendering and server
startup. Tokens must be signed JWTs from the issuer's discovery/JWKS endpoint,
with the configured audience. Opaque OAuth access tokens are unsupported.

The Scala client can use `CredentialProvider.none` for an anonymous API. Managed
workers keep using `LinhaWorker.projectedIdentity()` and `projectedCredentials()`;
the updated SDK handles the master switch automatically. Rebuild worker images
with this SDK before using fully disabled security. Switching authentication
configuration does not rewrite ownership of existing contexts/jobs: anonymous
and previously authenticated identities keep separate records.

All driver and executor operations target the Helm release namespace.
`rbac.namespaceOnly: true` (default) creates **no ClusterRole or ClusterRoleBinding**.
Workers' projected JWTs are checked against Kubernetes signing keys, issuer,
audience and expiry. Each call also checks the live Pod UID, service-account UID,
labels, deletion state, and persisted instance assignment. Signing keys are fetched
from the trusted API server and cached/refreshed by the JWT library; Pod and
service-account identity checks are not cached. Rotating token files are reread.
The namespaced server Role grants `get` on service accounts in addition to Pod/Service
management. Signing-key discovery uses Kubernetes' default service-account issuer
discovery access. Clusters that removed that default access must restore it for
Linha or select TokenReview; Linha never silently disables verification.
See [Kubernetes service-account discovery](https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/#service-account-issuer-discovery)
and [bound token validation](https://kubernetes.io/docs/reference/access-authn-authz/service-accounts-admin/#schema-for-service-account-private-claims).

Setting `rbac.namespaceOnly: false` adds only the cluster-scoped TokenReview grant
and uses TokenReview for workers; resource management remains namespace-scoped.
If `security.enabled: false`, no cluster-scoped RBAC is rendered in either case.

Spark's worker Role includes `deletecollection` on Pods and ConfigMaps in the
release namespace. Kubernetes treats collection DELETEs (including label-selector
cleanup) as a [separate permission from single-object delete](https://kubernetes.io/docs/reference/access-authn-authz/authorization/#resource-requests).
A `cannot deletecollection resource "pods"` or `"configmaps"` error from the worker
service account indicates that the installed Role lacks this permission. Upgrade
the existing Helm release using your normal values file and target context; no
image rebuild or forced driver restart is required for this RBAC correction.
The server Role and collection-deletion permissions for other resources are unchanged.

To verify the installation, substitute your release namespace and worker account:

```sh
kubectl --context YOUR_CONTEXT -n YOUR_NAMESPACE auth can-i deletecollection pods \
  --as=system:serviceaccount:YOUR_NAMESPACE:YOUR_WORKER_SERVICE_ACCOUNT
kubectl --context YOUR_CONTEXT -n YOUR_NAMESPACE auth can-i deletecollection configmaps \
  --as=system:serviceaccount:YOUR_NAMESPACE:YOUR_WORKER_SERVICE_ACCOUNT
```

Both should report `yes`. Running these checks as another identity requires
permission to impersonate that service account. For the reported `lihna` release
in `metocprod`, the worker account is `lihna-linha-worker`.

Registry tags are resolved during each ensure; the effective config and digest identify
a version under a stable context name. Replacements reuse that version's digest. Select namespaced registry
Secrets through the driver/executor Pod templates; permit them using Helm
`registrySecrets.allowedNames` or top-level `imagePullSecrets`. Linha uses these
credentials for digest lookup and Kubernetes uses the refs for image pulling.
See [Pod customization](pod-templates.md) for the Scala/JSON interface, defaults,
scoped RBAC and protected fields. The [SparkApplication migration guide](versioned-spark-applications.md)
covers the operator dependency, context leases and required client renewal (dataset output additionally upgrades schema to v6).

Worker images and this namespace are an administrator-controlled trust boundary.
Spark drivers need Kubernetes permissions to create executor pods. Do not offer
arbitrary untrusted container images in this namespace. Public API ownership
checks do not sandbox business code with shared filesystem or Kubernetes access.
Separate trust domains using separate namespaces/deployments and admission policy.

For local results, set `local.enabled`, `local.root`, and `local.existingClaim`.
Use a caller-owned persistent volume with access from every serving replica and
participating worker/executor. An ephemeral `emptyDir` is unsuitable for retained
results. `workerVolumes` and `workerMounts` configure administrator-owned driver
mounts. PVC mounts are also propagated to native Spark executors. Other driver volume types
are not automatically propagated.
The default shared group is 10001; pre-provision compatible ownership and access.

For S3, select its own configuration Secret and key:

```yaml
s3:
  enabled: true
  secretRef: my-result-storage
  destinationsKey: destinations
```

That key contains destination JSON, for example:

```json
{"analytics":{"bucket":"my-results","prefix":"linha","region":"eu-west-1"}}
```

An optional `endpoint` and `pathStyle: true` support S3-compatible services.
Credentials come from the AWS default provider chain in the server (for example,
a scoped workload role); destination credentials never travel in job JSON.
The role needs object read/write/head under its configured prefix. Current file
and JSON uploads are proxied through the server and published conditionally.
No database result-payload fallback is used when the destination is unavailable.
Bucket, prefix and endpoint identity are snapshotted in newly ensured S3 policies.
Changing that location under an existing destination name returns
`DESTINATION_CHANGED` for retained data. Keep destination names bound to stable
locations; register a new name when moving output to another bucket or prefix.
Credentials can rotate without changing the destination fingerprint. Contexts
created by earlier development snapshots without a fingerprint require their
original destination configuration to remain unchanged.

Paths are templates, not executable callbacks. For example:

```scala
ResultStorage.S3("analytics", ResultPath.template(
  "exports/{submittedAt:yyyy/MM/dd}/{requestId}/{attemptId}/{file}"
))
```

The bucket and authorized base prefix come from the destination configuration.
`submittedAt` is the original database acceptance timestamp, always expanded in
UTC. `{requestId}` and `{attemptId}` are mandatory complete segments; `{file}`
appears once at the end. `contextId` is optional. Supported date fields are
`yyyy MM dd HH mm ss SSS`, with `/ - _ T Z` literals. Traversal, encoded traversal,
absolute paths, schemes, reserved `_linha` names, and overlapping allocations
are rejected. Retrying output allocation returns the persisted allocation.

Defaults are 30-second attempt leases, one execution attempt, 64 MiB total result
bytes, and seven-day result availability. Callers can request bounded retries
(up to ten attempts), backoff, deadlines, result size, and retention.
The maximum configured result size is 5 GiB; server staging needs enough disk for
concurrent uploads, independently of per-job limits. Stream helpers enforce a
byte limit while writing. Seekable file callbacks are checked before upload;
use pod ephemeral-storage quotas to bound arbitrary callback disk use.

Expiry returns `RESULT_EXPIRED` while preserving job status and completion
metadata. Physical deletion is opt-in through `results.cleanup.enabled` (standalone:
`LINHA_CLEANUP_ENABLED`). It removes only persisted allocations from terminal
attempts, protecting retained results and unexpired delegated writers, with ten
minutes of grace. S3 cleanup aborts abandoned multipart uploads under the exact
allocated prefix. Job metadata, receipts and idempotency records remain indefinitely.
See [dataset setup and cleanup](dataset-results.md) for quota and delegation limits.
Provider object-version retention and backups remain the administrator's policy.


A lost lease fences further progress, allocation, and completion. The worker
cancels local execution when its monotonic lease budget expires, independently
of network calls. Controller replacement does not itself retry a request:
`maxAttempts` governs retry after interruption. User cancellation takes precedence
if committed before completion. Exactly-once business side effects are not
promised; make external writes idempotent when enabling retries.

`/livez` reports process liveness; `/readyz` checks PostgreSQL and the configured
local root. `/metrics` exports bounded job-state, queue age, lease, worker-slot,
backend-condition, and result gauges. Keep operational endpoints private.
Request correlation headers are returned; request tracing uses debug log level.
Storage failures and provisioning errors remain visible in API/backend conditions.

## Configuration migration

The old Helm `existingSecret`, top-level `oidc`, and `mode`/`deploymentMode` keys
are removed; rendering with them fails with a migration message. Split the old
`database-url` into `database.postgres` connection settings and the selected
username/password Secret keys. Move OIDC under `security.oidc`. Move S3 configuration
to `s3.secretRef`/`s3.destinationsKey` (these may point at the existing S3 key).
The chart has no deployment mode and no development-token requirement.

For standalone server use, `LINHA_DATABASE_URL` remains supported, mutually
exclusive with `LINHA_DATABASE_HOST`, `PORT`, `NAME`, `USERNAME`, `PASSWORD`,
`SSL_MODE`, and `SSL_ROOT_CERT` (each with the `LINHA_DATABASE_` prefix).
Configure `LINHA_SECURITY_ENABLED`, `LINHA_OIDC_ENABLED`, `LINHA_OIDC_ISSUER`, and
`LINHA_OIDC_AUDIENCE`. `LINHA_WORKER_AUTH` selects `projected-token` (default) or
`token-review` when security is enabled. `LINHA_MODE` is rejected and
`LINHA_DEV_TOKEN` is no longer used. The fake engine is an independent test-only
opt-in: `LINHA_ENABLE_FAKE_ENGINE=true`. The local `/dev-workers` helper is available
only with security disabled and returns routing identity with an empty token.
