## Context

See proposal.md for motivation. The server supports one OIDC issuer. Both constructor validation and role evaluation currently repeat an issuer check per rule; the chart requires that field in every rule. Sessions are durable and their roles are reevaluated on each request.

## Goals / Non-Goals

**Goals:** Make the configured issuer the only required issuer setting and preserve compatibility with valid existing charts.

**Non-Goals:** No multiple-provider support, default administrator grants, SDK changes or database migrations.

## Decisions

- Accept omitted rule issuers as inherited from the configured issuer. Preserve optional explicit fields for compatibility, rejecting conflicting values. Removing fields entirely would cause unnecessary upgrade breakage and hide configuration mistakes.
- Keep the existing top-level issuer/subject check in role evaluation. Optional explicit per-rule issuers can only narrow that check. This preserves denial for unknown issuers and missing subjects, including retained sessions.
- Remove issuer from Helm schema required arrays and public examples; retain its optional string property. Avoid rewriting already archived historical specifications or old immutable release examples.
- Job acknowledgement, owner hashes, retries, lease fencing, result publication and restart recovery remain unchanged. Browser sessions need no migration: existing claims are reevaluated with equivalent rules on any replica.
- Validate both rule types for viewer/operator roles with omitted, matching and conflicting issuers, unknown issuers and empty subjects. Exercise issuer-inheriting browser login across replicas and after administrator-service recreation using independent PostgreSQL pools.

## Risks / Trade-offs

- [Issuer omission might accidentally allow another provider] → Always require token/session issuer to match the global configured issuer before checking membership.
- [Upgrade compatibility] → Keep explicit matching legacy issuers supported; older servers still require repeated fields, so upgrade the server before removing them.

## Migration Plan

Upgrade the chart and server, then omit repeated issuer fields under security.admin.viewer/operator subjects/claims. Existing explicit matching configurations remain valid. For rollback to an older server, restore the repeated issuer fields.
