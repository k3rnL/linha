## Context

SparkApplication roots use immutable instance IDs for discovery and cleanup. The operator reports the actual driver name in status, but worker authentication additionally recognizes only hard-coded ID-based names. See proposal.md for motivation.

## Goals / Non-Goals

Provide readable driver names through operator submission while retaining full instance identity. Root resource naming, executor naming, stored context configuration, acceptance, retries and result handling are outside this change.

## Decisions

- Use `<context-slug>-<full-instance-id>-driver`. Reserving the full 32-character ID plus delimiters and role suffix leaves 23 characters for the context in a 63-character DNS label, sufficient for common context names. A branded prefix would reduce that readable budget; shortening IDs would sacrifice uniqueness.
- Supply the supported SparkApplication `driver.podName` field. Keep SparkApplication roots stable, so existing inventory and UID-fenced deletion continue working. Existing resources are not patched during idempotent ensure; observations continue using the Pod name reported by the operator.
- Share Kubernetes driver naming/validation helpers between submission and authentication. Authentication accepts a valid DNS-label name with a nonempty context prefix and the exact full instance suffix, still requiring Spark driver role and existing token/instance UID fences. Named drivers additionally read the current owned SparkApplication and match its configured Pod name, UID, context and instance labels; a forged child with the same suffix is rejected. Pod name remains a readable identifier, not independent authorization.
- No schema or protocol change. Submission acknowledgement, ownership, retries, client leases, attempt fences and result publication keep their existing semantics. Different instances across owners, versions and replacements have distinct full IDs.

## Risks / Trade-offs

- Context names may normalize to the same slug or be truncated → retain the full unique instance suffix; keep the full logical name in the context view.
- A name change could reject worker registration → test both original and named drivers plus rejected impersonation cases.
- Existing drivers remain on their original names → document that new names apply when an instance is provisioned, without disrupting accepted jobs.

## Migration Plan

Deploy the updated server. New managed Spark instances receive readable names; existing applications continue normally. Reverting the server requires retiring newly named drivers first, because older binaries recognize only their original naming pattern. No database migration is required.
