# ha-deployment Specification

## Purpose

Deploy Linha redundantly on Kubernetes while preserving authoritative requests, results, and backend management through replica and process failures.

## Requirements


### Requirement: HA Helm deployment
Linha SHALL provide a Helm chart supporting multiple server replicas, a shared service endpoint, health probes, graceful termination, disruption controls, secret references, and least-privilege backend-management permissions. The default production-oriented server replica count SHALL be at least two.

#### Scenario: Install with external dependencies
- **WHEN** the chart is installed with valid database, identity, result-store, and engine settings
- **THEN** multiple replicas serve one logical job service using the same persistent state

### Requirement: Shared durable coordination
Public requests and worker operations SHALL be safe through any healthy replica without sticky sessions. Backend controllers and migrations SHALL coordinate shared work across replicas. HA documentation SHALL identify HA PostgreSQL and durable result storage as dependencies.

#### Scenario: One server replica fails
- **WHEN** the replica serving a client or worker stops
- **THEN** subsequent calls through a healthy replica observe the same jobs and continue valid leases without losing acknowledged work

#### Scenario: Two replicas start migrations together
- **WHEN** multiple replicas or deployment processes attempt schema initialization concurrently
- **THEN** migrations run under exclusive coordination and traffic is admitted only against a compatible schema

### Requirement: Full process restart recovery
After restart, the service SHALL reconstruct queued jobs, attempt outcomes, committed results, context requirements, and worker reconciliation from durable state. No process-local map SHALL be the sole authority for acknowledged requests or desired backends.

#### Scenario: All application processes stop and restart
- **WHEN** every server/controller/worker process restarts while persistent stores retain their data
- **THEN** completed results remain retrievable, queued jobs remain schedulable, interrupted attempts resolve under policy, and required backend capacity is restored

### Requirement: Persistence outage behavior
When required persistence is unavailable, the service SHALL stop accepting durable mutations and new claims, mark affected replicas unready, and preserve liveness independently of transient dependency failure. Workers SHALL obey ownership deadlines during the outage.

#### Scenario: Database outage exceeds attempt leases
- **WHEN** workers cannot renew leases during a database outage
- **THEN** they attempt to stop execution and recovery rejects unauthorized late completion once persistence returns

### Requirement: Operational visibility and configuration safety
Deployment SHALL expose correlation by context/job/attempt, queue age/depth, worker health/capacity, retry/lease events, provisioning failures, and result errors. Logs SHALL avoid recording secret credentials or entire request/result payloads by default.

#### Scenario: Backend repeatedly fails to start
- **WHEN** a worker image cannot initialize after bounded retries
- **THEN** operators can identify the affected context and provisioning cause without inspecting private request payloads

### Requirement: Local result mounts and provider recovery
Deployment SHALL support references to caller-managed persistent result volumes and mounts for server replicas, worker drivers, and executors that require access. Documentation SHALL distinguish a local filesystem provider backed by shared durable storage from ephemeral or unrelated node-local disks. Readiness and result access SHALL expose missing required storage; deployment SHALL NOT silently create an ephemeral substitute.

#### Scenario: Result is read through another replica
- **WHEN** a result is committed to a configured shared local filesystem and the serving replica is replaced
- **THEN** another properly mounted replica can serve the same authorized result using its persisted reference

#### Scenario: Restore with a missing required mount
- **WHEN** a server or writer restarts without access to its configured result mount
- **THEN** the affected component exposes a storage/readiness failure without claiming that unavailable payloads are durably accessible

### Requirement: Independent deployment and security settings
The chart SHALL expose PostgreSQL connection settings under database.postgres and read credentials using secretRef, usernameKey, and passwordKey. OIDC SHALL be configurable under security.oidc. A master security.enabled switch SHALL disable all Linha authentication without a deployment mode. With only OIDC disabled, worker authentication SHALL remain enabled. Anonymous public access SHALL use one stable owner and SHALL NOT promise per-client isolation or remap retained ownership.

#### Scenario: Custom database Secret keys
- **WHEN** the administrator selects a Secret and custom username/password keys
- **THEN** the chart reads those keys independently and passwords containing URI syntax remain literal credentials

#### Scenario: Authentication disabled
- **WHEN** security.enabled is false
- **THEN** clients and workers operate without OIDC or bearer credentials using a stable anonymous owner and worker routing identities, while durable leases and attempt fencing still apply

#### Scenario: Only OIDC disabled
- **WHEN** security is enabled and security.oidc.enabled is false
- **THEN** public requests use the anonymous owner and worker requests still require verified assigned identities

### Requirement: Namespace-only backend permissions
The chart SHALL offer namespace-only RBAC without rendering ClusterRoles or ClusterRoleBindings, including when authentication is enabled. Signed projected worker tokens SHALL be verified against Kubernetes issuer keys and checked against live Pod/service-account identity and persisted instance assignments. TokenReview cluster permission SHALL be opt-in. All engine resources SHALL remain in the release namespace in either variant.

#### Scenario: Namespace-only authenticated installation
- **WHEN** the administrator installs with rbac.namespaceOnly true and standard Kubernetes issuer-discovery access
- **THEN** worker authentication succeeds without TokenReview calls or Linha cluster-scoped RBAC

#### Scenario: Token belongs to a replaced Pod or service account
- **WHEN** a correctly signed token refers to a deleted, terminating, or replaced bound identity
- **THEN** the worker operation is rejected even if the JWT has not expired

#### Scenario: Signing-key discovery is inaccessible
- **WHEN** namespace-only worker validation cannot access the trusted issuer configuration
- **THEN** the server reports a configuration/dependency failure without disabling worker authentication
