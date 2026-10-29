# Configuration reference

Choose the configuration surface you are working with. Application backend fields
and operator deployment fields have separate complete references.

| Surface | Canonical reference |
| --- | --- |
| Declare a worker/backend | [BackendSpec and ensure](reference/backend/index.md) |
| Set Spark main class, JAR, resources and scaling | [SparkSettings](reference/backend/spark.md) |
| Configure environment, files and scheduling | [Pod fragments](reference/backend/pod-templates.md) |
| Select result storage, size, retention and paths | [ResultPolicy](reference/backend/results.md) |
| Access application inputs/shared files | [Workload storage](reference/storage.md) |
| Submit a request, retry or deadline | [SubmitRequest](reference/requests.md#submitrequest) |
| Configure the Linha deployment | [Helm values](reference/deployment/helm.md) |

The [model index](reference/index.md#json-model-index) includes all named public
models and nested configuration structures. Tables list field types, requiredness,
defaults, bounds and interactions; JSON/Scala example tabs share those definitions.

## Helm deployment settings {#helm-deployment-settings}

Every chart option is listed in [Helm values](reference/deployment/helm.md).
[S3 destination and membership models](reference/deployment/storage.md) describe
structured operator policy. Use [deployment procedures](operations.md) to install.

## Spark settings {#spark-settings}

See [SparkSettings](reference/backend/spark.md#sparksettings), including
[application](reference/backend/spark.md#sparksettings-application-model),
[drivers](reference/backend/spark.md#sparksettings-drivers-model),
[executors](reference/backend/spark.md#sparksettings-executors-model),
[UI](reference/backend/spark.md#sparksettings-ui-model), and
[Pod fragments](reference/backend/pod-templates.md).

## Results and request policy {#results-and-request-policy}

[ResultPolicy](reference/backend/results.md#resultpolicy) controls output storage,
size and retention. [SubmitRequest](reference/requests.md#submitrequest) controls
retry/deadline; [job state semantics](reference/requests.md#job-states) describe
acceptance, execution and cancellation. These are different model surfaces.

## Result path templates {#result-path-templates}

See [PathTemplate](reference/backend/results.md#pathtemplate) and the complete
[placeholder/validation rules](reference/backend/results.md#path-placeholders).

## Standalone server environment {#standalone-server-environment}

[Server environment](reference/deployment/environment.md) lists every Linha server
variable, standalone default and Helm equivalent. Worker-injected variables are
in the [worker reference](reference/sdk/worker.md#managed-worker-environment).
