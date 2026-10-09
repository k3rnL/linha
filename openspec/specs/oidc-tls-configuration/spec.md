# oidc-tls-configuration Specification

## Purpose

Allow operators to configure certificate verification for Linha's OIDC provider connections without weakening other authentication or transport boundaries.

## Requirements

### Requirement: Explicit OIDC TLS configuration
The Helm chart SHALL expose `security.oidc.tls.insecureSkipVerify` as a boolean defaulting to false. The server SHALL accept the equivalent `LINHA_OIDC_TLS_INSECURE_SKIP_VERIFY` environment variable, defaulting to false, and reject invalid boolean values when OIDC is enabled. Disabled security or OIDC SHALL ignore the OIDC TLS setting and SHALL NOT contact the provider.

#### Scenario: Secure default
- **WHEN** the option is omitted or false and the provider presents an untrusted HTTPS certificate
- **THEN** OIDC initialization fails certificate validation

#### Scenario: Explicit opt-in
- **WHEN** the option is true and the provider presents an untrusted certificate or a certificate for another hostname
- **THEN** Linha skips certificate-chain and hostname verification for that OIDC provider's HTTPS connections

#### Scenario: Invalid option
- **WHEN** an active OIDC configuration contains a non-boolean TLS option
- **THEN** server configuration or Helm validation rejects it

#### Scenario: Disabled identity provider
- **WHEN** security or OIDC is disabled with a leftover OIDC TLS setting
- **THEN** the server ignores that setting and makes no provider connection

### Requirement: Consistent OIDC transport policy
The OIDC TLS setting SHALL apply to discovery, signing-key retrieval and refresh, and browser OAuth token exchange. TLS verification SHALL remain enabled for unrelated HTTP clients, Kubernetes API and worker identity verification, database connections, and result storage. Initializing one replica SHALL NOT mutate transport defaults used by other clients.

#### Scenario: API authentication after initialization
- **WHEN** Linha initializes with the option enabled and later receives a signed JWT requiring a provider signing-key fetch
- **THEN** the signing keys are fetched using the configured OIDC TLS policy even after initialization has finished

#### Scenario: Browser callback on another replica
- **WHEN** browser login starts on one replica and completes on a second replica configured with the same OIDC TLS option
- **THEN** the second replica uses that policy for its token exchange and signing-key retrieval while retaining durable one-use login state

#### Scenario: Other clients remain strict
- **WHEN** an OIDC client is configured to skip verification and an unrelated client or Kubernetes worker-key verifier contacts an untrusted HTTPS server
- **THEN** the unrelated connection still rejects its certificate

#### Scenario: Restart restores the same policy
- **WHEN** a server restarts with the same OIDC TLS configuration
- **THEN** its recreated provider clients apply the same policy without changing persisted job ownership or session records

### Requirement: Identity validation remains enforced
Skipping OIDC HTTPS verification SHALL NOT disable JWT signature, trusted issuer, audience, expiry, browser nonce, or configured administrator membership checks. Public owner identity SHALL remain derived from the validated issuer and subject, preserving cross-owner isolation. Stale worker identities and attempts SHALL retain existing fencing behavior regardless of OIDC TLS configuration.

#### Scenario: Invalid token with permissive TLS
- **WHEN** the option is enabled and a token has a forged signature, wrong issuer or audience, expired timestamp, missing subject, or wrong browser nonce
- **THEN** authentication fails

#### Scenario: Cross-owner API access
- **WHEN** two validated subjects authenticate with the option enabled
- **THEN** they retain distinct stable owners and one subject's token cannot choose the other subject's owner

#### Scenario: Non-administrator browser identity
- **WHEN** a correctly signed browser token does not match configured administrator membership
- **THEN** administrative access remains forbidden

### Requirement: Deployment guidance
Operator documentation SHALL explain the option, its scope, the need for a server build containing the feature, and the alternative of trusting a mounted provider CA. It SHALL explain that bypassing verification does not change browser certificate trust. An active verification bypass SHALL produce a startup warning without logging credentials.

#### Scenario: Operator configures a private CA
- **WHEN** an operator mounts the provider CA and configures the system trust directories while leaving the option false
- **THEN** Linha can verify the provider certificate without enabling the bypass
