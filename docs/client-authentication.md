# Configure OAuth 2.0 client credentials

Use an OAuth 2.0 service client to authenticate an application to Linha. The SDK
obtains and replaces access tokens itself. Mount the OAuth **client secret**, not
a pre-generated JWT. No token file or identity-provider-specific login adapter is
required.

This guide assumes an OAuth authorization server that issues signed JWT access
tokens and publishes OIDC discovery and signing keys. Linha does not accept opaque
access tokens. For the identity model, see [authentication boundaries](architecture.md#authentication-boundaries).

## 1. Register an application identity

Create a confidential client with the `client_credentials` grant enabled.
Give every independent application its own client ID and secret. Replicas of the
same application should share that identity so they can retrieve the same jobs.

Configure the access token's audience to include `linha`. This is the identity of
the target API; it is shared by all applications calling Linha. The client ID
identifies the caller and does not replace this audience.

Find the exact issuer and token endpoint in the provider's discovery document.
The issuer URL and token endpoint serve different purposes.

## 2. Configure Linha's API verifier

```yaml
security:
  enabled: true
  api:
    enabled: true
    jwt:
      issuer: https://identity.example/realms/jobs
      audience: linha
      tls:
        insecureSkipVerify: false
```

Linha checks the token signature, issuer, audience, expiry and nonempty subject.
Owners are derived from the verified issuer and subject. A correctly signed token
intended for another service is rejected.

Browser administrator login is separate: it uses `security.oidc`,
`ui.oidc.clientId` and `security.admin`. API tokens do not automatically grant
administrator access. See [enable the console](admin-ui.md).

## 3. Construct the SDK provider

<!-- development-only:start -->
Install the development SDKs from this checkout with Java 17 `mvn -B install`.
<!-- development-only:end -->

Use the matching client and protocol artifacts:

```scala
import linha.CredentialProvider
import linha.client.LinhaClient
import linha.client.auth.CredentialProviders
import java.nio.file.Paths
import scala.concurrent.ExecutionContext.Implicits.global

val credentials = CredentialProviders.oauthClientCredentials(
  tokenEndpoint = "https://identity.example/realms/jobs/protocol/openid-connect/token",
  clientId = "reporting-api",
  clientSecret = CredentialProvider.fileSecret(Paths.get("/credentials/client-secret")),
  scopes = Seq.empty
)
val client = new LinhaClient("https://linha.example", credentials)
```

The secret supplier is called when a token is acquired, so a refreshed Secret file
can be used without restarting the SDK. The SDK sends HTTP Basic client
authentication and `grant_type=client_credentials`. Request scopes only when the
provider requires them; `openid` is not required by Linha's machine API.

The provider is shared by job submission, context heartbeats, status polling and
result downloads. Keep it, the client and its contexts for the application process
lifetime. Close the client during graceful shutdown.

## 4. Check the Keycloak settings

For Keycloak, enable **Client authentication** and **Service accounts roles** on
an OpenID Connect client. Create an audience mapper or client scope that adds
`linha` to the **access token** audience. Attach that scope to each service client
that may call Linha. The service-account subject gives each client a distinct owner.

Typical URLs are:

```text
Issuer:         https://identity.example/realms/jobs
Token endpoint: https://identity.example/realms/jobs/protocol/openid-connect/token
```

An installation configured with a context path might include `/auth` before
`/realms`. Use the URLs in discovery rather than guessing the path. The realm URL
alone is not a token endpoint. Keycloak documents
[service accounts and audiences](https://www.keycloak.org/docs/latest/server_admin/index.html).

## 5. Configure TLS when needed

For a private CA, mount its PEM file and configure this one provider:

```scala
import linha.client.auth.IdentityHttpOptions

val options = IdentityHttpOptions(
  caFile = Some(Paths.get("/credentials/identity-ca.crt"))
)
```

Pass `options = options` to `oauthClientCredentials`. Verification is enabled by
default. `insecureSkipVerify = true` explicitly skips certificate and hostname
verification for this provider only. `allowHttp = true` permits HTTP for an isolated
fixture. Neither option changes TLS settings for the Linha HTTP transport.

## Refresh and failure behavior

Tokens refresh on the next SDK call near expiry; active context heartbeats also
make those calls. Concurrent requests share one acquisition. If refresh fails,
an unexpired cached token can still be used, with a five-second acquisition
backoff. An expired token is never returned. Credentials need no background thread.

Errors report the operation and, when available, the HTTP status. They exclude
provider response bodies, tokens and secrets. The SDK does not retry a job
mutation automatically after an authentication failure.

For defaults and limits, see [SDK credential reference](sdk-reference.md#oauth-credentials).
For `401` and `403` diagnosis, see [job and authentication failures](job-failures.md).
