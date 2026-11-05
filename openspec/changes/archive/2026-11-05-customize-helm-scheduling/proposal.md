## Why

The server chart hardcodes hostname topology spreading and does not expose affinity. Operators need to fit server placement to their clusters while retaining the existing default behavior.

## What Changes

- Add native server Pod affinity and topology spread values alongside the existing node selector and tolerations.
- Preserve the current release-scoped, soft hostname spread when no override is supplied; allow replacement or explicit disabling.
- Validate scheduling value types and document server scheduling separately from worker Pod templates.
- Verify default, custom, and disabled scheduling through rendered chart checks.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `ha-deployment`: Allow operators to customize server placement without changing the chart's default HA deployment behavior.

## Impact

Changes are limited to Helm values, schema, server Deployment rendering, chart checks, and deployment documentation. There are no API, SDK, engine adapter, persistence, or runtime coordination changes and no new dependencies.
