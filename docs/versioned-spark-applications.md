# SparkApplication backends and client leases

New managed Spark backends require **Kubeflow Spark Operator 2.4.0**, with the
`sparkoperator.k8s.io/v1beta2` CRD and controller watching Linha's namespace.
Install the operator separately. Native templates were verified with its mutating
webhook disabled; configure `spark.jobNamespaces` to include the Linha namespace. Linha's chart grants its server namespaced
SparkApplication access; it does not install CRDs or the operator. Linha's own
namespace-only RBAC mode still needs no ClusterRole. The operator has its own
installation permissions.

## Spark launch and resources

```json
{
  "name": "metoc-extract",
  "clientId": "unique-api-process-uuid",
  "spec": {
    "image": "registry.example/metoc/worker:latest",
    "engine": {
      "type": "spark",
      "version": "3.5.6",
      "settings": {
        "application": {
          "mainClass": "fr.cls.bigdata.metoc.linha.MetocWorker",
          "mainApplicationFile": "local:///opt/linha/metoc-linha-worker.jar"
        },
        "ui": {"enabled": true, "port": 4040},
        "drivers": {
          "minDrivers": 1, "maxDrivers": 3,
          "maxConcurrentRequestsPerDriver": 2,
          "cores": 2, "coreRequest": "1", "coreLimit": "2",
          "memory": "10g", "memoryOverhead": "2g",
          "javaOptions": "-Dconfig.file=/etc/metoc/application.conf"
        },
        "executors": {
          "cores": 2, "coreRequest": "1", "coreLimit": "2",
          "memory": "4g", "memoryOverhead": "1g",
          "dynamicAllocation": true,
          "executorIdleTimeout": "60s",
          "cachedExecutorIdleTimeout": "120s",
          "shuffleTrackingTimeout": "120s",
          "minExecutors": 0, "initialExecutors": 1, "maxExecutors": 8,
          "javaOptions": "-Dconfig.file=/etc/metoc/application.conf"
        }
      }
    },
    "results": {"type": "s3", "destination": "metoc-results"}
  }
}
```

Mount `/etc/metoc/application.conf` in both driver and executor Pod templates
using the existing ConfigMap/Secret helpers. `application.arguments` supplies
main-class arguments. The worker image must retain Spark's driver/executor
entrypoint and contain the declared JAR. The old `/opt/linha/bin/worker` launch
script is **not invoked** for SparkApplications; move its Java options, mounts
and arguments into the settings above. The main method should use `new SparkConf()`
so it inherits Spark submission configuration. `LINHA_SPARK_CONF` is `{}` in
operator mode; SDK identity and capacity environment variables remain available.

`memory` means JVM heap. `memoryOverhead` adds container headroom, for both driver
and executor. For example, 10g + 2g produces a 10g heap and 12Gi memory request
and limit. Omitted overhead defaults to max(384 MiB, 10% of heap). `cores` controls
Spark parallelism; CPU request and limit default to it and can be set separately.
Deprecated `memoryMi` is a heap alias and cannot be combined with `memory`.
Use Spark units `m` or `g`. Heap overrides in `javaOptions` are rejected.

Linha submits one SparkApplication per driver, with `restartPolicy: Never`.
Linha controls pool size and replacement; the operator submits each application
and Spark creates its Pods. Pod-template container name `main` remains the public
SDK convention; Linha translates it to `spark-kubernetes-driver` or
`spark-kubernetes-executor`. Native Pod customizations remain available.

## Spark UI and executor idle retention

Configure these under `spec.engine.settings` when calling `ensureBackend`.
They are context settings, not Linha Helm values. The JVM client already accepts
these JSON fields; a new SDK is not required. For example, merge this into your
existing settings (preserving application, driver and executor resource fields):

```scala
val updatedSettings = settings.deepMerge(Json.obj(
  "ui" -> Json.obj("enabled" -> true.asJson, "port" -> 4040.asJson),
  "executors" -> Json.obj(
    "dynamicAllocation" -> true.asJson,
    "minExecutors" -> 0.asJson,
    "executorIdleTimeout" -> "60s".asJson,
    "cachedExecutorIdleTimeout" -> "120s".asJson,
    "shuffleTrackingTimeout" -> "120s".asJson
  )
))
```

Use `io.circe.Json` and `io.circe.syntax._`, and place `updatedSettings` in
`spec.engine.settings` before ensuring your backend.

| Setting | Default | Meaning |
| --- | --- | --- |
| `ui.enabled` | `false` | Start the driver's Spark UI. |
| `ui.port` | `4040` | Driver UI port; integer 1024–65535. |
| `executors.executorIdleTimeout` | `60s` | Idle removal without retained data. |
| `executors.cachedExecutorIdleTimeout` | `infinity` | Idle removal with cached blocks. |
| `executors.shuffleTrackingTimeout` | `infinity` | Timeout for executors retained by shuffle tracking. |

Timeouts accept positive integers with `s`, `m`, `h`, or `d`, up to 2147483647
seconds. Cache and shuffle timeouts also accept `infinity`; omit a field or use
an empty string for its default. Durations normalize to seconds (`2m` and `120s`
share a configuration version). Explicit defaults are equivalent to omission.

These timeouts apply only with dynamic allocation enabled. Spark controls actual
executor removal and respects `minExecutors`; use zero to allow scale-down to
zero executors. The initial count is a startup setting, not a permanent minimum.
Removal timing is not a hard deadline, and active work or retained data can delay
it. Finite cache/shuffle retention can cause recomputation. Release job-owned
caches with `unpersist()` when finished; avoid globally clearing shared caches
while other requests run. Driver pool lifetime still follows Linha client leases
and the separate driver scaling settings.

After enabling the UI, choose the relevant driver and forward its configured port:

```sh
kubectl -n metocprod get pods -l spark-role=driver
kubectl -n metocprod port-forward pod/<driver-pod-name> 4040:4040
```

Open `http://localhost:4040`. For a custom driver port of 4050, use `4040:4050`.
Each driver has its own UI, available while that application lives. Linha creates
no public Ingress or authentication proxy for it; Linha's OIDC does not protect
Spark's separate HTTP UI. Persistent event logs and a Spark History Server are
separate configuration for browsing executions after drivers stop.

Upgrade every Linha server replica before submitting these fields; older servers
reject them. Ensure the same logical context name with the new settings to create
a new version. Existing drivers are not patched: old versions drain according to
their client leases. Remove hardcoded UI/timeout overrides in worker bootstrap
code if you added them earlier, since programmatic SparkConf overrides submitted
configuration. Otherwise no worker-image rebuild is necessary. Omitted settings
preserve previous defaults and normalized context identities. Before rolling back
to an older server, drain versions that contain these new fields.

See [Spark configuration](https://spark.apache.org/docs/3.5.6/configuration.html#dynamic-allocation)
and [Spark monitoring](https://spark.apache.org/docs/3.5.6/monitoring.html).

## Stable names, versions and lifetime

Keep a name such as `metoc-extract` across releases. The server hashes the
normalized effective settings, merged deployment defaults, result policy and
resolved primary worker-image digest. Identical owner/name/configuration shares
a context ID; changed settings or a changed image tag target creates another
version under that name. Image resolution occurs during ensure, before the
context response; job submission still acknowledges immediately after DB commit.
Use immutable versioned ConfigMaps/Secrets when their contents are part of the
release: referenced object contents are not included in the hash.

Each API process owns a lease on every version it uses. Leases last 90 seconds,
using PostgreSQL time. The Scala client renews them every 30 seconds. Keep the
client and context alive for the API process lifetime; close them during graceful
shutdown. Handles for the same version within a client share one lease. Closing
the final handle releases it. A killed process loses its lease through expiry.

```scala
val client = new LinhaClient(serverUrl, credentials)
val context = Await.result(client.ensureBackend("metoc-extract", spec), 30.seconds)
// Keep context in your API service. Submission serializes business parameters.
val job = Await.result(context.submit(Extract(parameters), requestKey), 10.seconds)
// On API shutdown, after stopping acceptance of new API requests:
context.close()
client.close()
```

During rolling updates, versions coexist while their API replicas are alive.
With no live clients a version becomes `DRAINING`: it finishes accepted queued,
running and retrying work, replacing lost drivers if needed. Once all accepted
jobs are terminal, Linha stops all instances even when `minDrivers` is positive.
It becomes `STOPPED` and retains job IDs, status and results. Jobs with no deadline
that never finish can keep a draining backend alive; cancellation is explicit.
Reattaching by context ID reactivates that version without changing its history.

Do not call ensure to poll a job: that acquires a backend lease. Use
`client.rawJob(jobId)` or `client.job(jobId, codec)` for independent retrieval.
Idempotency keys are scoped by owner/logical name/key across versions; retries
with the same request and result policy return the original job ID. A changed
request or result policy with the same key is a conflict.

## Raw JSON lifecycle

- `POST /v1/contexts:ensure` returns `id`, `version`, and `clientLease`.
- `POST /v1/contexts/{id}/clients` with `{"clientId":"process-uuid"}` attaches
  to an existing immutable version and returns the same response shape.
- `POST /v1/contexts/{id}/clients/{lease}/renew` renews a live lease. Expired
  tokens return `CLIENT_LEASE_EXPIRED`; attach again to get a fresh token.
- `DELETE /v1/contexts/{id}/clients/{lease}` releases a lease idempotently.
- Submit new jobs with the `X-Linha-Client-Lease` header set to its lease ID.

All endpoints retain owner isolation. Polling context/job state does not renew a
lease. Leases are persisted; restarting any or all server replicas does not
reset their expiry. The JVM SDK reacquires expired leases by immutable context ID.

## Upgrade

Schema v5 preserves existing context/job IDs and grants old contexts a 90-second
migration lease. Persisted legacy contexts retain their old Pod launch path for
draining. Upgrade server, Helm chart and client SDK together with coordinated
server shutdown/startup; mixed schema-v4/v5 server writers are unsupported.
New ensures require application settings and lease-capable clients. Existing
clients without heartbeats do not keep a backend alive indefinitely. Production
worker images may need adjustment for the Spark entrypoint described above.
Database migrations do not delete result bytes or job history.

References: [Spark memory configuration](https://spark.apache.org/docs/3.5.6/configuration.html),
[SparkApplication guide](https://spark.kubeflow.org/en/latest/user-guide/writing-sparkapplication.html),
[operator v2.4.0 API](https://github.com/kubeflow/spark-operator/blob/v2.4.0/api/v1beta2/sparkapplication_types.go).
