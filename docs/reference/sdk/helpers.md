# Scala Pod and result-path helpers

These helpers produce the same JSON as the language-independent
[backend reference](../backend/index.md). They do not introduce another backend
model or upload local Pod-template paths to the server.

## PodTemplate {#podtemplate}

Package `linha.client`. Immutable wrapper with public `json: Json`.

| Factory / member | Return type | Contract |
| --- | --- | --- |
| `PodTemplate.empty` | `PodTemplate` | Empty object |
| `PodTemplate.maxBytes` | `Int` | 65,536 serialized bytes |
| `PodTemplate.fromJson(json: Json)` | `PodTemplate` | Validates object shape, size, list identities and null/nesting rules |
| `PodTemplate.fromYaml(yaml: String)` | `PodTemplate` | Safe client-side YAML parsing; duplicate keys, arbitrary tags and aliases are rejected |
| `PodTemplate.fromFile(path: Path)` | `PodTemplate` | Reads JSON/YAML content, at most 64 KiB; sends the content, not the path |
| `merge(overrides: PodTemplate)` | `PodTemplate` | New merged value with the [wire merge rules](../backend/pod-templates.md#allowed-fragments-and-merge-behavior) |
| `withImagePullSecret(name: String)` | `PodTemplate` | Adds a named permitted Secret |
| `withEnv(name: String, value: String)` | `PodTemplate` | Environment value on container main |
| `withSecretEnv(name: String, secret: String, key: String)` | `PodTemplate` | SecretKeyRef environment value on main |
| `withConfigMap(name: String, configMap: String, mountPath: String)` | `PodTemplate` | Read-only ConfigMap volume and main mount |
| `withSecretVolume(name: String, secret: String, mountPath: String)` | `PodTemplate` | Read-only Secret volume and main mount |
| `withPVC(name: String, claim: String, mountPath: String, readOnly: Boolean = false)` | `PodTemplate` | PVC volume and main mount |

Parsing/shape errors raise IllegalArgumentException or parser errors; nesting
depth must remain below 48. Server validation is still authoritative for
protected lifecycle fields, effective volume references, image policy and Secret
permissions. Environment names such as `LINHA_...` remain protected.

## SparkPodTemplates {#sparkpodtemplates}

`SparkPodTemplates(driver: PodTemplate = PodTemplate.empty, executor: PodTemplate = PodTemplate.empty)`
is a public case class in `linha.client`.

`json: Json` yields `{"driverPodTemplate":...,"executorPodTemplate":...}`.
`withSettings(settings: Json): Json` requires an object and sets its `kubernetes`
member to this pair; it replaces an existing kubernetes member rather than
merging with that member.

```scala
import io.circe.Json
import linha.client.{PodTemplate, SparkPodTemplates}

object TemplateExample {
  def settings(base: Json): Json = SparkPodTemplates(
    driver = PodTemplate.empty.withEnv("REPORT_MODE", "daily")
  ).withSettings(base)
}
```

The server merges deployment defaults and client overrides when creating a new
immutable version. Changing referenced ConfigMap/Secret contents alone does not
change that version; choose immutable object names for reproducible rollouts.

## ResultStorage {#resultstorage}

Object in `linha.client`, all functions return Json:

| Signature | Contract |
| --- | --- |
| `Local(root: String)` / `local(root: String)` | Local result policy; root must match configured shared root |
| `S3(destination: String, path: ResultPath = ResultPath.template(ResultPath.defaultTemplate))` | S3 alias with validated version-1 path |
| `s3(destination: String, path: String)` | Lowercase helper with explicit template string; server validates the resulting policy |

MaxBytes and retention can be set on the resulting JSON according to
[ResultPolicy](../backend/results.md#resultpolicy); there are no extra typed
constructor arguments for them.

## ResultPath and PathVariables {#resultpath}

Package `linha`. `ResultPath.template(text: String): ResultPath` validates a
version-1 template. Public members: `version: Int`, `template: String` and
`resolve(values: PathVariables): String`.

`PathVariables(contextId: String, requestId: String, attemptId: String, file: String, submittedAt: Instant)`
provides expansion values; submittedAt is acceptance time in UTC. Context,
request and attempt IDs must each be a single safe segment; file can be a safe
relative name. `ResultPath.defaultTemplate` is
`{contextId}/{requestId}/{attemptId}/{file}`.

`ResultPath.validateName(name: String): Unit` validates relative names. Template,
name and UTC token rules match [path placeholders](../backend/results.md#path-placeholders).
Invalid paths raise IllegalArgumentException. The server persists the actual
allocation; local resolve is not a way to authorize an arbitrary storage path.
