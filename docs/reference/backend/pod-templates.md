# Kubernetes Pod templates

Pod fragments live at `engine.settings.kubernetes`. Driver and executor
templates are independent. The runtime container is named `main`; you customize
its application environment and mounts while Linha/Spark own its lifecycle.
See the [Pod customization guide](../../pod-templates.md) for procedures and
[Scala helpers](../sdk/helpers.md#podtemplate) for JSON/YAML loading.


Field paths below are relative to their containing model; output fields have no caller default.

## Allowed fragments and merge behavior

Each serialized template is at most 64 KiB. `metadata` accepts only `labels` and
`annotations`. `spec` uses the Kubernetes v1 PodSpec field vocabulary from Linha's
Kubernetes client version (v0.33.1). Consult the
[versioned PodSpec definition](https://github.com/kubernetes/api/blob/v0.33.1/core/v1/types.go)
for upstream field types; the restrictions here also apply.

| Structure | Merge rule |
| --- | --- |
| Objects/maps | Recursive merge; client values override defaults |
| `containers`, `initContainers` | Recursive merge by unique `name` |
| `env`, `volumes`, `imagePullSecrets` | Replace matching entries by `name` |
| `volumeMounts` | Replace matching entries by `mountPath` |
| Other arrays | Replace the whole array |

Null values and delete directives are rejected. Administrator legacy mounts,
template defaults and client overrides form an effective snapshot for a new
context. Referenced Secret/ConfigMap contents are not hashed: use immutable names
when their contents must create a new version. Only the primary worker image is
automatically pinned to a digest; pin sidecar/init images explicitly if needed.

| Protected area | Rule |
| --- | --- |
| Top-level template | No name, namespace, owner references or other fields outside `metadata`/`spec` |
| Metadata keys | No `linha.io/`, `spark-`, `sparkoperator.k8s.io/` prefixes or `app.kubernetes.io/managed-by` |
| Pod lifecycle | No `serviceAccountName`, `serviceAccount`, `automountServiceAccountToken`, `restartPolicy`, `ephemeralContainers` or Pod-level `resources` |
| Container `main` | No `command`, `args`, `image`, `workingDir` or `restartPolicy`; CPU/memory use Spark settings |
| Runtime environment | No `LINHA_` or `SPARK_` names on `main` |
| Mounts | No overlaps with `/opt/linha`, `/var/run/linha`, `/var/run/linha-pod-templates`, `/opt/spark/conf`, service-account token mounts, including parent mounts |
| Volume names | No `linha-`, `spark-`, `kube-api-access-` prefixes |
| Extra containers | Unique valid names, image required in the effective template, image repository must be allowed |
| Pull Secrets | Valid names explicitly allowed by deployment imagePullSecrets or registrySecrets.allowedNames |

Secrets, ConfigMaps and PVCs must already exist in the deployment namespace.
Mount paths are absolute and normalized; effective mounts must reference defined
volumes. The server identifies the offending field on validation failure.

```json
{"driverPodTemplate":{"spec":{"containers":[{"name":"main","env":[{"name":"REPORT_MODE","value":"daily"}]}]}}}
```

## SparkPodTemplates {#sparkpodtemplates}

Input at `SparkSettings.kubernetes`; maps independent driver and executor fragments.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="sparkpodtemplates-driverpodtemplate"></span>`driverPodTemplate` | [PodTemplate](pod-templates.md#podtemplate) | Optional; omitted | Native driver Pod fragment; defaults to an empty application override. |
| <span id="sparkpodtemplates-executorpodtemplate"></span>`executorPodTemplate` | [PodTemplate](pod-templates.md#podtemplate) | Optional; omitted | Native executor Pod fragment; defaults to an empty application override. |

## PodTemplate {#podtemplate}

Input fragment used by SparkPodTemplates. Only metadata and spec are top-level fields.

Native Kubernetes metadata/spec fragment, max 64 KiB. Runtime container name: main. Linha/Spark managed lifecycle fields are rejected. Maps merge recursively; containers/initContainers merge by name; env/volumes/imagePullSecrets replace by name; volumeMounts replace by mountPath; other arrays replace. Null/delete directives are unsupported. See docs/pod-templates.md.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="podtemplate-metadata"></span>`metadata` | object ([fields](#podtemplate-metadata-model)) | Optional; omitted | Only application labels and annotations; reserved runtime keys are protected. |
| <span id="podtemplate-spec"></span>`spec` | object | Optional; omitted | Native Kubernetes v1 PodSpec fields subject to the managed-field rules below; arbitrary keys are not accepted. |

### metadata {#podtemplate-metadata-model}

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="podtemplate-metadata-labels"></span>`metadata.labels` | map of string | Optional; omitted | Map of application label keys to string values, subject to reserved-label validation. |
| <span id="podtemplate-metadata-annotations"></span>`metadata.annotations` | map of string | Optional; omitted | Map of application annotation keys to string values, subject to reserved-label validation. |
