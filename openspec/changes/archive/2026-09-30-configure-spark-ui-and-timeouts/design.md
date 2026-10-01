## Context

See proposal.md. Spark settings are strict JSON decoded by the Go adapter and normalized before backend hashing. Existing JVM clients send settings as JSON. SparkApplication and legacy Pod launch paths currently duplicate fixed UI and allocation configuration.

## Goals / Non-Goals

**Goals:** Typed, validated settings with consistent rendering and stable hashes.

**Non-Goals:** Arbitrary Spark configuration, automatic Ingress, History Server, changing worker business code, or forcing executors away during active work.

## Decisions

- Use `ui` with enabled and optional port, and three strings under `executors` named after Spark's controls. Keep disabled/4040 and Spark retention defaults for compatibility rather than silently changing existing workload behavior.
- Accept integer s/m/h/d durations and normalize to whole seconds. Limit to 2147483647 seconds for safe conversion in Spark. Empty means omitted. Support `infinity` for cache/shuffle by omitting their overrides; do not pass the literal to Spark's duration parser.
- Omit new default-valued fields from normalized JSON, including disabled/default-port UI and the explicit default `60s` idle timeout. This preserves historical normalized configuration bytes as well as equality of equivalent inputs. Represent custom UI port with a pointer so explicit zero is rejected.
- Share emission of scheduler, UI and allocation policy between both launch paths. Set the UI port only when UI settings are nondefault. Emit finite retention overrides only with dynamic allocation enabled. Existing renderers retain ownership of launch-specific fields.
- Existing context persistence, replacement and version hashing consume normalized JSON. There is no schema migration. A changed config creates a new version; it does not mutate existing drivers. Durable acknowledgement, ownership, retries, client and attempt leases/fencing, and result publication remain unchanged. Restart/replacement reconstruct configuration from the stored spec.
- Use raw JSON and Scala Circe examples instead of introducing a partial typed settings SDK beside the existing JSON API.

## Risks / Trade-offs

- Finite cache/shuffle retention can require recomputation → explain this in configuration documentation; preserve unlimited defaults.
- Spark UI has separate access controls from Linha API → document port-forwarding; do not add automatic public routing.
- Spark owns scheduling and actual executor removal → tests verify generated config, not a guaranteed wall-clock deletion deadline.
- Explicit worker SparkConf overrides can supersede submitted settings → document removing earlier UI/timeout workarounds.

## Migration Plan

Upgrade all server replicas before sending new fields because old strict decoders reject them. Then ensure under the same logical name with the desired settings; existing lease/drain behavior handles rolling clients. No SDK or worker rebuild is needed if the worker inherits submitted SparkConf. Drain versions containing new fields before rolling back to an old server. Validate Go adapter/rendering, persisted settings and existing HA lifecycle tests, OpenAPI and OpenSpec. No production deployment is part of this change.
