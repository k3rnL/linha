# S3 destinations and administrator membership

These models configure the server. Applications select destination aliases and
submit [backend declarations](../backend/index.md); they do not supply provider
credentials or administrator membership.

## S3Config {#s3config}

Each value of `LINHA_S3_DESTINATIONS` is a destination configuration. Helm loads
this JSON from `s3.secretRef` and `s3.destinationsKey`. Credentials use the server
AWS default chain. The provider and its bucket are provisioned externally.

```json
{
  "analytics": {
    "bucket": "example-results", "prefix": "linha", "region": "eu-west-1"
  },
  "application-data": {
    "bucket": "example-inputs", "prefix": "processing", "region": "eu-west-1",
    "allowWorkloadAccess": true
  }
}
```

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="s3config-bucket"></span>`bucket` | string | Required | Nonempty bucket name without `/` or backslash |
| <span id="s3config-prefix"></span>`prefix` | string | Empty | Relative base prefix, validated as a safe name; one trailing `/` is removed |
| <span id="s3config-endpoint"></span>`endpoint` | string | AWS endpoint resolution | Optional S3-compatible service endpoint |
| <span id="s3config-region"></span>`region` | string | `us-east-1` | Destination-specific SDK region |
| <span id="s3config-pathstyle"></span>`pathStyle` | boolean | `false` | Enable path-style bucket addressing for compatible stores |
| <span id="s3config-datasetrolearn"></span>`datasetRoleArn` | string | Unset | Role assumed for scoped distributed dataset writes; required for S3 datasets |
| <span id="s3config-stsendpoint"></span>`stsEndpoint` | string | AWS endpoint resolution | Optional compatible STS endpoint for dataset delegation |
| <span id="s3config-allowworkloadaccess"></span>`allowWorkloadAccess` | boolean | `false` | Permit contexts to request application I/O across this entire destination prefix |

Result identity hashes bucket, normalized prefix and endpoint. Credential rotation
does not change it. Use a new alias when moving storage locations. Ordinary
JSON/file results do not require STS. Distributed datasets require AssumeRole
with enforced session policies, an allocation-scoped staging prefix and grants
lasting at most one hour without mid-write renewal. Unsupported delegation
returns `DATASET_UNSUPPORTED`; see [dataset setup](../../dataset-results.md).

## AdminMappings {#adminmappings}

Input under Helm `security.admin` or standalone `LINHA_ADMIN_MAPPINGS`. Browser
OIDC issuer is authoritative. Membership is checked again on credential refresh;
removing membership can end access before the absolute browser lifetime.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="adminmappings-viewer"></span>`viewer` | [RoleRules](#rolerules) | Empty | Read-only administrator membership |
| <span id="adminmappings-operator"></span>`operator` | [RoleRules](#rolerules) | Empty | Administrator mutation membership; takes precedence over viewer |

## RoleRules {#rolerules}

Any matching subject or claim rule grants the containing role.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="rolerules-subjects"></span>`subjects` | array of [SubjectRule](#subjectrule) | `[]` | Explicit subjects |
| <span id="rolerules-claims"></span>`claims` | array of [ClaimRule](#claimrule) | `[]` | Nested claim membership |

## SubjectRule {#subjectrule}

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="subjectrule-subject"></span>`subject` | string | Required | Nonempty browser identity subject |
| <span id="subjectrule-issuer"></span>`issuer` | string | Browser issuer | Optional explicit value must match configured browser issuer |

## ClaimRule {#claimrule}

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="claimrule-path"></span>`path` | array of strings | Required | 1–8 nested keys; each nonempty, at most 128 bytes, not `__proto__` |
| <span id="claimrule-values"></span>`values` | array of strings | Required | Nonempty set of allowed string values |
| <span id="claimrule-issuer"></span>`issuer` | string | Browser issuer | Optional explicit value must match configured browser issuer |

```json
{"viewer":{"claims":[{"path":["groups"],"values":["linha-viewers"]}]},"operator":{"subjects":[{"subject":"operator-subject"}]}}
```

Use [administrator setup](../../admin-ui.md) for browser client registration,
provider scopes, ingress and diagnostics.
