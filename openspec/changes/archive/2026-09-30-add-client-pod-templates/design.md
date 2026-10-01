## Context

See proposal.md. Linha directly creates drivers; classic Spark in client mode creates
executors. Context JSON is durable but deployment mount defaults are currently read
at provisioning time. Image resolution currently receives only an image string.

## Goals / Non-Goals

Goals: preserve the generic job lifecycle, support native Pod settings, make the
accepted effective configuration reproducible, and keep namespace-only installations.
Non-goals: arbitrary remote template fetching, managing application Secrets/PVCs,
live reconfiguration of shared SparkContexts, and changes to result retention.

## Decisions

- Use native metadata/spec fragments, with only labels/annotations in metadata and
  a main container named main. Decode Pod specs against pinned Kubernetes API types
  to reject typos. Retain generic JSON at the engine boundary. Native cluster admission
  still determines which supported Pod features may run in the trusted namespace.
- Merge maps recursively; merge containers by name recursively; replace same-name env,
  volumes and imagePullSecrets entries; merge mounts by mountPath; replace other lists.
  Reject duplicate keys in named lists. Empty lists add no named entries; there is no
  delete/JSON-Patch language in v1. Validate both client and merged defaults. Preserve
  main CPU/memory through Spark settings, allow other resource fields, and reserve
  Linha/Spark variables, generated labels, runtime mounts and lifecycle fields.
- Keep Normalize independent of mutable Helm defaults, then prepare effective settings
  through an optional engine hook. Add requested_spec JSONB to contexts (schema v4).
  Compare requested_spec on repeat ensure; return the stored effective spec. Legacy
  records fall back to comparing their existing spec and keep the pre-template path.
  Acceptance, ownership, attempts, retry policy and result publication are unchanged.
- Store merged defaults in new context engine settings. Existing workerVolumes/mounts
  feed new driver defaults; their historical PVC-only executor propagation remains.
  Add workerPodTemplates for independent native defaults. The top-level imagePullSecrets
  become explicit defaults for both roles, so image pulls and resolver access agree.
- Render a context-scoped immutable ConfigMap named linha-<contextId>-executor containing
  executor.json. Mount it in the driver at /var/run/linha-pod-templates and configure
  Spark's executor podTemplateFile and podTemplateContainerName. JSON is valid YAML.
  Its content does not contain instance identity; it is reusable for all context drivers.
  Create is idempotent with label/data/immutability checks. The ConfigMap is retained
  for the context's lifetime (context deletion is not implemented). Missing template
  resources are recreated on provisioning; caller-owned config resources are never edited.
- Image resolution receives BackendSpec through the controller's generic callback.
  Read selected, permitted dockerconfigjson/dockercfg Secrets from the namespace;
  construct a registry keychain and keep administrator default-keychain fallback.
  Use registrySecrets.allowedNames plus deployment imagePullSecrets as permitted names.
  Helm grants get with resourceNames, never list/watch or unrestricted Secret reads.
  Additional container images pass repository policy; digest references are recommended
  for those images, while Linha continues pinning the primary worker image itself.
- Scala client helpers use immutable Circe JSON and safe bounded YAML parsing; no
  Kubernetes client dependency is required in the JVM SDK. YAML files are expanded
  and sent as JSON data. Main container helpers use the same named-entry merge rules.

## Risks / Trade-offs

- Mutable referenced ConfigMaps change application content independently of context
  identity: document versioned ConfigMap names; keep Secret rotation possible.
- Spark overrides parts of executor templates: reject conflicting managed fields and
  translate supported executor pull policy; verify with real Spark executor creation.
- Pod creation is a privileged capability within the namespace: retain the trusted
  application-image boundary, image policy and cluster admission policies.
- Default changes must not change repeat ensure: test concurrent acceptance, migration,
  restart and persisted rendering using PostgreSQL.
- ConfigMap size/env limits: bound each template to 64 KiB and reject oversized settings.

## Migration Plan

Apply the additive v4 migration under existing migration locking. Upgrade server and
Helm together for template ConfigMap and scoped registry Secret reads. Existing JVM
workers already consume LINHA_SPARK_CONF, so executor template support requires no
new worker bootstrap contract. Clients opt into the new SDK helpers or raw fields.
Legacy contexts without persisted templates retain legacy deployment defaults; use a
new context name to adopt snapshotted templates. Rollback after v4 needs a server that
understands that schema; retain the additive column and data.
