# Result policy and paths

`BackendSpec.results` is snapshotted on job acceptance. It governs published
JSON, files and datasets. [Application storage](../storage.md) has a separate
lifecycle and is never deleted by job-result expiry.

`maxBytes: 0` and `retentionSeconds: 0` select their omission defaults. The enforced
result limit is **5 GiB**, including where older OpenAPI metadata describes a
larger maximum. The server rejects a limit above 5 GiB during backend ensure.


Field paths below are relative to their containing model; output fields have no caller default.

## Provider-specific requirements

| Provider | Required selection | Path behavior |
| --- | --- | --- |
| `local` | `root` must exactly match the server's configured root | Uses the default identity path; custom templates are rejected |
| `s3` | `destination` must name a configured server alias | Supports a version-1 custom relative template |

All readers/writers of local results need the same persistent shared filesystem.
An identical pathname on unrelated local disks does not provide durability.
The server records an S3 destination fingerprint; changing bucket, prefix or
endpoint under an existing alias makes retained access fail with
`DESTINATION_CHANGED`. Credentials can rotate without changing that identity.

## Path placeholders {#path-placeholders}

Default: `{contextId}/{requestId}/{attemptId}/{file}`.

| Placeholder | Meaning and placement |
| --- | --- |
| `{requestId}` | Public job ID; exactly once as a complete path segment |
| `{attemptId}` | Current attempt ID; exactly once as a complete segment |
| `{file}` | Output name; exactly once at the end, preceded by `/` |
| `{contextId}` | Optional context-version ID |
| `{submittedAt}` | Original acceptance time in UTC, default `yyyyMMddTHHmmssSSSZ` |
| `{submittedAt:yyyy/MM/dd}` | UTC date formatting with `yyyy`, `MM`, `dd`, `HH`, `mm`, `ss`, `SSS` and `/ - _ T Z` literals |

```text
exports/{submittedAt:yyyy/MM/dd}/{requestId}/{attemptId}/{file}
```

Templates and resolved names are at most 1024 UTF-8 bytes. Absolute paths, URI
schemes, empty/`.`/`..` segments, control characters, backslashes, `%`, braces
remaining after expansion, and segments starting `_linha` are rejected. Resolved
allocations are persisted; downloads do not reevaluate templates.

## ResultPolicy {#resultpolicy}

Input at `BackendSpec.results`, then retained as the job publication policy.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="resultpolicy-type"></span>`type` | string | Required; no default | Discriminator selecting the supported implementation. Values: `local`, `s3`. |
| <span id="resultpolicy-root"></span>`root` | string | Required for local | Exact configured shared-filesystem root; no inferred caller default. Not used by S3. |
| <span id="resultpolicy-destination"></span>`destination` | string | Required for S3 | Named server-configured destination; no provider credentials in job JSON. Not used by local results. |
| <span id="resultpolicy-path"></span>`path` | [PathTemplate](results.md#pathtemplate) | Optional; omitted | Relative path or path template as defined by this model; traversal is forbidden. |
| <span id="resultpolicy-maxbytes"></span>`maxBytes` | integer | `67108864` (also for `0`) | Maximum accepted result bytes, default 67108864; enforced bound 1–5368709120 (5 GiB). Zero selects default. Minimum: 1. Maximum: 5368709120. |
| <span id="resultpolicy-retentionseconds"></span>`retentionSeconds` | integer | `604800` (also for `0`) | Payload retention from successful completion, default 604800 seconds; 1–31536000. Zero selects default. Minimum: 1. Maximum: 31536000. |
| <span id="resultpolicy-destinationfingerprint"></span>`destinationFingerprint` | string | Output only | Server-computed physical destination identity; credentials are excluded. |

## PathTemplate {#pathtemplate}

Input at `ResultPolicy.path`; versioned relative result key format.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="pathtemplate-version"></span>`version` | integer | `1` (also for `0`) | Version of the associated protocol, handler or configuration. Values: `1`. |
| <span id="pathtemplate-template"></span>`template` | string | Default identity path | Relative version-1 key template; see the placeholder rules below. |
