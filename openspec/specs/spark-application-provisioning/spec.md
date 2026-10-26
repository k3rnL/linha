# spark-application-provisioning Specification

## Purpose
Provision managed Spark workers through operator submission with consistent heap and container resources, while preserving ownership and user Pod customization.

## Requirements

### Requirement: Managed Spark submission
New managed Spark contexts SHALL declare a main class and local application JAR and SHALL run through SparkApplication cluster submission with no operator restart loop.
#### Scenario: Heap plus overhead
- **WHEN** a client requests memory 10g and memoryOverhead 2g
- **THEN** Spark submission SHALL configure a 10g JVM heap and a 12Gi container request and limit
#### Scenario: Native customization
- **WHEN** a client supplies validated volumes, environment or pull secrets
- **THEN** the driver and executor templates SHALL preserve them with managed identities protected
### Requirement: Fenced lifecycle
Linha SHALL observe SparkApplication and driver identity separately and SHALL only delete resources it owns using UID preconditions.
#### Scenario: Pending driver
- **WHEN** a SparkApplication exists before its driver Pod
- **THEN** the instance SHALL remain starting without duplicate submission
#### Scenario: Lost driver
- **WHEN** a driver disappears while capacity is needed
- **THEN** Linha SHALL retire its application and provision a replacement instance
#### Scenario: Legacy stored context
- **WHEN** a previously persisted context lacks application settings
- **THEN** Linha SHALL retain its legacy provisioning path while draining accepted work

### Requirement: Context-readable driver names
New operator-managed Spark driver Pods SHALL use a readable prefix derived from their logical context name, followed by the full instance ID and `-driver`. Names SHALL be deterministic valid Kubernetes DNS labels of at most 63 characters. Normalization SHALL lowercase ASCII letters, replace runs of unsupported characters with a hyphen, trim boundary hyphens and use `context` when no readable characters remain. Only the context prefix SHALL be truncated when needed; the full instance ID SHALL remain intact.

#### Scenario: Concurrent versions and replacements
- **WHEN** different versions, owners or replacement drivers share a logical context name
- **THEN** their distinct full instance IDs keep the driver names distinct, while repeated submission of the same instance produces the same name

#### Scenario: Arbitrary context names
- **WHEN** a context name contains spaces, punctuation, non-ASCII characters or a long prefix
- **THEN** submission produces a valid bounded driver name without shortening its instance ID

### Requirement: Named driver identity fencing
Context-readable driver names SHALL preserve worker-token Pod UID and service-account checks, driver role, matching SparkApplication ownership and durable instance incarnation authorization. Existing driver names SHALL remain observable and authorizable after a server restart without forced driver replacement.

#### Scenario: Named driver authentication and impersonation
- **WHEN** a named Pod presents worker credentials
- **THEN** access is allowed only for its matching instance and context with the required role, application owner, Pod UID, service account and assigned incarnation; a missing or mismatched binding is rejected

#### Scenario: Existing application after upgrade
- **WHEN** an existing SparkApplication reports its original driver name after the server restarts
- **THEN** Linha continues observing that reported Pod and applies the same identity and UID deletion fences without renaming its resources
