## Context

See proposal.md for motivation. The console calls `/v1/admin` and authenticates
through `/ui/oidc`; cookie mutation checks use the explicit `ui.publicURL` origin.
The release already has a namespaced Service with a named `http` port. UI exposure
is currently delegated to an externally managed ingress.

## Goals / Non-Goals

**Goals:** Offer opt-in chart routing with class, annotations and explicit TLS while
keeping the canonical public URL and browser origin consistent.

**Non-Goals:** Install an ingress controller, manage DNS/certificates, expose SDK,
worker or metrics endpoints, or change SparkApplication-owned UI ingresses.
Job acknowledgement, ownership, idempotency, concurrent ensure, attempt fencing,
result publication, leases and restart recovery are unchanged.

## Decisions

- Put configuration under `ui.ingress`, disabled by default. This belongs to the
  server deployment; it adds no SDK bindings or Spark-specific behavior.
- Derive the host from `ui.publicURL` rather than accept a second host setting or
  aliases. One canonical origin avoids login/CSRF mismatches. Remove any URL port
  for the DNS host rule while preserving the configured public origin.
- Render fixed Prefix paths `/ui` and `/v1/admin` to the existing named service
  port. A `/` catch-all would also expose monitoring and SDK/worker paths; path
  rewriting would break the existing absolute frontend API and callback URLs.
- Support optional `className`, string annotations, and `tls.enabled` plus
  `tls.secretName`. TLS requires HTTPS and a Secret; HTTP ingress behind an
  upstream TLS terminator remains valid with an HTTPS public URL.
- Validate DNS host, UI enablement and TLS combinations in Helm, with schema
  types and meaningful rendering tests. No cluster-scoped permissions or server
  rebuild are required.

## Risks / Trade-offs

- Controller annotations can alter routing → document unchanged paths and Origin
  headers, and leave controller-specific authentication/TLS settings to operators.
- Certificates/controller/DNS may not exist → document prerequisites; the chart
  references a Secret but neither reads nor creates credentials/certificates.
- A single origin excludes alternate hostnames → use the configured canonical URL
  consistently, retaining externally managed routing when aliases are needed.

## Migration Plan

Upgrade the chart with existing UI/OIDC settings plus `ui.ingress.enabled: true`,
class and optional TLS Secret. Register the existing `/ui/oidc/callback` redirect.
Disabling the new flag removes only the chart-managed ingress. No database
migration, image rebuild or API protocol change is involved.
