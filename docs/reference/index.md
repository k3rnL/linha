# Reference

Look up the JSON contract first, then follow SDK usage for your language.
Field tables list exact names, types, requiredness/defaults and constraints.
JSON/Scala tabs show equivalent operations; the field definition stays shared.

## Browse by area

| Area | What you can find |
| --- | --- |
| [Backend configuration](backend/index.md) | Every backend option; nested Spark, driver/executor, Pod, results and storage settings |
| [Requests and jobs](requests.md) | Submission, retries/deadlines, states, failures, attempts and pagination |
| [Results](results.md) and [application storage](storage.md) | Published descriptors/datasets versus independent application files/sessions |
| [Scala/JVM SDK](../sdk-reference.md) | Client, worker, Spark, credentials, Pod/path and Hadoop APIs |
| [HTTP API](../http-api.md) | Public routes, [worker protocol](http/workers.md), [admin requests](http/admin.md), [observations](http/operations.md) and [errors](http/errors.md) |
| [Server deployment](deployment/helm.md) | Chart values, [environment](deployment/environment.md), [S3/membership](deployment/storage.md) |

For task-based steps use the [tutorial](../walkthrough.md) or [Spark guide](../versioned-spark-applications.md).
[Architecture](../architecture.md) explains context lifetime, durability and execution.

## Reading field tables

“Required” applies when the containing input object is present. Response fields
have no caller default, and optional output keys may be absent. Omission defaults
are distinguished from explicit zero/false/null where the runtime treats them
differently. Arbitrary handler payloads and documented dynamic maps have no fixed
list of application-owned keys.

## JSON model index {#json-model-index}

Model names identify wire shapes; a Scala class with the same name exists only
where the SDK reference explicitly declares one. Nested inline objects appear
under their containing model (for example SparkSettings.drivers).

| Model | Area |
| --- | --- |
| [`AdminAudit`](http/admin.md#adminaudit) | Administrator requests |
| [`AdminAuditPage`](http/admin.md#adminauditpage) | Administrator requests |
| [`AdminBatchItem`](http/admin.md#adminbatchitem) | Administrator requests |
| [`AdminBatchJob`](http/admin.md#adminbatchjob) | Administrator requests |
| [`AdminBatchRequest`](http/admin.md#adminbatchrequest) | Administrator requests |
| [`AdminBatchResult`](http/admin.md#adminbatchresult) | Administrator requests |
| [`AdminJob`](http/admin.md#adminjob) | Administrator requests |
| [`AdminJobPage`](http/admin.md#adminjobpage) | Administrator requests |
| [`AdminJobSelection`](http/admin.md#adminjobselection) | Administrator requests |
| [`AdminMappings`](deployment/storage.md#adminmappings) | Server provider/membership |
| [`AdminOverview`](http/operations.md#adminoverview) | Operational observations |
| [`AdminPreview`](http/admin.md#adminpreview) | Administrator requests |
| [`AdminResult`](http/admin.md#adminresult) | Administrator requests |
| [`AdminSession`](http/admin.md#adminsession) | Administrator requests |
| [`AdminSubmitRequest`](http/admin.md#adminsubmitrequest) | Administrator requests |
| [`Allocation`](http/workers.md#allocation) | Worker protocol |
| [`Assignment`](http/workers.md#assignment) | Worker protocol |
| [`AttachClient`](backend/index.md#attachclient) | Backend configuration |
| [`Attempt`](requests.md#attempt) | Requests and jobs |
| [`AttemptUpdate`](http/workers.md#attemptupdate) | Worker protocol |
| [`BackendSpec`](backend/index.md#backendspec) | Backend configuration |
| [`Capability`](http/workers.md#capability) | Worker protocol |
| [`ClaimRule`](deployment/storage.md#claimrule) | Server provider/membership |
| [`ClientLease`](backend/index.md#clientlease) | Backend configuration |
| [`Completion`](http/workers.md#completion) | Worker protocol |
| [`Context`](backend/index.md#context) | Backend configuration |
| [`DatasetAccess`](results.md#datasetaccess) | Published results |
| [`DatasetManifest`](results.md#datasetmanifest) | Published results |
| [`DatasetPart`](results.md#datasetpart) | Published results |
| [`DatasetPartPage`](results.md#datasetpartpage) | Published results |
| [`DatasetPartRegistration`](http/workers.md#datasetpartregistration) | Worker protocol |
| [`DatasetSeal`](http/workers.md#datasetseal) | Worker protocol |
| [`EngineObservation`](http/operations.md#engineobservation) | Operational observations |
| [`EngineSpec`](backend/index.md#enginespec) | Backend configuration |
| [`EnsureRequest`](backend/index.md#ensurerequest) | Backend configuration |
| [`Error`](http/errors.md#error) | HTTP errors |
| [`Failure`](requests.md#failure) | Requests and jobs |
| [`FailureRequest`](http/workers.md#failurerequest) | Worker protocol |
| [`Job`](requests.md#job) | Requests and jobs |
| [`JobPage`](requests.md#jobpage) | Requests and jobs |
| [`OperationalClient`](http/operations.md#operationalclient) | Operational observations |
| [`OperationalClientPage`](http/operations.md#operationalclientpage) | Operational observations |
| [`OperationalContext`](http/operations.md#operationalcontext) | Operational observations |
| [`OperationalContextPage`](http/operations.md#operationalcontextpage) | Operational observations |
| [`OperationalInstance`](http/operations.md#operationalinstance) | Operational observations |
| [`OperationalInstancePage`](http/operations.md#operationalinstancepage) | Operational observations |
| [`OperationalServer`](http/operations.md#operationalserver) | Operational observations |
| [`OperationalServerPage`](http/operations.md#operationalserverpage) | Operational observations |
| [`OperationalWorker`](http/operations.md#operationalworker) | Operational observations |
| [`OperationalWorkerPage`](http/operations.md#operationalworkerpage) | Operational observations |
| [`OutputRequest`](http/workers.md#outputrequest) | Worker protocol |
| [`OverviewContext`](http/operations.md#overviewcontext) | Operational observations |
| [`OverviewFailure`](http/operations.md#overviewfailure) | Operational observations |
| [`PathTemplate`](backend/results.md#pathtemplate) | Result policy |
| [`PodTemplate`](backend/pod-templates.md#podtemplate) | Pod templates |
| [`Progress`](requests.md#progress) | Requests and jobs |
| [`RenewRequest`](http/workers.md#renewrequest) | Worker protocol |
| [`Result`](results.md#result) | Published results |
| [`ResultDescriptor`](results.md#resultdescriptor) | Published results |
| [`ResultFile`](results.md#resultfile) | Published results |
| [`ResultPolicy`](backend/results.md#resultpolicy) | Result policy |
| [`RetryPolicy`](requests.md#retrypolicy) | Requests and jobs |
| [`RoleRules`](deployment/storage.md#rolerules) | Server provider/membership |
| [`S3Config`](deployment/storage.md#s3config) | Server provider/membership |
| [`SparkPodTemplates`](backend/pod-templates.md#sparkpodtemplates) | Pod templates |
| [`SparkSettings`](backend/spark.md#sparksettings) | Spark settings |
| [`StorageObject`](storage.md#storageobject) | Application storage |
| [`StorageSession`](storage.md#storagesession) | Application storage |
| [`StorageSessionInput`](storage.md#storagesessioninput) | Application storage |
| [`SubjectRule`](deployment/storage.md#subjectrule) | Server provider/membership |
| [`SubmitRequest`](requests.md#submitrequest) | Requests and jobs |
| [`Worker`](http/workers.md#worker) | Worker protocol |
| [`WorkloadStoragePolicy`](storage.md#workloadstoragepolicy) | Application storage |

### Nested Spark structures

| Structure | Fields |
| --- | --- |
| [`SparkSettings.application`](backend/spark.md#sparksettings-application-model) | Spark settings |
| [`SparkSettings.drivers`](backend/spark.md#sparksettings-drivers-model) | Spark settings |
| [`SparkSettings.executors`](backend/spark.md#sparksettings-executors-model) | Spark settings |
| [`SparkSettings.ui`](backend/spark.md#sparksettings-ui-model) | Spark settings |

## Scala/JVM type index {#scala-type-index}

All entries below refer to public SDK families in their documented packages.
Case-class fields and method signatures are listed on the linked pages.

| Type / object | Reference |
| --- | --- |
| `AccessToken` | [API](sdk/credentials.md#accesstoken) |
| `ArtifactResult` | [API](sdk/worker.md#resultwriter) |
| `Capability` | [API](sdk/worker.md#resultwriter) |
| `CredentialException` | [API](sdk/credentials.md#accesstoken) |
| `CredentialProvider` | [API](sdk/credentials.md#credentialprovider) |
| `CredentialProviders` | [API](sdk/credentials.md#oauth-client-credentials) |
| `DatasetLocation` | [API](sdk/worker.md#resultwriter) |
| `DatasetManifest` | [API](sdk/worker.md#resultwriter) |
| `DatasetPart` | [API](sdk/worker.md#resultwriter) |
| `DatasetProduced` | [API](sdk/worker.md#resultwriter) |
| `DatasetResult` | [API](sdk/worker.md#resultwriter) |
| `EntrypointCodec` | [API](sdk/worker.md#entrypointcodec) |
| `FileResult` | [API](sdk/worker.md#resultwriter) |
| `IdentityHttpOptions` | [API](sdk/credentials.md#identityhttpoptions) |
| `JobHandle` | [API](sdk/client.md#jobhandle) |
| `JsonResult` | [API](sdk/worker.md#resultwriter) |
| `LinhaClient` | [API](sdk/client.md#linhaclient) |
| `LinhaContext` | [API](sdk/client.md#linhacontext) |
| `LinhaEntrypoint` | [API](sdk/worker.md#entrypointcodec) |
| `LinhaException` | [API](sdk/credentials.md#transport) |
| `LinhaFileSystem` | [API](sdk/storage.md#linhafilesystem) |
| `LinhaJobContext` | [API](sdk/worker.md#linhajobcontext) |
| `LinhaWorker` | [API](sdk/worker.md#linhaworker) |
| `PathVariables` | [API](sdk/helpers.md#resultpath) |
| `PodTemplate` | [API](sdk/helpers.md#podtemplate) |
| `ProducedResult` | [API](sdk/worker.md#resultwriter) |
| `RawJobHandle` | [API](sdk/client.md#rawjobhandle) |
| `RawRequest` | [API](sdk/worker.md#resultwriter) |
| `ResultDescriptor` | [API](sdk/worker.md#resultwriter) |
| `ResultFile` | [API](sdk/worker.md#resultwriter) |
| `ResultPath` | [API](sdk/helpers.md#resultpath) |
| `ResultStorage` | [API](sdk/helpers.md#resultstorage) |
| `ResultWriter` | [API](sdk/worker.md#resultwriter) |
| `RetryableJobException` | [API](sdk/worker.md#linhaworker) |
| `Spark` | [API](sdk/spark.md#sparkruntime) |
| `SparkEntrypoint` | [API](sdk/spark.md#sparkruntime) |
| `SparkPodTemplates` | [API](sdk/helpers.md#sparkpodtemplates) |
| `SparkRuntime` | [API](sdk/spark.md#sparkruntime) |
| `StatusResult` | [API](sdk/worker.md#resultwriter) |
| `StorageObject` | [API](sdk/storage.md#storagesession) |
| `StorageSession` | [API](sdk/storage.md#storagesession) |
| `StorageSession.StorageLock` | [API](sdk/storage.md#storagelock) |
| `Transport` | [API](sdk/credentials.md#transport) |
| `WorkerCredential` | [API](sdk/worker.md#linhaworker) |
| `WorkerIdentity` | [API](sdk/worker.md#linhaworker) |
| `json` | [API](sdk/worker.md#entrypointcodec) |
| `syntax.SparkJobContext` | [API](sdk/spark.md#spark-syntax) |
| `syntax.SparkResultWriter` | [API](sdk/spark.md#spark-syntax) |

Other contracts: [Prometheus metrics](../metrics.md), [compatibility](../compatibility.md),
[validation scope](../validation.md), [release notes](../releases/index.md).
