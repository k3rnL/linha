# Manage Spark applications

Configure driver capacity, executor allocation and Spark resources in the backend
spec passed to `ensureBackend`. Linha creates one SparkApplication per managed
driver. Kubeflow Spark Operator submits it, and Spark creates driver/executor Pods.

This guide assumes the operator watches the Linha namespace and your worker image
contains the declared JVM main class and JAR. See [deployment](operations.md).

New driver Pods use `<context-name>-<instance-id>-driver`, for example
`reporting-0123456789abcdef0123456789abcdef-driver`. Linha lowercases the context
prefix, replaces unsupported characters with hyphens and limits that prefix to
23 characters, keeping the complete unique instance ID within Kubernetes' 63-character
name limit. A name with no supported characters uses `context` as its prefix.
Existing applications retain their current driver names until a new instance is
provisioned. No client configuration change is needed.

## Set the application and resources

```json
{
  "image": "registry.example/workers/reporting:1",
  "engine": {
    "type": "spark",
    "version": "3.5.6",
    "settings": {
      "application": {
        "mainClass": "example.WorkerMain",
        "mainApplicationFile": "local:///opt/linha/worker.jar"
      },
      "drivers": {
        "minDrivers": 1,
        "maxDrivers": 3,
        "maxConcurrentRequestsPerDriver": 2,
        "cores": 2,
        "coreRequest": "1",
        "coreLimit": "2",
        "memory": "10g",
        "memoryOverhead": "2g"
      },
      "executors": {
        "cores": 2,
        "memory": "4g",
        "memoryOverhead": "1g",
        "dynamicAllocation": true,
        "minExecutors": 0,
        "initialExecutors": 1,
        "maxExecutors": 8,
        "executorIdleTimeout": "60s",
        "cachedExecutorIdleTimeout": "120s",
        "shuffleTrackingTimeout": "120s"
      },
      "ui": {"enabled": true, "port": 4040}
    }
  },
  "results": {"type": "s3", "destination": "analytics"}
}
```

`memory` is the JVM heap, with `memoryOverhead` added to the container request
and limit. A 10g heap plus 2g overhead gives the driver 12Gi. CPU requests and
limits default to `cores`; override them separately when needed. Do not put `-Xmx`
in `javaOptions` or override these resources through the main-container template.
Defaults and bounds are in [configuration reference](configuration.md#spark-settings).

The worker main method should use `new SparkConf()` to inherit submission
configuration. Preserve Spark's driver/executor image entrypoint. Use
`application.arguments` and `javaOptions` for application configuration, with
[Pod templates](pod-templates.md) for files and environment variables.

## Tune the driver pool

Set `minDrivers` and `maxDrivers` for capacity bounds. Use
`maxConcurrentRequestsPerDriver` to bound requests sharing one SparkContext.
`queueThreshold`, `waitSeconds` and `cooldownSeconds` control driver growth;
`idleSeconds` allows idle drivers above the minimum to retire.

Lost required drivers are replaced. With no live clients, the context version
finishes accepted work and then stops all drivers, even when its minimum is
positive. This lifetime behavior is described in [architecture](architecture.md#context-names-and-versions).

## Let idle executors retire

Enable dynamic allocation and set `minExecutors: 0` if an idle application may
have no executors. `initialExecutors` applies at startup; it is not a permanent
minimum. Set finite cache and shuffle timeouts when their retained data should
not keep executors indefinitely.

Default cache and shuffle timeouts are `infinity`. That is why an application
can retain executors even after its visible jobs finish. Active tasks, cached
blocks, shuffle tracking and the minimum count can delay executor removal.
Finite retention can cause recomputation. Use `ctx.cache` and `ctx.broadcast`
for request-owned resources; avoid globally clearing caches while other jobs run.

## Open a driver UI

Set `ui.enabled: true`, then choose a current driver:

```sh
kubectl -n linha get pods -l spark-role=driver
kubectl -n linha port-forward pod/<driver-pod-name> 4040:4040
```

Open `http://localhost:4040`. For `ui.port: 4050`, forward `4040:4050` instead.
Each driver has its own UI and it disappears when that application stops.

The administrator console uses an ingress URL exposed by the SparkApplication
when available. Linha creates no public Spark UI ingress or authentication proxy.
The Spark UI is a separate service; configure its access policy separately.
Use Spark event logs and a History Server for completed applications.

## Roll out a changed backend

Keep a logical name such as `reporting`. Ensure with the new image/settings and
retain the returned context for the application process lifetime. Linha derives
an immutable configuration version. Existing clients keep their version and
accepted requests; new replicas attach to the new version. Close old contexts on
graceful shutdown, or let their heartbeat expire after an unexpected exit.

Referenced ConfigMap and Secret contents do not enter the configuration hash.
Use immutable names when a change must produce a new reproducible version.
Only the primary worker image is resolved to a digest automatically; specify
sidecar/init image digests yourself when required.

Read the upstream [Spark configuration](https://spark.apache.org/docs/3.5.6/configuration.html)
and [SparkApplication guide](https://spark.kubeflow.org/en/latest/user-guide/writing-sparkapplication.html)
for the engine-side behavior.
