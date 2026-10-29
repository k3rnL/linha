# Scala credentials and transport

Packages `linha` and `linha.client.auth`. Application OAuth credentials, browser
OIDC and managed-worker projected credentials are independent identities. This
page covers JVM API clients; [worker bootstrap](worker.md) uses its injected
runtime identity.

## CredentialProvider {#credentialprovider}

The interface defines `bearerToken(): String`. Providers can be shared across
clients/transports. The server validates token claims; the provider only acquires
and refreshes a bearer value.

| Factory | Return type | Contract |
| --- | --- | --- |
| `CredentialProvider.none` | `CredentialProvider` | Empty token for explicitly anonymous deployments |
| `CredentialProvider.apply(get: () => String)` | `CredentialProvider` | Calls application acquisition logic per request, validates nonempty token and hides nested acquisition errors |
| `CredentialProvider.static(token: String)` | `CredentialProvider` | Fixed validated bearer token |
| `CredentialProvider.file(path: Path)` | `CredentialProvider` | Rereads mounted token on each acquisition |
| `CredentialProvider.fileSecret(path: Path)` | `() => String` | Rereads mounted OAuth client secret; pass this function to OAuth factory |
| `CredentialProvider.refreshing(acquire: () => AccessToken, clock: Clock = Clock.systemUTC(), refreshBefore: Duration = Duration.ofSeconds(30), retryDelay: Duration = Duration.ofSeconds(5))` | `CredentialProvider` | Synchronized expiry-aware acquisition; nonnegative timing arguments |

Secret files are bounded to 65,536 bytes, trimmed, nonempty and reject control
characters. Token values additionally reject embedded whitespace. Refresh occurs
on use, with no background acquisition thread. The refresh window is the smaller
of refreshBefore and one-fifth of the new token's remaining lifetime. A failed
early refresh may reuse an unexpired token until expiry; retries respect
retryDelay. An expired token is never returned as valid cached credentials.

## AccessToken and CredentialException {#accesstoken}

`AccessToken(value: String, expiresAt: Instant)` validates a bearer value and
non-null expiry. Its public members are `value` and `expiresAt`; `toString`
redacts the value. The refreshing provider rejects already-expired acquisitions.

`new CredentialException(message: String, status: Option[Int] = None)` contains
the safe message and optional provider HTTP status. Bundled acquisition code
excludes provider bodies, credentials and nested causes from these exceptions.

## OAuth client credentials {#oauth-client-credentials}

```scala
import java.nio.file.Paths
import linha.CredentialProvider
import linha.client.auth.CredentialProviders

object ApplicationCredentials {
  def provider(): CredentialProvider = CredentialProviders.oauthClientCredentials(
    "https://identity.example/token",
    "reporting-api",
    CredentialProvider.fileSecret(Paths.get("/credentials/client-secret"))
  )
}
```

`CredentialProviders.oauthClientCredentials` returns CredentialProvider with
these arguments:

| Argument | Type / default | Contract |
| --- | --- | --- |
| `tokenEndpoint` | `String`, required | Absolute HTTPS URL, no embedded credentials or fragment |
| `clientId` | `String`, required | Nonempty OAuth service client identifier |
| `clientSecret` | `() => String`, required | Evaluated on token acquisition; supports mounted secret rotation |
| `scopes` | `Seq[String]`, `Seq.empty` | Valid nonempty OAuth scope tokens, without quotes/backslash/whitespace |
| `options` | `IdentityHttpOptions`, defaults below | Applies only to this identity HTTP client |
| `clock` | `Clock`, `Clock.systemUTC()` | Acquisition/expiry clock, injectable for tests |

Uses `grant_type=client_credentials` and HTTP Basic client authentication. Token
responses require Bearer token_type and expires_in from 1 to 31,536,000 seconds.
Redirects are rejected. Secret retrieval does not require a pre-generated JWT.

## IdentityHttpOptions {#identityhttpoptions}

Public case class in `linha.client.auth`:

| Field | Type | Default | Contract |
| --- | --- | --- | --- |
| `caFile` | `Option[Path]` | `None` | PEM certificate trust store for this identity client; replaces its default trust configuration |
| `insecureSkipVerify` | `Boolean` | `false` | Per-client certificate/hostname bypass; mutually exclusive with caFile |
| `allowHttp` | `Boolean` | `false` | Permit HTTP only when explicitly selected, for isolated fixtures |
| `connectTimeout` | `Duration` | 5 seconds | Positive connection timeout |
| `requestTimeout` | `Duration` | 10 seconds | Positive whole-response timeout including body |
| `maxResponseBytes` | `Int` | 1,048,576 | Response bound, 1–16,777,216 bytes |

Invalid options/scopes raise IllegalArgumentException; acquisition/network/expiry
failures use CredentialException. Server TLS switches do not configure JVM
client trust and client identity options do not alter Linha server validation.

## Transport and LinhaException {#transport}

`LinhaException(status: Int, code: String, detail: String)` is the public case-class
HTTP/SDK exception; its message is `code + ": " + detail`. See
[error codes](../http/errors.md).

The low-level constructor is
`new Transport(baseUrl: String, credentials: CredentialProvider, headers: Map[String,String] = Map.empty, timeoutSeconds: Long = 40, retries: Int = 2)`.
Public `baseUrl` retains the API base. Requests require paths under `/v1/`.
Connection timeout is five seconds. The default gives up to three transport
attempts for IOException, HTTP 503 or HTTP 429, with 100 ms then 200 ms delay;
this does not change accepted execution maxAttempts.

| Method | Return type | Contract |
| --- | --- | --- |
| `call(method: String, path: String, body: Json = Json.obj(), extraHeaders: Map[String,String] = Map.empty)` | `Json` | JSON request/response; HTTP 204 becomes Json.Null |
| `upload(path: String, file: Path, attemptId: String, fence: Long)` | `Json` | Fenced worker result upload |
| `put(path: String, file: Path, extraHeaders: Map[String,String] = Map.empty)` | `Json` | Byte upload with accurate Content-Length |
| `open(path: String)` / `open(path: String, extraHeaders: Map[String,String])` | `InputStream` | Caller must close stream; stream reads are not automatically replayed |

Client lease maintenance uses a five-second request timeout without generic
transport retries. Application storage uses a 600-second request timeout. These
are runtime choices, not options on the LinhaClient constructor.
