# admin-ui-ingress Specification

## Purpose

Publish Linha's administrative console through an optional Helm-managed ingress
with a canonical browser origin and complete UI, login and admin API routing.

## Requirements

### Requirement: Optional administrative ingress
The Helm chart SHALL render a namespaced `networking.k8s.io/v1` Ingress only when
`ui.ingress.enabled` is true. This setting SHALL default to false and SHALL require
`ui.enabled` to be true. Operators SHALL be able to select an ingress class and
supply controller-specific string annotations without changing Linha RBAC.

#### Scenario: Existing deployment uses external routing
- **WHEN** UI ingress is disabled
- **THEN** no chart-managed Ingress is rendered and existing external routing remains usable

#### Scenario: Ingress enabled without UI
- **WHEN** `ui.ingress.enabled` is true and `ui.enabled` is false
- **THEN** chart rendering fails with a configuration error

### Requirement: Canonical same-origin browser routing
The ingress SHALL infer one concrete DNS host from `ui.publicURL` and route
exactly the `/ui` and `/v1/admin` Prefix paths to the release's existing named
HTTP service port without rewriting paths. Login callbacks, deep links, result
downloads and browser mutations SHALL preserve their original paths and origin.
The chart SHALL reject hosts that cannot be used in an Ingress rule.

#### Scenario: Browser login and result access
- **WHEN** an administrator opens the configured UI, follows its OIDC callback or fetches a published admin result
- **THEN** UI and admin requests reach the same service on the canonical host with unchanged paths

#### Scenario: Prefix boundaries
- **WHEN** the chart-managed ingress is enabled
- **THEN** its routing rules do not expose `/metrics`, health probes, ordinary SDK or worker endpoints

#### Scenario: Invalid canonical ingress host
- **WHEN** the public URL contains credentials, an IP literal, wildcard or invalid DNS host
- **THEN** chart rendering fails instead of producing an invalid ingress

#### Scenario: Existing authorization through ingress
- **WHEN** a viewer attempts a mutation or an unauthorized caller requests an admin endpoint through ingress
- **THEN** existing server-side administrator authorization remains authoritative without trusting ingress identity headers

### Requirement: Explicit TLS configuration
Operators SHALL be able to enable ingress TLS with an existing or externally
created Secret name. TLS host metadata SHALL match the canonical ingress host;
when ingress TLS is enabled, the public URL SHALL use HTTPS and a Secret name
SHALL be supplied. HTTPS public URLs with ingress TLS disabled SHALL remain
supported for TLS termination upstream of the ingress controller.

#### Scenario: TLS at the ingress controller
- **WHEN** ingress TLS is enabled with an HTTPS public URL and Secret name
- **THEN** the ingress contains that Secret and the same host as its routing rule

#### Scenario: Inconsistent TLS settings
- **WHEN** ingress TLS is enabled with an HTTP public URL or missing Secret name
- **THEN** rendering fails with an actionable configuration error
