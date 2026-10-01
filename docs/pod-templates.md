# Customize Spark Pods

Pass `engine.settings.kubernetes.driverPodTemplate` and `executorPodTemplate`
when ensuring a backend. Each is a native Kubernetes **fragment** containing
`metadata` (labels/annotations) and `spec`. Omit `apiVersion`, `kind`, Pod names
and namespaces. Spark Operator submits the application and Spark creates driver/executor Pods.
Use `main` in input templates; runtime names are `spark-kubernetes-driver` and
`spark-kubernetes-executor`. See [launch and lifecycle settings](versioned-spark-applications.md).

You can provide ConfigMap/Secret/PVC volumes, mounts, environment variables,
image pull Secret references, node selectors, affinity, tolerations, init
containers and sidecars. Resources you reference must already exist in the
Linha release namespace. Linha never copies or changes their contents.

## Scala client

```scala
import linha.client._
import io.circe.Json
import io.circe.syntax._

val shared = PodTemplate.empty
  .withConfigMap("app-config", "metoc-config-v1", "/etc/metoc")
  .withSecretEnv("APP_PASSWORD", "metoc-app", "password")
  .withImagePullSecret("metoc-registry")

val pods = SparkPodTemplates(
  driver = shared.withEnv("APP_ROLE", "driver"),
  executor = shared.withEnv("APP_ROLE", "executor")
)
val settings = pods.withSettings(Json.obj(
  "application" -> Json.obj("mainClass" -> "example.WorkerMain".asJson,
    "mainApplicationFile" -> "local:///opt/linha/worker.jar".asJson),
  "drivers" -> Json.obj("minDrivers" -> 1.asJson, "maxDrivers" -> 2.asJson),
  "executors" -> Json.obj("instances" -> 2.asJson)
))
val spec = Json.obj(
  "image" -> "registry.example/metoc/worker:1".asJson,
  "engine" -> Json.obj("type" -> "spark".asJson,
    "version" -> "3.5.6".asJson, "settings" -> settings),
  "results" -> ResultStorage.Local("/var/lib/linha/results")
)
val context = client.ensureBackend("metoc", spec)
```

Use `PodTemplate.fromJson(json)`, `.fromYaml(text)` or
`.fromFile(Paths.get("driver-template.yaml"))` for any other Kubernetes fields.
YAML is parsed on the client and sent as JSON; paths and executable callbacks
are never sent. Helpers return new templates. `.merge(other)` uses the same
rules as the server. `.withSecretVolume` and `.withPVC` add mounts on `main`.
YAML duplicate keys, collection aliases, custom object tags, null fields and
excessive nesting are rejected. Each template is limited to 64 KiB; the merged
server template must also fit. See the compiled example
[`PodTemplatesExample.scala`](../examples/jvm/src/main/scala/linha/example/PodTemplatesExample.scala).

The business entrypoint is unchanged. Code running on the driver can read
`/etc/metoc/...` and `sys.env("APP_PASSWORD")`; code running inside Spark tasks
gets the separately configured executor files/environment.

## Base JSON

The helpers above produce ordinary JSON. For example, under `engine.settings`:

```json
{
  "kubernetes": {
    "driverPodTemplate": {
      "metadata": {"annotations": {"example.com/workload": "metoc"}},
      "spec": {
        "imagePullSecrets": [{"name": "metoc-registry"}],
        "volumes": [{"name": "app-config", "configMap": {"name": "metoc-config-v1"}}],
        "containers": [{
          "name": "main",
          "env": [{"name": "APP_MODE", "value": "production"}],
          "volumeMounts": [{"name": "app-config", "mountPath": "/etc/metoc", "readOnly": true}]
        }]
      }
    },
    "executorPodTemplate": {
      "spec": {"imagePullSecrets": [{"name": "metoc-registry"}]}
    }
  }
}
```

Driver and executor templates are independent. Repeat shared configuration in
both, or reuse a Scala `PodTemplate`. The executable JSON request in
[`examples/spark/backend-with-templates.json`](../examples/spark/backend-with-templates.json)
shows both sides.

## Spark CPU and memory

Set resources in `engine.settings.drivers` and `engine.settings.executors`.
CPU/memory values on the `main` container template remain protected.

`cores` sets Spark parallelism and defaults both CPU request and limit. Override
`coreRequest` and `coreLimit` independently when needed. `memory` sets JVM heap;
`memoryOverhead` adds container headroom. For both drivers and executors, Spark
submission sets memory request and limit to heap plus overhead. For example,
`memory: "10g", memoryOverhead: "2g"` gives 12Gi to the container and 10g to the JVM.
See [resource semantics and migration](versioned-spark-applications.md).

## Deployment defaults and registry authentication

```yaml
allowedImages: [registry.example/metoc]
registrySecrets:
  allowedNames: [metoc-registry]
workerPodTemplates:
  driverPodTemplate:
    spec:
      nodeSelector: {workload: spark}
  executorPodTemplate:
    spec:
      nodeSelector: {workload: spark}
```

Create `metoc-registry` as a `kubernetes.io/dockerconfigjson` or
`kubernetes.io/dockercfg` Secret in the release namespace. Reference its name
in either Pod template. The Helm server Role grants **get on only the named
Secrets**, allowing Linha to resolve private worker tags to digests. Selected
credentials are loaded again on each resolution attempt, so rotation works;
only Secret names enter the persisted spec. After a context's worker image has
been pinned, replacements reuse its digest without another tag lookup.
Kubelet reads the referenced Secrets when pulling containers.

Top-level Helm `imagePullSecrets` apply to the server and become defaults for
both worker templates; their names are automatically permitted for registry
lookup. `registrySecrets.allowedNames` alone permits selection but does not
attach Secrets to Pods. Credentials from the server's normal registry keychain
remain a fallback. Repository-scoped registry credentials are matched only to
that repository prefix. Kubernetes glob-pattern registry auth keys are not
supported by the server resolver; use explicit registry/repository keys.

The server Role also gains namespaced `get/create` on ConfigMaps. No ClusterRole
is needed. Application ConfigMaps, Secrets and PVCs remain caller-managed; the
server does not fetch application Secret values. Existing image repository
allowlisting applies to init containers and sidecars too. Specify digests for
those images if you need immutable versions; automatic digest pinning applies
to the main worker image only.

For standalone deployment, the equivalent JSON environment variables are
`LINHA_WORKER_POD_TEMPLATES`, `LINHA_WORKER_IMAGE_PULL_SECRETS` (array of names),
and `LINHA_REGISTRY_SECRETS` (array of permitted names).

## Merge and lifecycle rules

For new contexts, precedence is legacy Helm worker mounts/pull refs, then
`workerPodTemplates`, then the client template. Objects merge recursively.
Containers and init containers merge by `name`. Environment variables, volumes
and image pull Secrets replace same-name entries. Volume mounts replace entries
with the same `mountPath`. Other arrays replace the entire array. Empty named
arrays preserve inherited entries; null/delete/JSON-patch directives are not
supported. This allows changing an env value to a Secret reference or a volume
source without keeping incompatible fields from the previous entry.

The server stores both the normalized request and its effective merged spec.
The effective configuration and resolved primary image digest determine the
version under a stable owner/name. Changed deployment defaults create a new
version on ensure; clients already attached keep their old snapshot. Referenced
ConfigMaps/Secrets remain external: use immutable names for reproducibility.

New SparkApplications contain native driver/executor templates. The operator
translates them into Spark submission templates. Legacy stored Pod contexts
retain the previous executor-template ConfigMap path while draining.

Linha/Spark reserve Pod identity/namespace, service-account and token-mount
settings, restart policy, ownership labels, main image/command/args/working
directory and main CPU/memory settings. Set CPU/memory through Spark driver and
executor settings. `LINHA_*`/`SPARK_*` env variables and runtime mounts are
protected, including parent paths. Validation returns an error identifying the
conflicting field. Kubernetes still validates admission, scheduling and resource
availability; errors appear in backend conditions. Custom Pods run within the
existing trusted namespace model and its admission policies.

Upgrade server/chart and lease-capable clients together; see the
[schema-v5 migration guide](versioned-spark-applications.md).

## Verification

`go test -race ./server/...` with `LINHA_TEST_DATABASE` covers snapshot persistence,
concurrent ensure, schema-v3 migration, ownership, deterministic merging, managed
field rejection, replacement and private registry credential rotation. The JVM
and Go tests share `api/fixtures/pod-templates.json`. Run the chart checks with:

```sh
uv run --no-project --with PyYAML==6.0.3 python3 scripts/test-helm.py
```

`scripts/pod-template-e2e.py --state /path/to/disposable-state.json` installs two
server replicas and validates real Spark mounts, Secret env, pull refs, init
containers, CPU/memory requests and limits, static/dynamic allocation, a defaults-changing
rollout, driver replacement and retained results.
It accepts only a Kind cluster named `linha-pods-*`, its explicit `kind-...`
context and dedicated kubeconfig. The state JSON needs `cluster`, `context`,
`kubeconfig`, a temporary `directory`, and `postgres` (a fixture Docker container
named `<cluster>-postgres`, attached to the Kind network with database/user
`linha` and disposable password `linha-test-only`). Load
`linha/spark-example:validation3` into the node first; build the server as
`linha/server:pod-templates`, or pass `--server-tag resource-limits` after building
`linha/server:resource-limits`. These are disposable test names, not release tags.
The script retains the fixture for diagnosis; delete that specific cluster and
PostgreSQL container/volume after the check. It never targets the current context.
