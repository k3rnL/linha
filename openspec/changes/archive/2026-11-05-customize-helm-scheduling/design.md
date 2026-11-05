## Context

See proposal.md for motivation. The server Deployment already passes through nodeSelector and tolerations, but embeds a release-specific soft hostname spread constraint. Worker scheduling uses independent driver/executor Pod templates.

## Goals / Non-Goals

**Goals:** Preserve existing rendered scheduling defaults and accept native server scheduling structures with explicit replacement semantics.

**Non-Goals:** Change runtime behavior, worker scheduling, or install cluster infrastructure. Durable acknowledgement, ownership, retry policy, attempt fencing, result publication, and recovery continue to use the existing server and database behavior; this change only places server Pods.

## Decisions

- Add top-level `affinity: {}` and render affinity only when nonempty, preserving the existing default Pod spec.
- Add `topologySpreadConstraints: null`. Missing/null keeps the current release-specific constraint; any list is rendered verbatim, including `[]`. Avoid Helm's `default` fallback for the list, which would also replace an intentional empty list. A literal default list in values would require either templated values or implicit selector rewriting to preserve release-specific labels; the null sentinel avoids both.
- Treat override values as Kubernetes data, without Helm template evaluation or implicit selector injection. Document selectors using the existing stable server labels.
- Validate nodeSelector as a string map, tolerations as an array of objects, affinity as an object, and topologySpreadConstraints as null or an array of objects. Kubernetes validates the native nested scheduling fields; do not maintain a second PodSpec schema.
- Extend the existing Helm render suite to check default/null/empty/replacement behavior, complex affinity, malformed types, worker isolation, and selectors across release names. Document the defaults and an executable values example in the chart README and Helm reference.

## Risks / Trade-offs

- Custom hard placement rules can leave Pods pending → describe overrides as operator-controlled Kubernetes rules and preserve soft defaults.
- Null versus empty has distinct behavior → document and test both, including Helm value coalescing.
- Native nested fields are not exhaustively validated by Helm → retain upstream Kubernetes validation and test rendered structures.

## Migration Plan

Existing values files need no changes. Operators opt in with a values file; scheduling changes roll the Deployment. Removing overrides restores the original defaults. No database migration or job protocol change is required. Live restart/HA fault injection is outside this chart-only change; existing recovery contracts remain unchanged and render checks cover replacement Pod templates.
