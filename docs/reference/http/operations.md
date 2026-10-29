# Operational observations

Operational models are read-only administrator observations across owners.
They combine durable records with time-stamped engine/server observations.
Check `available`, `stale` and `observedAt` before interpreting live resources or
links. Observation does not attach a client or start an engine.

Page envelopes contain `items` and an opaque `nextCursor`; an omitted cursor ends
the listing. Copy cursors unchanged. Overview counts are independent of page size.


Field paths below are relative to their containing model; output fields have no caller default.

## OperationalContext {#operationalcontext}

Administrator output combining Context with queue and client/worker counts.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="operationalcontext-id"></span>`id` | string | Present; no caller default | Opaque server/resource identity; preserve it verbatim. |
| <span id="operationalcontext-name"></span>`name` | string | Present; no caller default | Logical name within its containing model. |
| <span id="operationalcontext-spec"></span>`spec` | [BackendSpec](../backend/index.md#backendspec) | Present; no caller default | Effective normalized backend configuration; follow BackendSpec. |
| <span id="operationalcontext-state"></span>`state` | string | Present; no caller default | Current persisted or observed lifecycle state; see the model notes. |
| <span id="operationalcontext-condition"></span>`condition` | string | Optional output; no caller default | Optional diagnostic condition explaining provisioning or observation state. |
| <span id="operationalcontext-createdat"></span>`createdAt` | string (date-time) | Present; no caller default | Server-recorded creation instant (RFC 3339). |
| <span id="operationalcontext-version"></span>`version` | string | Present; no caller default | SHA-256 of effective configuration |
| <span id="operationalcontext-clientlease"></span>`clientLease` | [ClientLease](../backend/index.md#clientlease) | Optional output; no caller default | Lease returned by ensure/attach. A plain GET neither attaches nor renews it. |
| <span id="operationalcontext-owner"></span>`owner` | string | Optional output; no caller default | Authenticated application owner; supplied by the server, not application request JSON. |
| <span id="operationalcontext-metriccontext"></span>`metricContext` | string | Optional output; no caller default | Bounded metric alias for this owner/logical context. |
| <span id="operationalcontext-observedat"></span>`observedAt` | string (date-time) | Optional output; no caller default | Time of the latest observation or coherent summary (RFC 3339). |
| <span id="operationalcontext-liveclients"></span>`liveClients` | integer | Optional output; no caller default | Count of live client leases. |
| <span id="operationalcontext-queuedjobs"></span>`queuedJobs` | integer | Optional output; no caller default | Queued demand, including retrying jobs where noted by the summary. |
| <span id="operationalcontext-runningjobs"></span>`runningJobs` | integer | Optional output; no caller default | Count of running work, including cancellation in progress where the read model groups it. |
| <span id="operationalcontext-readyworkers"></span>`readyWorkers` | integer | Optional output; no caller default | Count of ready/live registered workers. |
| <span id="operationalcontext-desiredinstances"></span>`desiredInstances` | integer | Optional output; no caller default | Controller desired driver/instance count. |

## OperationalClient {#operationalclient}

Administrator output describing a client attachment or the last recorded lease.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="operationalclient-id"></span>`id` | string | Present; no caller default | Opaque server/resource identity; preserve it verbatim. |
| <span id="operationalclient-clientid"></span>`clientId` | string | Present; no caller default | Application process identifier, at most 128 bytes. Omission generates an ID; the SDK reuses one per client. |
| <span id="operationalclient-expiresat"></span>`expiresAt` | string (date-time) | Optional output; no caller default | Server-recorded expiry instant (RFC 3339); check before using retained access. |
| <span id="operationalclient-lastrenewedat"></span>`lastRenewedAt` | string (date-time) | Optional output; no caller default | Last recorded client lease renewal time (RFC 3339). |
| <span id="operationalclient-state"></span>`state` | string | Present; no caller default | Current persisted or observed lifecycle state; see the model notes. Values: `active`, `expired`, `released`. |
| <span id="operationalclient-hostname"></span>`hostname` | string | Optional output; no caller default | Optional client-reported hostname (at most 253 UTF-8 bytes, no whitespace/control characters). Display metadata only; never identity or authorization. Maximum length: 253. |

## EngineObservation {#engineobservation}

Optional adapter observation in OperationalInstance.detail.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="engineobservation-application"></span>`application` | string | Optional output; no caller default | Spark application identity or settings, according to the model. |
| <span id="engineobservation-applicationstate"></span>`applicationState` | string | Optional output; no caller default | Latest SparkApplication state reported by the adapter. |
| <span id="engineobservation-driverpod"></span>`driverPod` | string | Optional output; no caller default | Observed Spark driver Pod name. |
| <span id="engineobservation-driverphase"></span>`driverPhase` | string | Optional output; no caller default | Latest Kubernetes Pod phase for the driver. |
| <span id="engineobservation-uiaddress"></span>`uiAddress` | string | Optional output; no caller default | Observed driver UI address when available. |
| <span id="engineobservation-uiservice"></span>`uiService` | string | Optional output; no caller default | Observed Spark UI Service name. |
| <span id="engineobservation-condition"></span>`condition` | string | Optional output; no caller default | Optional diagnostic condition explaining provisioning or observation state. |
| <span id="engineobservation-uiport"></span>`uiPort` | integer | Optional output; no caller default | Observed Spark UI TCP port. |
| <span id="engineobservation-available"></span>`available` | boolean | Optional output; no caller default | Whether current engine observation is available. |
| <span id="engineobservation-executorstates"></span>`executorStates` | map of integer | Optional output; no caller default | Dynamic map of observed executor state to count. |
| <span id="engineobservation-resources"></span>`resources` | map of number | Optional output; no caller default | Dynamic map of normalized observed resource measurements, not configurable resource requests. |
| <span id="engineobservation-links"></span>`links` | array of object ([fields](#engineobservation-links-model)) | Optional output; no caller default | Observed engine links with display names and URLs; unavailable/stale data must not imply a live UI. |

### links[] {#engineobservation-links-model}

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="engineobservation-links-name"></span>`links[].name` | string | Present; no caller default | Display name for an observed engine link. |
| <span id="engineobservation-links-url"></span>`links[].url` | string (uri) | Present; no caller default | Observed absolute destination URL. |

## OperationalInstance {#operationalinstance}

Administrator output describing a managed engine instance and observation freshness.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="operationalinstance-id"></span>`id` | string | Present; no caller default | Opaque server/resource identity; preserve it verbatim. |
| <span id="operationalinstance-contextid"></span>`contextId` | string | Optional output; no caller default | Opaque immutable context-version ID, not the logical context name. |
| <span id="operationalinstance-incarnation"></span>`incarnation` | string | Optional output; no caller default | Managed worker incarnation; prevents a replacement process from impersonating an old attempt. |
| <span id="operationalinstance-resourcekind"></span>`resourceKind` | string | Optional output; no caller default | Adapter-managed resource kind, such as sparkapplication or pod. |
| <span id="operationalinstance-resourceuid"></span>`resourceUid` | string | Optional output; no caller default | Kubernetes resource UID used to reject replacement/deletion identity races. |
| <span id="operationalinstance-state"></span>`state` | string | Present; no caller default | Current persisted or observed lifecycle state; see the model notes. |
| <span id="operationalinstance-createdat"></span>`createdAt` | string (date-time) | Optional output; no caller default | Server-recorded creation instant (RFC 3339). |
| <span id="operationalinstance-observedat"></span>`observedAt` | string (date-time) | Optional output; no caller default | Time of the latest observation or coherent summary (RFC 3339). |
| <span id="operationalinstance-available"></span>`available` | boolean | Present; no caller default | Whether current engine observation is available. |
| <span id="operationalinstance-stale"></span>`stale` | boolean | Present; no caller default | True when the recorded engine observation is too old to represent current state. |
| <span id="operationalinstance-detail"></span>`detail` | [EngineObservation](operations.md#engineobservation) | Optional output; no caller default | Optional engine/server observation details; adapter-specific object. |

## OperationalWorker {#operationalworker}

Administrator output describing a registered worker and occupied request slots.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="operationalworker-id"></span>`id` | string | Present; no caller default | Opaque server/resource identity; preserve it verbatim. |
| <span id="operationalworker-contextid"></span>`contextId` | string | Present; no caller default | Opaque immutable context-version ID, not the logical context name. |
| <span id="operationalworker-incarnation"></span>`incarnation` | string | Present; no caller default | Managed worker incarnation; prevents a replacement process from impersonating an old attempt. |
| <span id="operationalworker-capacity"></span>`capacity` | integer | Present; no caller default | Concurrent request slots; worker registration accepts 1–128, Spark backend concurrency 1–32. Minimum: 1. Maximum: 128. |
| <span id="operationalworker-capabilities"></span>`capabilities` | array of [Capability](workers.md#capability) | Present; no caller default | Nonempty list (at most 256) of unique handler/version/result contracts. |
| <span id="operationalworker-draining"></span>`draining` | boolean | Optional output; no caller default | Whether the worker has stopped accepting new assignments. |
| <span id="operationalworker-heartbeatat"></span>`heartbeatAt` | string (date-time) | Optional output; no caller default | Last recorded worker/server heartbeat instant (RFC 3339). |
| <span id="operationalworker-state"></span>`state` | string | Optional output; no caller default | Current persisted or observed lifecycle state; see the model notes. Values: `ready`, `draining`, `stale`. |
| <span id="operationalworker-occupiedslots"></span>`occupiedSlots` | integer | Optional output; no caller default | Currently occupied worker request slots. |

## OperationalServer {#operationalserver}

Administrator output describing an API server process heartbeat.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="operationalserver-id"></span>`id` | string | Present; no caller default | Opaque server/resource identity; preserve it verbatim. |
| <span id="operationalserver-pod"></span>`pod` | string | Optional output; no caller default | API server Pod identity used for heartbeat deduplication. |
| <span id="operationalserver-version"></span>`version` | string | Optional output; no caller default | Version of the associated protocol, handler or configuration. |
| <span id="operationalserver-state"></span>`state` | string | Present; no caller default | Current persisted or observed lifecycle state; see the model notes. |
| <span id="operationalserver-startedat"></span>`startedAt` | string (date-time) | Optional output; no caller default | Server process start instant (RFC 3339). |
| <span id="operationalserver-heartbeatat"></span>`heartbeatAt` | string (date-time) | Present; no caller default | Last recorded worker/server heartbeat instant (RFC 3339). |
| <span id="operationalserver-detail"></span>`detail` | object | Optional output; no caller default | Optional engine/server observation details; adapter-specific object. |

## OperationalContextPage {#operationalcontextpage}

Paginated administrator output; items follow the named observation/receipt model.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="operationalcontextpage-items"></span>`items` | array of [OperationalContext](operations.md#operationalcontext) | Present; no caller default | Items in this page/receipt; follow the linked item type. |
| <span id="operationalcontextpage-nextcursor"></span>`nextCursor` | string | Optional output; no caller default | Opaque continuation token. Omission/empty value means no further page. |

## OperationalClientPage {#operationalclientpage}

Paginated administrator output; items follow the named observation/receipt model.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="operationalclientpage-items"></span>`items` | array of [OperationalClient](operations.md#operationalclient) | Present; no caller default | Items in this page/receipt; follow the linked item type. |
| <span id="operationalclientpage-nextcursor"></span>`nextCursor` | string | Optional output; no caller default | Opaque continuation token. Omission/empty value means no further page. |

## OperationalInstancePage {#operationalinstancepage}

Paginated administrator output; items follow the named observation/receipt model.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="operationalinstancepage-items"></span>`items` | array of [OperationalInstance](operations.md#operationalinstance) | Present; no caller default | Items in this page/receipt; follow the linked item type. |
| <span id="operationalinstancepage-nextcursor"></span>`nextCursor` | string | Optional output; no caller default | Opaque continuation token. Omission/empty value means no further page. |

## OperationalWorkerPage {#operationalworkerpage}

Paginated administrator output; items follow the named observation/receipt model.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="operationalworkerpage-items"></span>`items` | array of [OperationalWorker](operations.md#operationalworker) | Present; no caller default | Items in this page/receipt; follow the linked item type. |
| <span id="operationalworkerpage-nextcursor"></span>`nextCursor` | string | Optional output; no caller default | Opaque continuation token. Omission/empty value means no further page. |

## OperationalServerPage {#operationalserverpage}

Paginated administrator output; items follow the named observation/receipt model.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="operationalserverpage-items"></span>`items` | array of [OperationalServer](operations.md#operationalserver) | Present; no caller default | Items in this page/receipt; follow the linked item type. |
| <span id="operationalserverpage-nextcursor"></span>`nextCursor` | string | Optional output; no caller default | Opaque continuation token. Omission/empty value means no further page. |

## AdminOverview {#adminoverview}

Output of `GET /v1/admin/overview`, measured coherently across owners.

Coherent read-only summary over all owners. Counts are independent of pagination. queuedJobs includes retryingJobs; terminal outcomes use updatedAt within since..observedAt. Server counts use the latest heartbeat per pod in that window.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="adminoverview-runningjobs"></span>`runningJobs` | integer (int64) | Present; no caller default | Count of running work, including cancellation in progress where the read model groups it. Minimum: 0. |
| <span id="adminoverview-queuedjobs"></span>`queuedJobs` | integer (int64) | Present; no caller default | Queued demand, including retrying jobs where noted by the summary. Minimum: 0. |
| <span id="adminoverview-retryingjobs"></span>`retryingJobs` | integer (int64) | Present; no caller default | Retrying subset of queued demand, including work in backoff. Minimum: 0. |
| <span id="adminoverview-cancellingjobs"></span>`cancellingJobs` | integer (int64) | Present; no caller default | Work with committed cancellation intent that has not yet become terminal. Minimum: 0. |
| <span id="adminoverview-succeededjobs24h"></span>`succeededJobs24h` | integer (int64) | Present; no caller default | Succeeded jobs updated within the summary time window. Minimum: 0. |
| <span id="adminoverview-failedjobs24h"></span>`failedJobs24h` | integer (int64) | Present; no caller default | Failed jobs updated within the summary time window. Minimum: 0. |
| <span id="adminoverview-cancelledjobs24h"></span>`cancelledJobs24h` | integer (int64) | Present; no caller default | Cancelled jobs updated within the summary time window. Minimum: 0. |
| <span id="adminoverview-activecontexts"></span>`activeContexts` | integer (int64) | Present; no caller default | Active context-version count in the coherent summary. Minimum: 0. |
| <span id="adminoverview-liveclients"></span>`liveClients` | integer (int64) | Present; no caller default | Count of live client leases. Minimum: 0. |
| <span id="adminoverview-readyworkers"></span>`readyWorkers` | integer (int64) | Present; no caller default | Count of ready/live registered workers. Minimum: 0. |
| <span id="adminoverview-workerslots"></span>`workerSlots` | integer (int64) | Present; no caller default | Total available configured worker request capacity in the summary. Minimum: 0. |
| <span id="adminoverview-occupiedslots"></span>`occupiedSlots` | integer (int64) | Present; no caller default | Currently occupied worker request slots. Minimum: 0. |
| <span id="adminoverview-serversready"></span>`serversReady` | integer (int64) | Present; no caller default | Ready server count, deduplicated by Pod. Minimum: 0. |
| <span id="adminoverview-serversunready"></span>`serversUnready` | integer (int64) | Present; no caller default | Unready server count, deduplicated by Pod. Minimum: 0. |
| <span id="adminoverview-serversstale"></span>`serversStale` | integer (int64) | Present; no caller default | Server count whose latest Pod heartbeat is stale. Minimum: 0. |
| <span id="adminoverview-observedat"></span>`observedAt` | string (date-time) | Present; no caller default | Time of the latest observation or coherent summary (RFC 3339). |
| <span id="adminoverview-since"></span>`since` | string (date-time) | Present; no caller default | Start of the terminal-outcome summary window (RFC 3339). |
| <span id="adminoverview-oldestqueuedseconds"></span>`oldestQueuedSeconds` | number | Present; no caller default | Age in seconds of the oldest queued/retrying request, or zero. Minimum: 0. |
| <span id="adminoverview-meanprocessingseconds24h"></span>`meanProcessingSeconds24h` | number | Present; no caller default | Mean finished execution-attempt duration in the last 24 hours; excludes initial queue wait and counts retries separately. Null if no durations exist. Minimum: 0. |
| <span id="adminoverview-activecontextdetails"></span>`activeContextDetails` | array of [OverviewContext](operations.md#overviewcontext) | Present; no caller default | Bounded active-context summaries; counts above remain independent of list size. Maximum items: 5. |
| <span id="adminoverview-recentfailures"></span>`recentFailures` | array of [OverviewFailure](operations.md#overviewfailure) | Present; no caller default | Bounded recent failure summaries across owners. Maximum items: 5. |

## OverviewContext {#overviewcontext}

Active-context summary in AdminOverview.activeContextDetails.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="overviewcontext-id"></span>`id` | string | Present; no caller default | Opaque server/resource identity; preserve it verbatim. |
| <span id="overviewcontext-name"></span>`name` | string | Present; no caller default | Logical name within its containing model. |
| <span id="overviewcontext-owner"></span>`owner` | string | Present; no caller default | Authenticated application owner; supplied by the server, not application request JSON. |
| <span id="overviewcontext-state"></span>`state` | string | Present; no caller default | Current persisted or observed lifecycle state; see the model notes. |
| <span id="overviewcontext-condition"></span>`condition` | string | Optional output; no caller default | Optional diagnostic condition explaining provisioning or observation state. |
| <span id="overviewcontext-runningjobs"></span>`runningJobs` | integer (int64) | Present; no caller default | Count of running work, including cancellation in progress where the read model groups it. Minimum: 0. |
| <span id="overviewcontext-queuedjobs"></span>`queuedJobs` | integer (int64) | Present; no caller default | Queued demand, including retrying jobs where noted by the summary. Minimum: 0. |

## OverviewFailure {#overviewfailure}

Recent failed-job summary in AdminOverview.recentFailures.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="overviewfailure-id"></span>`id` | string | Present; no caller default | Opaque server/resource identity; preserve it verbatim. |
| <span id="overviewfailure-contextid"></span>`contextId` | string | Present; no caller default | Opaque immutable context-version ID, not the logical context name. |
| <span id="overviewfailure-contextname"></span>`contextName` | string | Present; no caller default | Stable logical context name. |
| <span id="overviewfailure-owner"></span>`owner` | string | Present; no caller default | Authenticated application owner; supplied by the server, not application request JSON. |
| <span id="overviewfailure-handler"></span>`handler` | string | Present; no caller default | Stable registered handler name; executable code is installed in the worker image. |
| <span id="overviewfailure-message"></span>`message` | string | Present; no caller default | Human-readable detail; progress messages are at most 4096 bytes. |
| <span id="overviewfailure-updatedat"></span>`updatedAt` | string (date-time) | Present; no caller default | Last durable job state/update instant (RFC 3339). |
