## ADDED Requirements

### Requirement: Bounded retired-resource discovery
Retired instances SHALL NOT cause repeated per-instance Kubernetes GET or DELETE requests for absent resources. Cleanup SHALL periodically discover existing Linha-managed resources within the deployment namespace, at most once per minute per server outside server restarts, with pagination as needed. Cleanup SHALL preserve durable contexts, requests, attempts and result metadata.

#### Scenario: Many absent retired instances
- **WHEN** thousands of retired instance records remain but their resources no longer exist
- **THEN** ordinary capacity reconciliation makes no Kubernetes requests for those records, and periodic cleanup request volume is independent of their number

#### Scenario: Late create after failover or restart
- **WHEN** a stale controller's create completes for a durably retired instance, including one belonging to a stopped context
- **THEN** a subsequent successful cleanup sweep discovers and removes that resource even after all servers restart

#### Scenario: Legacy Service survives its driver
- **WHEN** a retired legacy driver Pod is absent but its Service exists
- **THEN** cleanup removes the Service without repeatedly probing the absent Pod

#### Scenario: Concurrent ensure and unknown ownership
- **WHEN** discovery includes an active instance, a new ensure intent, an unknown instance, or labels that do not match the durable context and resource kind
- **THEN** cleanup does not delete those resources or change their requests

#### Scenario: HA ownership and replacement during deletion
- **WHEN** another server owns reconciliation or a resource UID changes between discovery and deletion
- **THEN** cleanup respects context ownership and UID preconditions and does not delete the replacement resource using stale identity

#### Scenario: Cleanup cannot reach Kubernetes
- **WHEN** inventory or deletion fails
- **THEN** the failure is observable, cleanup retries only on the bounded sweep cadence, and active capacity reconciliation continues

#### Scenario: Stopped context is reactivated
- **WHEN** an authorized client attaches to a stopped context version
- **THEN** ordinary reconciliation resumes promptly, creates new capacity without reusing retired instance IDs, and retains earlier job status and results
