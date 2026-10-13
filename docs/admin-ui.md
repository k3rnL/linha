# Enable the administrator console

The console displays contexts, clients, workers, requests, attempts and retained
results. Operators can create or edit-and-replay requests and cancel accepted work.
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

## 4. Verify the four pages

- **Overview:** current work, capacity, health and recent activity.
- **Contexts:** logical names and immutable versions, clients with hostnames,
  worker instances, current allocations and separated historical observations.
- **Requests:** payloads, state, progress, attempts, failures, cancellation and
  links to retained results. Edit-and-replay opens the create form.
- **Create request:** select a context version, edit JSON, set policy and submit.

Spark instances link to a current UI ingress when the SparkApplication exposes
one. Otherwise use the displayed service/port and
[port forwarding](versioned-spark-applications.md#open-a-driver-ui).
Historical instances do not imply that their UI is still reachable.

File results expose names, sizes, digests and locations, with bounded previews
when supported. Dataset parts are paginated. Operators have actions; viewers
have inspection access. Actions are recorded in the persisted audit history.

## Diagnose login

| Symptom | Check |
| --- | --- |
| Provider rejects the callback | Exact registered callback, browser client flow and PKCE settings |
| `administrator role required` | `security.admin` location, emitted ID-token claim path and exact group values |
| Groups missing | Provider mapper includes the ID token; requested scope is supported |
| Invalid audience | Browser ID token targets `ui.oidc.clientId`, not API audience `linha` |
| Certificate error | Provider CA trusted by server and browser; these are separate trust stores |

Log in again after changing scopes or the provider's token mapper. Administrator
mappings are evaluated using the running server configuration on every request.
The browser session is database-backed, HttpOnly, SameSite=Lax and Secure over
HTTPS. Its lifetime is bounded by the configured session duration and ID-token
expiry. JavaScript never stores bearer tokens.

For an isolated development fixture, `security.enabled: false` with
`ui.enabled: true` gives anonymous operator access. It provides no user isolation.
