# Administrator console

The server image embeds a React/TypeScript console at `/ui/`. It is disabled by
default. Administrators can inspect contexts and their immutable versions,
clients, workers, instances, requests, attempts, diagnostics, audit and retained
results. Opening a page never acquires a client lease. Ordinary SDK endpoints
retain owner isolation; the separate `/v1/admin/` endpoints require an explicit
administrator mapping.

## Enable with OIDC

Configure the normal SDK token audience and a separate browser OIDC client. The
browser client uses authorization code with PKCE. Register this exact redirect:
`https://linha.example/ui/oidc/callback`. Set its client secret in a caller-managed
Secret if the identity provider requires a confidential client:

```yaml
security:
  enabled: true
  oidc:
    enabled: true
    issuer: https://identity.example/realms/platform
    audience: linha-sdk
  admin:
    viewer:
      claims:
        - path: [realm_access, roles]
          values: [linha-viewer]
    operator:
      subjects:
        - subject: operator-subject-id
ui:
  enabled: true
  publicURL: https://linha.example/ui/
  sessionSeconds: 3600
  oidc:
    clientId: linha-admin
    secretRef: linha-browser-credentials
    clientSecretKey: client-secret
  grafanaURL: https://grafana.example/d/linha-operations/linha-operations
```

No user has administrator access by default. Rules inherit `security.oidc.issuer`
and match either an exact subject or a nested claim path with allowed values.
`security.admin` is a sibling of `security.oidc`. Existing explicit rule issuers
remain accepted when they match the configured issuer; a conflicting issuer is
rejected. Use the `0.1.1` chart and server or newer before omitting rule issuers;
server `0.1.0` still requires them.

The path identifies nested objects; string values and arrays of strings are supported. Operators can
also read. Viewers cannot submit/replay/cancel, even with forged browser headers.
Mappings are re-evaluated on every request using the running server configuration.
Redeploy replicas to apply configuration changes; sessions do not freeze roles.

`publicURL` must be absolute HTTP(S), end in `/ui/`, and have no query or fragment.
Use HTTPS in deployed environments. The chart can create the console ingress
with `ui.ingress.enabled: true`. Add this to the UI/OIDC configuration above:

```yaml
ui:
  enabled: true
  publicURL: https://linha.example/ui/
  ingress:
    enabled: true
    className: nginx
    annotations: {}
    tls:
      enabled: true
      secretName: linha-ui-tls
```

The host is inferred from `publicURL`; there is no second host setting to keep in
sync. The ingress routes `/ui` and `/v1/admin` with `Prefix` matching to the
existing server Service's `http` port. Login/callbacks, runtime configuration,
assets, deep links, API calls and result downloads all use that origin. It adds
no catch-all route for SDK/worker endpoints, metrics or health probes.

Install/select your ingress controller, point DNS at it, and provide the TLS
Secret in the release namespace (or have your certificate controller create it).
`className` is optional when your cluster has a suitable default IngressClass.
`annotations` accepts controller-specific string values, including certificate
issuer annotations. Preserve request paths and the `Origin` header; path-rewrite
annotations break the console's existing API and login URLs. The ingress does not
require sticky sessions: all replicas use the same database-backed sessions.
These settings follow the [Kubernetes Ingress API](https://kubernetes.io/docs/concepts/services-networking/ingress/).

Ingress TLS requires an HTTPS `publicURL` and `tls.secretName`. If TLS terminates
upstream of the ingress controller, use the external HTTPS public URL with
`tls.enabled: false`. Keep `ui.ingress.enabled: false` (the default) when using
an externally managed ingress; route `/ui` and `/v1/admin` on the same origin.
UI ingress requires `ui.enabled: true`. It does not change administrator role
checks or create Spark UI ingresses. A chart upgrade is sufficient; the existing
server image does not need rebuilding.

The server validates state, PKCE, nonce, issuer, signature, audience and expiry.
Login transactions are single-use PostgreSQL records. Sessions contain verified
claims and a hash of a random opaque session ID, and can be used through any
replica after restart. Cookies are HttpOnly, SameSite=Lax and Secure for HTTPS.
Cookie mutations require a matching origin and CSRF header, including logout.
Browser JavaScript never stores access/refresh tokens. Session lifetime is capped
by both the configured duration (60–86,400 seconds) and ID-token expiry; re-login
is required when it expires.

With the UI disabled, admin, login, session and runtime configuration routes are
not registered. If master security is enabled while OIDC is disabled, enabling
the UI is rejected. Explicitly setting `security.enabled: false` and
`ui.enabled: true` gives every visitor anonymous operator access, useful for an
isolated development installation. It shares the existing `anonymous` owner.

## Contexts and Spark links

Contexts group by owner and logical name. Each configuration hash identifies an
immutable version. Detail shows live clients and accepted work separately,
registered worker readiness, desired instances, timestamped observations, and
configuration. A running Kubernetes pod is different from a registered worker.

For Spark, each driver instance shows its application/pod, current owned executor
counts and main-container CPU/memory requests and limits. These are allocations,
not resource usage. Stale, missing or partial observations are displayed as such.
The controller persists them under its epoch and current application/pod identity.

Linha uses the existing SparkApplication
`status.driverInfo.webUIIngressAddress`. A scheme-less address requires matching
owned ingress metadata; TLS metadata determines the scheme. Linha neither
creates nor proxies Spark UI ingresses. Configure your Spark Operator's ingress
settings and an ingress controller/DNS/access policy separately. Each URL opens
in a new tab. Retired or replaced instances lose their live link. If no ingress
is published, the console shows the available service/port metadata instead.

## Overview and context history

`/ui/` opens the Overview, alongside Contexts, Requests and Servers. It shows
running/queued work now, outcomes during the last 24 hours, live client connections,
context versions, worker slots and server readiness. Average processing time uses
finished execution attempts during that window, excludes queue wait and counts
retries separately. The oldest waiting request helps identify a growing backlog.
Recent failures link to their full request diagnostics; active versions link to
context details.

`GET /v1/admin/overview` is read-only and has the same viewer/operator authorization
as other admin reads. Complete totals come from a consistent PostgreSQL snapshot,
independent of list pagination; detail lists contain at most five items. Queries
have a three-second timeout and indexes for current work and recent completions.
A loading/error state never substitutes a healthy zero. The overview needs neither
Prometheus nor Grafana and never creates demand or renews a client lease.

Context instance views show current/starting/stopping instances first, with a
compact driver/executor table for CPU and memory requests/limits. Retired instances
are independently paginated and loaded only when their collapsed history section
is opened. Historical resource values are explicitly last-recorded snapshots and
are excluded from current capacity. Stale/stopping instances show last-known
allocations; historical Spark URLs are never presented as live links.
The instance API supports `state=active` (all non-DEAD instances) and exact states;
omitting the filter preserves the previous behavior.

In Clients, the hostname appears above the client ID. The JVM client automatically
reports `HOSTNAME`, falling back to the JVM host resolver, when ensuring,
restoring or reacquiring a context. It is optional display metadata, limited to
253 UTF-8 bytes without whitespace/control characters. It does not affect context
versions, authorization or metric labels. Existing clients display **Hostname not
reported** until upgraded and attached again. Deploy the schema-10 server before
upgraded clients; older SDKs remain compatible with the new server.

## Creation, replay and cancellation

Operators create requests against an existing context version. Worker
capabilities suggest handler/version contracts; business JSON parameters remain
user-defined. The context determines ownership and result policy.

Replay opens the same editable form, defaulting to the source's exact version.
You can choose another version of the same owner's logical context, change the
payload/retry policy, and supply a new future deadline. The old job, result,
idempotency key and deadline are never changed. Each deliberate submission gets
a fresh key and ID. Network retries of that pending submission reuse the key;
conflicting edits produce a conflict rather than another acceptance.

A stopped or draining version requires explicit activation. Job creation,
wake-up, provenance and accepted audit commit together before HTTP 202. Accepted
work keeps capacity available until it finishes, even if the browser closes,
the administrator logs out or every API replica restarts. No browser client lease
is created. With no remaining SDK clients or accepted work, the context drains.
Replay executes business code again; business side effects are not exactly-once.

Cancellation uses existing attempt fencing. Queued requests cancel immediately;
running requests remain CANCELLING until worker acknowledgement or recovery.
Cancelling one request never kills its shared driver. Success committed first
remains success; cancellation committed first fences subsequent completion.
Accepted create/replay/cancel audit and provenance survive process restarts.

## Results and pagination

Result locations refer only to committed allocations: an absolute shared
filesystem path or `s3://bucket/key`. They are metadata, not public URLs or a
promise that your workstation can open them. Credentials, staging keys and
unfinished attempt outputs are excluded. Expired metadata remains inspectable;
missing storage content is reported as unavailable.

JSON preview defaults to 64 KiB and is capped at 1 MiB, with explicit truncation
and invalid-JSON state. It is rendered as escaped text. Downloads are attachments.
Datasets expose their manifest, totals and paginated published part index. Admin
lists default to 50 rows and cap at 200, with cursors bound to filters/order.
Visible pages poll every five seconds with error backoff; hidden or unauthorized
pages stop polling. Terminal request details stop periodic refresh.

The UI needs the internal operational read model but does not require metric
export, Prometheus, Grafana, or either monitoring operator. Grafana links are
optional. Build with `make build`; Go-only builds can run with UI disabled, but
UI startup fails clearly if embedded frontend assets were not built. See
[observability](observability.md) and [validation](admin-observability-validation.md)
for migration and acceptance evidence.
