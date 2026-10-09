## Why

Administrator subject and claim rules currently require the same issuer already configured for OIDC. This repetition makes Helm configuration harder to read without adding another trusted identity boundary.

## What Changes

- Subject and claim mappings inherit `security.oidc.issuer` when their issuer is omitted.
- Retain compatible explicit issuer fields only when they match the configured issuer; conflicting issuers remain invalid.
- Update chart validation and public examples to omit repeated issuers while retaining deny-by-default authorization.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `admin-web-ui`: Administrator membership rules use the configured OIDC issuer by default.

## Impact

Administrator authorization/configuration, Helm values schema, deployment examples and tests. No client/worker SDK, wire protocol, persistence schema, public owner identity, or job lifecycle changes.
