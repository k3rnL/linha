## Purpose

Allow applications to configure reusable Spark driver and executor Pods from the client while preserving durable backend identity and managed lifecycle behavior.

## ADDED Requirements

### Requirement: Per-context Pod templates
Ensure SHALL accept independent driverPodTemplate and executorPodTemplate values under engine.settings.kubernetes. Templates SHALL use Kubernetes metadata/spec structure and support application files through ConfigMap/Secret/PVC references, environment variables, pull secrets, scheduling settings, init containers and sidecars. The container named main SHALL identify the managed runtime. Scala helpers SHALL wrap the same JSON protocol and load JSON/YAML template contents on the client.

#### Scenario: Application config on both roles
- **WHEN** a client supplies config volumes and mounts on driver and executor templates
- **THEN** the driver and subsequently created executor Pods receive those settings without changing deployment-wide mounts

#### Scenario: Raw and Scala client equivalence
- **WHEN** clients send equivalent native template data through JSON or Scala helpers
- **THEN** they receive equivalent persisted contexts without requiring template files on the server or original client after acceptance

### Requirement: Durable defaults and context identity
The server SHALL merge deployment defaults with client overrides, preserve requested identity separately from effective configuration, and persist both before acknowledging ensure. Named entries SHALL merge deterministically without losing unrelated defaults. Identical normalized effective settings SHALL share a version under an owner/name. Changed deployment defaults or requested templates SHALL derive new versions under that same name. Attaching to an existing version SHALL preserve its original effective settings. Replacement drivers SHALL reuse persisted effective templates.

#### Scenario: Defaults change after acceptance
- **WHEN** the server restarts with different Pod defaults and the client repeats its original ensure
- **THEN** a new version is returned; attaching to the old version and replacing its Pods retains the original settings

#### Scenario: Concurrent ensure with different settings
- **WHEN** callers concurrently ensure one owner/name using different templates
- **THEN** the settings derive distinct immutable versions under the same logical name

### Requirement: Managed lifecycle fields remain authoritative
Linha SHALL reject attempted overrides of runtime identity, namespace, owner references, generated labels, main command/args/image, runtime environment, and protected mounts. Main CPU/memory settings SHALL continue to use the Spark resource settings. All additional container images SHALL satisfy image policy. Errors SHALL identify the offending field. Referenced application objects SHALL remain in the deployment namespace and caller-managed.

#### Scenario: Worker identity override
- **WHEN** a template attempts to replace a Linha identity environment variable or mount
- **THEN** ensure rejects it before provisioning, preserving worker incarnation fencing

#### Scenario: Foreign context access
- **WHEN** a caller requests another owner's context containing Pod templates
- **THEN** existing ownership restrictions still prevent disclosure or mutation

### Requirement: Executor template materialization
Linha SHALL materialize the persisted executor template for Spark to consume. Template resources SHALL have deterministic context-scoped identities and conflicts with unrelated or altered resources SHALL fail explicitly. Replacing a driver after server/client restarts SHALL not require resubmission of the template.

#### Scenario: Repeated provisioning
- **WHEN** controllers repeat creation after an ambiguous response or driver replacement
- **THEN** they reuse the matching template resource and never overwrite unrelated configuration

### Requirement: Scoped private registry credentials
Selected image pull Secret references SHALL be used both for Pod pulls and for resolving the primary worker image digest. Server Secret reads SHALL be restricted to administrator-permitted names in the deployment namespace. Secret values SHALL NOT be included in context/job records or error messages. Credentials SHALL be reread when resolution is retried.

#### Scenario: Private worker image
- **WHEN** the selected permitted Secret contains valid credentials for the worker registry
- **THEN** Linha resolves and pins the image and driver/executor Pods receive their selected pull references

#### Scenario: Secret outside policy
- **WHEN** a client selects an unpermitted pull Secret
- **THEN** ensure rejects the reference before any Secret read

### Requirement: Namespace-only deployment compatibility
The Helm chart SHALL expose Pod defaults and registry Secret permissions using namespaced Roles. Enabling Pod customization SHALL NOT require a ClusterRole in namespace-only mode. Existing contexts and clients without template fields SHALL remain usable, with legacy context behavior documented.

#### Scenario: Namespace-only chart
- **WHEN** templates and registry credentials are configured with namespaceOnly true
- **THEN** the chart grants only required namespaced resource permissions and no ClusterRole or ClusterRoleBinding
