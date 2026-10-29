# Backend configuration

A backend is the worker image, engine configuration and storage policy submitted
to `POST /v1/contexts:ensure`. In the Scala SDK this is the `spec: io.circe.Json`
argument of `LinhaClient.ensureBackend(name, spec)`; there is no typed Scala
`BackendSpec` builder.

Follow the nested objects: **BackendSpec → EngineSpec → SparkSettings →
application / drivers / executors / kubernetes / ui**. Results and application
storage are separate backend settings.

| Configuration area | Reference |
| --- | --- |
| Worker JAR, main class, arguments | [Spark application](spark.md#sparksettings-application-model) |
| Driver count, request concurrency, CPU and heap | [Driver settings](spark.md#sparksettings-drivers-model) |
| Executor count, allocation and retention | [Executor settings](spark.md#sparksettings-executors-model) |
| Environment, files, scheduling and sidecars | [Pod templates](pod-templates.md) |
| Retained outputs and path templates | [Result policy](results.md) |
| Input/shared application files | [Workload storage](../storage.md#workloadstoragepolicy) |

Only fields documented for Linha's chosen adapter are supported. Spark's entire
configuration namespace and the SparkApplication CRD are not accepted as a
generic backend configuration object.


Field paths below are relative to their containing model; output fields have no caller default.

## Backend declaration examples

These examples ensure the same backend. The JSON tab is the HTTP body for
`POST /v1/contexts:ensure`; the Scala tab uses the existing SDK and takes an
already authenticated `LinhaClient`.

=== "JSON"

    ```json
    {
      "name": "reporting",
      "spec": {
        "image": "registry.example/workers/reporting:1",
        "engine": {
          "type": "spark",
          "version": "3.5.6",
          "settings": {
            "application": {
              "mainClass": "example.WorkerMain",
              "mainApplicationFile": "local:///opt/linha/worker.jar"
            }
          }
        },
        "results": {"type": "s3", "destination": "analytics"}
      }
    }
    ```

=== "Scala"

    ```scala
    import io.circe.parser.parse
    import linha.client.{LinhaClient, LinhaContext}
    import scala.concurrent.{ExecutionContext, Future}

    object BackendExample {
      def ensure(client: LinhaClient)(implicit ec: ExecutionContext): Future[LinhaContext] = {
        val spec = parse("""{
          "image": "registry.example/workers/reporting:1",
          "engine": {
            "type": "spark", "version": "3.5.6",
            "settings": {"application": {
              "mainClass": "example.WorkerMain",
              "mainApplicationFile": "local:///opt/linha/worker.jar"
            }}
          },
          "results": {"type": "s3", "destination": "analytics"}
        }""").fold(throw _, identity)
        client.ensureBackend("reporting", spec)
      }
    }
    ```

Keep the returned context for the application process lifetime; it renews a
client attachment. Close it when this process no longer submits work. For a
complete worker image and main method, follow the [tutorial](../../walkthrough.md)
and consult the [Spark runtime API](../sdk/spark.md).

## EnsureRequest {#ensurerequest}

Input to `POST /v1/contexts:ensure`; returns Context with a client lease.

Ensures an immutable version under a stable owner/name, hashing normalized effective configuration and the resolved worker image. Changed settings create another version. Acquires or renews a client lease.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="ensurerequest-name"></span>`name` | string | Required; no default | Stable logical workload name, 1–128 bytes; scoped to the authenticated owner. Keep the name when changing configuration. Minimum length: 1. Maximum length: 128. |
| <span id="ensurerequest-spec"></span>`spec` | [BackendSpec](index.md#backendspec) | Required; no default | Effective normalized backend configuration; follow BackendSpec. |
| <span id="ensurerequest-clientid"></span>`clientId` | string | Generated ID | Unique process identity; generated if omitted. Renew the returned lease. Maximum length: 128. |
| <span id="ensurerequest-hostname"></span>`hostname` | string | Omitted | Optional client-reported hostname (at most 253 UTF-8 bytes, no whitespace/control characters). Display metadata only; never identity or authorization. Maximum length: 253. |

## BackendSpec {#backendspec}

Input `EnsureRequest.spec`; the effective configuration is normalized, hashed and retained in Context.spec.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="backendspec-image"></span>`image` | string | Required; no default | Primary worker image containing executable handlers; must satisfy the operator image allow-list and resolves to a digest. |
| <span id="backendspec-engine"></span>`engine` | [EngineSpec](index.md#enginespec) | Required; no default | Adapter type, engine version and nested engine settings. |
| <span id="backendspec-results"></span>`results` | [ResultPolicy](results.md#resultpolicy) | Required; no default | Result publication/storage policy, separate from application file access. |
| <span id="backendspec-storage"></span>`storage` | object ([fields](#backendspec-storage-model)) | No application access | Optional WorkloadStoragePolicy: destinations lists 1–16 unique operator-enabled aliases. Server snapshots destination fingerprints. |

### storage {#backendspec-storage-model}

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="backendspec-storage-destinations"></span>`storage.destinations` | array of string | Required; no default | 1–16 unique S3 destination aliases with allowWorkloadAccess enabled by the operator; grants access to each entire configured prefix. Minimum items: 1. Maximum items: 16. |

## EngineSpec {#enginespec}

Input `BackendSpec.engine`; selects the adapter and its versioned settings.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="enginespec-type"></span>`type` | string | Required; no default | `spark` for managed deployment. The development-only `fake` adapter requires its explicit server flag. |
| <span id="enginespec-version"></span>`version` | string | Required; no default | `3.5.6` for Spark; `1` for the enabled development fake adapter. Other values are rejected. |
| <span id="enginespec-settings"></span>`settings` | [SparkSettings](spark.md#sparksettings) for Spark | `{}`; Spark still requires application | Engine-specific object. Follow the complete nested Spark fields; fake settings must be empty. |

## Context {#context}

Output of ensure, attach and context status. A persisted immutable backend version under an owner/name.

Immutable backend version. GET does not renew a client lease. DRAINING finishes accepted work; STOPPED retains job history and results. Ensure and attach return clientLease.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="context-id"></span>`id` | string | Present; no caller default | Opaque server/resource identity; preserve it verbatim. |
| <span id="context-name"></span>`name` | string | Present; no caller default | Logical name within its containing model. |
| <span id="context-spec"></span>`spec` | [BackendSpec](index.md#backendspec) | Present; no caller default | Effective normalized backend configuration; follow BackendSpec. |
| <span id="context-state"></span>`state` | string | Present; no caller default | STARTING, READY, DRAINING or STOPPED. With no live clients, accepted work drains before drivers stop. |
| <span id="context-condition"></span>`condition` | string | Optional output; no caller default | Optional diagnostic condition explaining provisioning or observation state. |
| <span id="context-createdat"></span>`createdAt` | string (date-time) | Present; no caller default | Server-recorded creation instant (RFC 3339). |
| <span id="context-version"></span>`version` | string | Present; no caller default | SHA-256 configuration identity derived from normalized settings, storage policy, template snapshot and resolved primary image digest. |
| <span id="context-clientlease"></span>`clientLease` | [ClientLease](index.md#clientlease) | Optional output; no caller default | Lease returned by ensure/attach. A plain GET neither attaches nor renews it. |

## ClientLease {#clientlease}

Output returned by ensure/attach/renew. Records an application process attachment, not worker execution authority.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="clientlease-id"></span>`id` | string | Present; no caller default | Opaque server/resource identity; preserve it verbatim. |
| <span id="clientlease-clientid"></span>`clientId` | string | Present; no caller default | Application process identifier, at most 128 bytes. Omission generates an ID; the SDK reuses one per client. |
| <span id="clientlease-expiresat"></span>`expiresAt` | string (date-time) | Present; no caller default | Server-recorded expiry instant (RFC 3339); check before using retained access. |
| <span id="clientlease-durationseconds"></span>`durationSeconds` | integer | Present; no caller default | Lease duration in seconds; client attachments currently use 90. Values: `90`. |
| <span id="clientlease-hostname"></span>`hostname` | string | Optional output; no caller default | Optional client-reported hostname (at most 253 UTF-8 bytes, no whitespace/control characters). Display metadata only; never identity or authorization. Maximum length: 253. |

## AttachClient {#attachclient}

Input to `POST /v1/contexts/{context}/clients`; acquires a lease on an existing version.

| Field | Type | Presence / default | Meaning and constraints |
| --- | --- | --- | --- |
| <span id="attachclient-clientid"></span>`clientId` | string | Generated ID | Application process identifier, at most 128 bytes. Omission generates an ID; the SDK reuses one per client. Maximum length: 128. |
| <span id="attachclient-hostname"></span>`hostname` | string | Optional; omitted | Optional client-reported hostname (at most 253 UTF-8 bytes, no whitespace/control characters). Display metadata only; never identity or authorization. Maximum length: 253. |
