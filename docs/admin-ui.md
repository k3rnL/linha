# Enable the administrator console

The console displays contexts, clients, workers, requests, attempts and retained
results. Operators can create or edit-and-replay requests, cancel accepted work and
move pending requests to a corrected context version.
It is embedded in the server image and disabled by default.

This guide assumes a reachable Linha server and an OIDC provider. API OAuth
configuration remains independent; see [client authentication](client-authentication.md).

## 1. Register a browser client

Create a separate OpenID Connect client, such as `linha-admin`, using authorization
code with PKCE. Register the exact callback URL:

```text
https://linha.example/ui/oidc/callback
```

Use the provider's issuer URL for `security.oidc.issuer`. If the browser client is
confidential, store its secret in a Secret in the release namespace. A public
client can omit `ui.oidc.secretRef`.

## 2. Configure administrator groups

Add a provider mapper that includes group membership in the **ID token**. Match
the exact emitted values, including any group path prefix. For example:

```yaml
security:
  oidc:
    enabled: true
    issuer: https://identity.example/realms/jobs
  admin:
    viewer:
      claims:
        - path: [groups]
          values: [linha-viewers]
    operator:
      claims:
        - path: [groups]
          values: [linha-operators]
ui:
  enabled: true
  publicURL: https://linha.example/ui/
  sessionSeconds: 3600
  oidc:
    clientId: linha-admin
    scopes: [openid, profile, email, groups]
    secretRef: linha-browser-client
    clientSecretKey: client-secret
```

Add a `groups` scope only if the provider supports it. Alternatively attach a
default group mapper and use the provider's ordinary browser scopes. Keycloak's
Group Membership mapper can emit a claim such as `groups`; configure **Add to ID
token** and choose whether values include full group paths.

Rules belong under `security.admin`, alongside `security.oidc`. They inherit the
browser issuer; do not repeat it. Claim paths support nested objects and values
that are strings or arrays of strings. Operators can also view. No administrator
is granted access by default.

## 3. Expose the same origin

```yaml
ui:
  publicURL: https://linha.example/ui/
  ingress:
    enabled: true
    className: nginx
    tls:
      enabled: true
      secretName: linha-ui-tls
```

Install the ingress controller, configure DNS and provide its TLS Secret.
The chart infers the host from `ui.publicURL` and routes `/ui` and `/v1/admin`
to the existing Service. Preserve these paths; no rewriting or sticky sessions
are required. If TLS terminates upstream, retain the external HTTPS public URL
and set ingress TLS to false. An externally managed ingress can use the same routes.

## 4. Verify the console

- **Overview:** current work, capacity, health and recent activity.
- **Contexts:** logical names and immutable versions, clients with hostnames,
  worker instances, current allocations and separated historical observations.
- **Requests:** payloads, state, progress, attempts, failures, cancellation and
  links to retained results. Edit-and-replay opens the create form.
- **Servers:** replica readiness, versions and heartbeat freshness.
- **Create request:** select a context version, edit JSON, set policy and submit.

Spark instances link to a current UI ingress when the SparkApplication exposes
one. Otherwise use the displayed service/port and
[port forwarding](versioned-spark-applications.md#open-a-driver-ui).
Historical instances do not imply that their UI is still reachable.

File results expose names, sizes, digests and locations, with bounded previews
when supported. Dataset parts are paginated. Operators have actions; viewers
have inspection access. Actions are recorded in the persisted audit history.

## Recover a stranded queue

Use this procedure when an old context version is draining but its workers cannot
start. First ensure that the corrected version has working workers and supports
the queued handlers and payload contracts.

1. Open **Requests**, or open the old context version and choose its **Requests** tab.
2. Use the context suggestions and local date/time inputs to locate the work.
3. Select the queued or retrying requests. The header checkbox selects eligible
   rows on the current page; repeat for further pages as needed.
4. Choose **Move selected**, search the destination versions, and choose the
   corrected active version. Confirm the move.
5. Review **Batch results**. Successful rows move immediately; failed rows remain
   selected with an explanation. Refresh and retry those rows if appropriate.

The destination must have the same owner, logical context name, engine type and
engine version. It must retain the source's workload storage destinations and
fingerprints. Stopped and draining versions cannot receive moved work.

A move preserves public request IDs, payloads, submission time, deadline, retry
count/backoff and the original result-storage policy. Clients keep polling the
same IDs. The source can finish draining once its remaining accepted work ends.
Each actual move appears in the request's administrative audit with links to the
source and destination versions.

Running requests cannot move. Choose **Cancel selected** to signal running work or
cancel pending work; cancellation leaves drivers available for other requests.
If a payload needs editing, use **Edit and replay** to create a new request and
cancel the obsolete request instead of moving it unchanged.

Batch actions apply per request. A worker can claim a row while the dialog is open;
that row then reports a conflict instead of being executed twice. The API accepts
up to 200 unique IDs per batch; the UI selects one page at a time. See the
[OpenAPI contract](http-api.md) for `POST /v1/admin/jobs/batch`.

## Keep browser sessions connected

Linha renews short-lived identity credentials on the server when the provider
issues a refresh token. Refresh credentials remain encrypted in PostgreSQL;
JavaScript and browser cookies never contain provider tokens. Any replica can
renew the same session, and current administrator membership is checked again.

Set the maximum browser session duration explicitly. For example, for eight hours:

```yaml
ui:
  sessionSeconds: 28800
```

The default is 3600 seconds; the supported range is 60–86400 seconds. Renewal does
not extend this absolute limit. The provider's own session expiry or revocation
can end access earlier. Sign in once after upgrading: sessions created by older
servers have no refresh credential and retain their original expiry.

For Keycloak, enable the browser client's **Standard flow** and align its client
and realm session timeouts with the duration you need. Ordinary interactive
renewal does not require `offline_access`. Other providers may require an extra
scope or consent to issue a refresh token; use `ui.oidc.scopes` when needed.
If refreshed ID tokens are omitted, Linha uses fresh UserInfo and verifies its
subject, so UserInfo must include the claims used by administrator mappings.
See the [Keycloak session documentation](https://www.keycloak.org/docs/26.7.5/server_admin/index.html#_timeouts)
and [OpenID Connect refresh rules](https://openid.net/specs/openid-connect-core-1_0.html#RefreshTokens).

A temporary provider outage does not extend an expired identity. Rejected refresh
credentials, changed identity, removed administrator membership, or an interrupted
provider-token rotation can require another login. Logout removes the shared
session so another replica cannot revive it.

## Diagnose login

| Symptom | Check |
| --- | --- |
| Provider rejects the callback | Exact registered callback, browser client flow and PKCE settings |
| `administrator role required` | `security.admin` location, emitted ID-token claim path and exact group values |
| Groups missing | Provider mapper includes the ID token; requested scope is supported |
| Invalid audience | Browser ID token targets `ui.oidc.clientId`, not API audience `linha` |
| Certificate error | Provider CA trusted by server and browser; these are separate trust stores |
| Login needed at each short token expiry | Sign in after upgrading; verify the provider issues a refresh token and the browser client can refresh |
| Login needed at a fixed longer interval | Check `ui.sessionSeconds` and the provider's maximum session duration |

Log in again after changing scopes or the provider's token mapper. Administrator
mappings are evaluated using the running server configuration on every request.
The browser session is database-backed, HttpOnly, SameSite=Lax and Secure over
HTTPS. Its absolute lifetime is bounded by `ui.sessionSeconds`. Without a refresh
token, it also ends at identity-token expiry. JavaScript never stores bearer tokens.

For an isolated development fixture, `security.enabled: false` with
`ui.enabled: true` gives anonymous operator access. It provides no user isolation.
