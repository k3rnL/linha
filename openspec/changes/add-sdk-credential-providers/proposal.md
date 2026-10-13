## Why

Long-running applications need to obtain and renew API credentials without maintaining access-token files. Public API authentication also needs to be independent of browser login so many applications can use one Linha resource audience.

## What Changes

- Add generic JVM token acquisition and expiry-aware concurrent refresh with bounded HTTP calls and secret-safe errors.
- Provide OAuth 2.0 client credentials as the built-in client integration, with configured token endpoint, client ID, client-secret supplier and optional scopes.
- **BREAKING**: remove provider-specific client integrations and the mounted client JWT configuration. OAuth obtains and renews tokens; deployments mount only the client secret and optional CA.
- **BREAKING**: configure API JWT issuer, audience and TLS independently of browser OIDC. Browser authorization remains bound to its own issuer and client.
- Preserve the extensible credential-provider boundary and internal worker identity authentication.

## Capabilities

### New Capabilities

- `sdk-credential-providers`: generic expiring-token acquisition and built-in OAuth client credentials.
- `api-jwt-authentication`: independent API JWT validation with one resource audience and many application subjects.

### Modified Capabilities

- `admin-web-ui`: browser identity validation is independent of API authentication.
- `ha-deployment`: independent API and browser authentication settings in Helm.
- `oidc-tls-configuration`: separate API JWT and browser OIDC transport policies.

## Impact

Protocol and client JVM SDKs, example clients, Go authentication/configuration, Helm, tests and documentation. No database schema, job protocol, lease fencing, result publication or Spark behavior changes. Application integrations consume the generic OAuth factory; their deployment configuration is outside the public platform repository.
