## ADDED Requirements

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
