# Customize driver and executor Pods

The [Pod-template reference](reference/backend/pod-templates.md) lists complete
fragment, merge and protected-field rules. The
[Scala helper reference](reference/sdk/helpers.md#podtemplate) lists every builder
and JSON/YAML loading method.

Pass native Kubernetes fragments through
`engine.settings.kubernetes.driverPodTemplate` and `executorPodTemplate` when
ensuring a backend. Use this for configuration files, Secret references, private
registry pulls, scheduling, sidecars and init containers.

Resources you reference must already exist in the Linha namespace. Templates
contain `metadata` labels/annotations and `spec`; omit Pod names, namespaces,
`apiVersion` and `kind`.

## Add files, environment and pull Secrets

```scala
import linha.client._
import io.circe.Json
import io.circe.syntax._

val shared = PodTemplate.empty
  .withConfigMap("app-config", "reporting-config-v1", "/etc/reporting")
  .withSecretEnv("APP_PASSWORD", "reporting-credentials", "password")
  .withImagePullSecret("worker-registry")

val templates = SparkPodTemplates(
  driver = shared.withEnv("APP_ROLE", "driver"),
  executor = shared.withEnv("APP_ROLE", "executor")
)
val settings = templates.withSettings(Json.obj(
  "application" -> Json.obj(
    "mainClass" -> "example.WorkerMain".asJson,
    "mainApplicationFile" -> "local:///opt/linha/worker.jar".asJson
  ),
  "drivers" -> Json.obj("minDrivers" -> 1.asJson, "maxDrivers" -> 2.asJson),
  "executors" -> Json.obj("instances" -> 2.asJson)
))
```

Put `settings` inside the Spark engine spec. Driver and executor templates are
independent; apply shared values to both when Spark tasks need them.
Helpers also support Secret/PVC volumes, JSON, YAML and files. YAML is parsed
locally and sent as JSON; file paths and callbacks are not transmitted.

The raw JSON equivalent belongs under `engine.settings`:

```json
{
  "kubernetes": {
    "driverPodTemplate": {
      "spec": {
        "imagePullSecrets": [{"name": "worker-registry"}],
        "volumes": [{"name": "app-config", "configMap": {"name": "reporting-config-v1"}}],
        "containers": [{
          "name": "main",
          "volumeMounts": [{"name": "app-config", "mountPath": "/etc/reporting", "readOnly": true}]
        }]
      }
    }
  }
}
```

Use `main` for the Spark main container in public templates. Linha translates it
to the driver/executor names used by Spark.

## Permit private registry Secrets

```yaml
registrySecrets:
  allowedNames: [worker-registry]
workerPodTemplates:
  driverPodTemplate:
    spec:
      nodeSelector: {workload: spark}
  executorPodTemplate:
    spec:
      nodeSelector: {workload: spark}
```

Create the selected registry Secret as `kubernetes.io/dockerconfigjson` or
`kubernetes.io/dockercfg`. The server Role permits `get` only on approved registry
Secret names for image digest resolution. The names, not credential values, enter
the persisted spec. Kubelet uses the same Secret for pulling.

Top-level `imagePullSecrets` also apply to the server and become worker defaults;
those names are automatically permitted. `registrySecrets.allowedNames` only
permits selection and does not attach the Secret. Use explicit registry/repository
auth keys; wildcard keys are not supported by Linha's resolver.

## Understand merge and protected fields

Deployment defaults are merged first, then the client template. Objects merge
recursively. Containers, init containers, env, volumes and image pull Secrets
merge by name; mounts merge by `mountPath`. Other arrays replace. Empty named
arrays preserve inherited entries. Null/delete/JSON-patch directives are unsupported.

The normalized effective template is saved with the context version. Changing
Helm defaults affects future ensures; existing versions and replacements keep
their saved template. Use versioned immutable ConfigMaps/Secrets for reproducibility.

Linha reserves Pod identity, service accounts, ownership labels, worker token
mounts, restart policy and the main image/command/args/working directory.
`LINHA_*` and `SPARK_*` env names and runtime mount paths are protected. Set main
CPU and memory through [Spark settings](reference/backend/spark.md), not the
container template. Sidecars and init images follow the image allow-list too.

Each template is limited to 64 KiB, including after merging. Conflicting managed
fields return a validation error. Kubernetes admission and scheduling errors
remain visible through context conditions.
