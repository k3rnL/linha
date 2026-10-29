# Spark settings

`SparkSettings` is `BackendSpec.engine.settings` when `engine.type` is `spark`
and `engine.version` is `3.5.6`. Managed deployments require `application` and use
Kubeflow Spark Operator 2.4.0 (`sparkoperator.k8s.io/v1beta2`). Fields below are
Linha settings, not an arbitrary SparkApplication or `sparkConf` passthrough.

For the Kubernetes resource Linha creates, see the upstream
[SparkApplication API reference (Operator v2.4.0)](https://github.com/kubeflow/spark-operator/blob/v2.4.0/docs/api-docs.md)
and [Writing a SparkApplication guide](https://spark.kubeflow.org/en/latest/user-guide/writing-sparkapplication.html).

Omitted driver/executor objects retain the defaults shown below. Explicit zero is
retained for numeric Spark settings, then validated: for example `minDrivers: 0`
is valid, `maxDrivers: 0` is invalid. Resource aliases have special rules below.


Field paths below are relative to their containing model; output fields have no caller default.

## Resource and allocation rules

Heap and overhead are binary MiB/GiB quantities even though Spark spells them
`m`/`g`. `memory: "10g"` plus `memoryOverhead: "2g"` requests and limits a 12 GiB
container with a 10 GiB JVM heap. Default overhead is rounded down to whole MiB,
with a 384 MiB minimum. CPU requests/limits default separately to `cores`.

Dynamic allocation controls executors within a driver. Linha independently scales
the number of drivers within the driver-pool bounds. One driver has one shared
SparkContext; requests use separate SparkSessions, scheduler pools and
attempt-specific job groups. Increasing request concurrency does not create more
executor task slots. Cancelling one request targets its own job group.

Idle, cache and shuffle timeouts accept positive integer `s`, `m`, `h`, `d`
durations up to 2147483647 seconds. Cache/shuffle timeouts also accept `infinity`.
Equivalent values such as `1m` and `60s` normalize identically for version hashing.
Timeouts are validated even with dynamic allocation disabled; they are applied
only when enabled. Finite cache/shuffle retention can require recomputation.

```json
{
  "drivers": {"minDrivers": 0, "maxDrivers": 3,
    "maxConcurrentRequestsPerDriver": 2, "cores": 2,
    "memory": "10g", "memoryOverhead": "2g"},
  "executors": {"dynamicAllocation": true, "minExecutors": 0,
    "initialExecutors": 1, "maxExecutors": 8,
    "cachedExecutorIdleTimeout": "120s", "shuffleTrackingTimeout": "120s"}
}
```

This fragment belongs inside `engine.settings` alongside required `application`.
Use `new SparkConf()` in the worker main method to inherit submitted settings.
See [driver main and image contract](../sdk/spark.md#worker-image) and
[Pod customization](pod-templates.md) for environment, files and scheduling.

## JVM system properties and Log4j {#jvm-system-properties-and-log4j}

Use `spec.engine.settings.drivers.javaOptions` for driver JVM flags, including
`-Dname=value` system properties. It is a string of JVM options, not an array of
main-class arguments. Linha passes it to the SparkApplication's
[`spec.driver.javaOptions` field](https://spark.kubeflow.org/en/latest/user-guide/writing-sparkapplication.html#specifying-extra-java-options).
`application.arguments` is for arguments received by your main method.

For a custom Log4j 2 configuration, merge this fragment into `engine.settings`:

```json
{
  "drivers": {
    "javaOptions": "-Dlog4j.configurationFile=file:/etc/job-logging/log4j2.properties"
  }
}
```

The file must exist at that path **inside the driver container**. Include it in
the worker image or mount a ConfigMap through
[`kubernetes.driverPodTemplate`](pod-templates.md); see the
[file-mount example](../../pod-templates.md#add-files-environment-and-pull-secrets).
Setting the property does not upload a client-local file. Spark's
[Log4j configuration example](https://spark.apache.org/docs/3.5.6/running-on-yarn.html#debugging-your-application)
documents this system property and the explicit `file:` URI.

Multiple flags are space-separated in the same string. Use
`executors.javaOptions` and an executor template mount separately if executors
also need custom logging. Heap flags remain subject to the resource restrictions
below. Changing JVM options changes the backend version: ensure the updated spec
under the same logical name and submit through the returned context. Existing
drivers keep their original version and JVM startup configuration.

## SparkSettings {#sparksettings}

Input at `BackendSpec.engine.settings`. Nested objects below specify every Linha-owned Spark option.

Spark 3.5.6 through Kubeflow Spark Operator v2.4.0. Memory is JVM heap; memoryOverhead is additional container memory. Deprecated memoryMi is an alternative heap alias.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="sparksettings-drivers"></span>`drivers` | object ([fields](#sparksettings-drivers-model)) | Optional; seeded defaults | Driver pool, request concurrency and driver resources. Omission keeps the defaults in the nested table. |
| <span id="sparksettings-executors"></span>`executors` | object ([fields](#sparksettings-executors-model)) | Optional; seeded defaults | Executor resources and Spark executor allocation. Omission keeps the nested defaults. |
| <span id="sparksettings-kubernetes"></span>`kubernetes` | [SparkPodTemplates](pod-templates.md#sparkpodtemplates) | Optional; omitted | Independent Pod fragments. Operator defaults merge with client overrides and are snapshotted in the version. |
| <span id="sparksettings-application"></span>`application` | object ([fields](#sparksettings-application-model)) | Required; no default | Required for managed deployment; JVM entrypoint already installed in the worker image. |
| <span id="sparksettings-ui"></span>`ui` | object ([fields](#sparksettings-ui-model)) | Optional; omitted | Per-driver Spark UI settings; does not configure Linha administrator UI. |

### Driver settings {#sparksettings-drivers-model}

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="sparksettings-drivers-memory"></span>`drivers.memory` | string | Optional; `"1024m"` | JVM heap string in m/g units (case-insensitive), 512m–128g. Default 1024m. Explicit 0m is invalid. Pattern: `^[0-9]+[mMgG]$`. |
| <span id="sparksettings-drivers-memoryoverhead"></span>`drivers.memoryOverhead` | string | Optional; max(384m, 10% heap rounded down to MiB) | Extra container memory, 1m–128g. Default max(384, floor(heap MiB / 10)) MiB for managed SparkApplications. Pattern: `^[0-9]+[mMgG]$`. |
| <span id="sparksettings-drivers-memorymi"></span>`drivers.memoryMi` | integer | Optional; omitted | Deprecated integer heap alias, 512–131072 MiB when nonzero; zero selects default. A nonzero alias cannot be combined with memory. Minimum: 512. Maximum: 131072. Deprecated. |
| <span id="sparksettings-drivers-cores"></span>`drivers.cores` | integer | Optional; `1` | Spark cores/task parallelism, 1–32; also the default CPU request and limit. Minimum: 1. Maximum: 32. |
| <span id="sparksettings-drivers-corerequest"></span>`drivers.coreRequest` | string | Optional; corresponding cores | CPU request string, positive and at most 32 cores. Decimal cores or millicores (e.g. 500m); defaults to cores. |
| <span id="sparksettings-drivers-corelimit"></span>`drivers.coreLimit` | string | Optional; corresponding cores | CPU limit string, at least coreRequest and at most 32 cores; defaults to cores, so a larger request also needs a larger limit. |
| <span id="sparksettings-drivers-javaoptions"></span>`drivers.javaOptions` | string | Optional; empty string | Extra driver JVM flags, including `-Dname=value` system properties for Log4j and other libraries. See [JVM/Log4j example](#jvm-system-properties-and-log4j). -Xmx, -Xms and MaxRAM substrings are rejected; set heap through memory. |
| <span id="sparksettings-drivers-mindrivers"></span>`drivers.minDrivers` | integer | Optional; `1` | Minimum drivers while clients have live leases; 0 is valid. After accepted work drains without clients, all drivers stop. Minimum: 0. Maximum: 16. |
| <span id="sparksettings-drivers-maxdrivers"></span>`drivers.maxDrivers` | integer | Optional; `1` | Maximum managed drivers; 1–16 and at least minDrivers. Minimum: 1. Maximum: 16. |
| <span id="sparksettings-drivers-maxconcurrentrequestsperdriver"></span>`drivers.maxConcurrentRequestsPerDriver` | integer | Optional; `1` | Request slots sharing one SparkContext; 1–32. Independent of executor task slots. Minimum: 1. Maximum: 32. |
| <span id="sparksettings-drivers-queuethreshold"></span>`drivers.queueThreshold` | integer | Optional; `1` | Queued-request threshold used for driver scale-up after cooldown; at least 1. Minimum: 1. |
| <span id="sparksettings-drivers-waitseconds"></span>`drivers.waitSeconds` | integer | Optional; `5` | Queue age threshold for scale-up, in seconds; at least 1. Either queue threshold or wait threshold can trigger growth. Minimum: 1. |
| <span id="sparksettings-drivers-cooldownseconds"></span>`drivers.cooldownSeconds` | integer | Optional; `10` | Minimum interval between ordinary scaling decisions, seconds; at least 1. Startup/minimum capacity has exceptions. Minimum: 1. |
| <span id="sparksettings-drivers-idleseconds"></span>`drivers.idleSeconds` | integer | Optional; `60` | Idle seconds before a ready excess driver can drain during scale-down; at least 1. Minimum: 1. |

### Executor settings {#sparksettings-executors-model}

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="sparksettings-executors-memory"></span>`executors.memory` | string | Optional; `"1024m"` | JVM heap string in m/g units (case-insensitive), 512m–128g. Default 1024m. Explicit 0m is invalid. Pattern: `^[0-9]+[mMgG]$`. |
| <span id="sparksettings-executors-memoryoverhead"></span>`executors.memoryOverhead` | string | Optional; max(384m, 10% heap rounded down to MiB) | Extra container memory, 1m–128g. Default max(384, floor(heap MiB / 10)) MiB for managed SparkApplications. Pattern: `^[0-9]+[mMgG]$`. |
| <span id="sparksettings-executors-memorymi"></span>`executors.memoryMi` | integer | Optional; omitted | Deprecated integer heap alias, 512–131072 MiB when nonzero; zero selects default. A nonzero alias cannot be combined with memory. Minimum: 512. Maximum: 131072. Deprecated. |
| <span id="sparksettings-executors-cores"></span>`executors.cores` | integer | Optional; `1` | Spark cores/task parallelism, 1–32; also the default CPU request and limit. Minimum: 1. Maximum: 32. |
| <span id="sparksettings-executors-corerequest"></span>`executors.coreRequest` | string | Optional; corresponding cores | CPU request string, positive and at most 32 cores. Decimal cores or millicores (e.g. 500m); defaults to cores. |
| <span id="sparksettings-executors-corelimit"></span>`executors.coreLimit` | string | Optional; corresponding cores | CPU limit string, at least coreRequest and at most 32 cores; defaults to cores, so a larger request also needs a larger limit. |
| <span id="sparksettings-executors-javaoptions"></span>`executors.javaOptions` | string | Optional; empty string | Extra executor JVM flags, including `-Dname=value` system properties; configured separately from driver options. -Xmx, -Xms and MaxRAM substrings are rejected; set heap through memory. |
| <span id="sparksettings-executors-minexecutors"></span>`executors.minExecutors` | integer | Optional; `0` | Dynamic minimum, 0–128; zero permits an idle application to have no executors. Minimum: 0. Maximum: 128. |
| <span id="sparksettings-executors-initialexecutors"></span>`executors.initialExecutors` | integer | Optional; `1` | Dynamic startup count, 0–128, at least minExecutors; not a permanent minimum. Minimum: 0. Maximum: 128. |
| <span id="sparksettings-executors-maxexecutors"></span>`executors.maxExecutors` | integer | Optional; `4` | Dynamic maximum, 1–128, at least initialExecutors. Minimum: 1. Maximum: 128. |
| <span id="sparksettings-executors-instances"></span>`executors.instances` | integer | Optional; `1` | Static executor count, 1–128; ignored for allocation when dynamicAllocation is true, but still validated. Minimum: 1. Maximum: 128. |
| <span id="sparksettings-executors-dynamicallocation"></span>`executors.dynamicAllocation` | boolean | Optional; `false` | Enable Spark dynamic allocation and shuffle tracking. Linha scales drivers separately. |
| <span id="sparksettings-executors-executoridletimeout"></span>`executors.executorIdleTimeout` | string | Optional; `"60s"` | Idle timeout without retained data; positive integer s/m/h/d, at most 2147483647 seconds. Empty selects default. Applied only with dynamic allocation. Pattern: `^([1-9][0-9]*[smhd])?$`. |
| <span id="sparksettings-executors-cachedexecutoridletimeout"></span>`executors.cachedExecutorIdleTimeout` | string | Optional; `"infinity"` | Cached-block retention timeout; positive integer s/m/h/d or infinity. Same duration bound. Empty selects default. Applied only with dynamic allocation. Pattern: `^([1-9][0-9]*[smhd]&#124;infinity)?$`. |
| <span id="sparksettings-executors-shuffletrackingtimeout"></span>`executors.shuffleTrackingTimeout` | string | Optional; `"infinity"` | Shuffle-tracking retention timeout; positive integer s/m/h/d or infinity. Same duration bound. Empty selects default. Applied only with dynamic allocation. Pattern: `^([1-9][0-9]*[smhd]&#124;infinity)?$`. |

### Spark application {#sparksettings-application-model}

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="sparksettings-application-mainclass"></span>`application.mainClass` | string | Required; no default | Required JVM main-class name matching [a-zA-Z_$][a-zA-Z0-9_$.]*, installed in the image. |
| <span id="sparksettings-application-mainapplicationfile"></span>`application.mainApplicationFile` | string | Required; no default | Required image-local JAR path starting local:/// and ending .jar; no JAR upload occurs. Pattern: `^local:///.+\.jar$`. |
| <span id="sparksettings-application-arguments"></span>`application.arguments` | array of string | Optional; `[]` | Optional main-class arguments; at most 128 strings, default empty list. Maximum items: 128. |

### Driver UI {#sparksettings-ui-model}

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="sparksettings-ui-enabled"></span>`ui.enabled` | boolean | Optional; `false` | Enable each driver UI; default false. Access control/ingress is not provisioned by this flag. |
| <span id="sparksettings-ui-port"></span>`ui.port` | integer | Optional; `4040` | UI TCP port, 1024–65535; default 4040, validated even when UI is disabled. Minimum: 1024. Maximum: 65535. |
