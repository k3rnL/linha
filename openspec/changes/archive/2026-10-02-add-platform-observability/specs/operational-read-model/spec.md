## Purpose

Expose consistent, timestamped operational observations of Linha contexts, clients, workers, engines, and servers for authorized administration and aggregate monitoring.

## ADDED Requirements

### Requirement: Observation without lifecycle mutation
Linha SHALL provide paginated operational context/version/client/worker/instance/server observations using authoritative durable identities and the same lease/freshness rules as execution. Reading observations SHALL NOT create or renew client leases, ensure backends, resolve images, or change desired capacity. Owner-scoped consumers SHALL retain isolation; cross-owner detail SHALL require separate administrative authorization.

#### Scenario: Concurrent ensure and read
- **WHEN** clients concurrently ensure identical owner/name/configuration while an administrator observes the service
- **THEN** observations converge on the shared context version and its independent leases without introducing additional demand

#### Scenario: Unauthorized cross-owner detail
- **WHEN** a non-administrative caller attempts to inspect another owner's client or instance detail
- **THEN** the observation surface discloses no unauthorized data

#### Scenario: Reading a draining version
- **WHEN** a context has no live clients and only accepted work remains
- **THEN** observations explain the work demand without extending its lifetime

### Requirement: Desired and observed state with freshness
Operational detail SHALL distinguish desired configuration, durable lifecycle state, engine observations, and source timestamps. Missing, stale, or failed observations SHALL be distinguishable from successful zero counts. Client expiry and worker eligibility SHALL use authoritative database time and existing lifecycle thresholds.

#### Scenario: Kubernetes becomes unreachable
- **WHEN** the last engine observation is older than the freshness threshold or its refresh fails
- **THEN** detail reports last-known information as unavailable/stale and does not assert fresh zero executors or ready capacity

#### Scenario: Client disappears
- **WHEN** a client stops renewing and its lease expires
- **THEN** live-demand observations stop counting it independently of the observing server's wall clock

### Requirement: Fenced engine observations
Engine observations SHALL remain engine-neutral at their common boundary and support optional engine-specific detail. A persisted observation SHALL identify its resource/incarnation and only the valid controller owner SHALL update it. Stale worker/controller observations SHALL NOT override a replacement instance's state or links.

#### Scenario: Delayed observation from an old driver
- **WHEN** a driver is replaced and a delayed observation for its old UID/incarnation arrives
- **THEN** the update is rejected and current capacity, pod identity, and engine links remain associated with the replacement

#### Scenario: Another engine lacks Spark detail
- **WHEN** a supported non-Spark engine reports its generic state
- **THEN** common observations remain usable without requiring Spark fields or Kubernetes identities

### Requirement: Spark runtime detail and ingress discovery
Spark observations SHALL expose application/driver state, current owned executor states/counts, configured and observed main-container resource settings when available, and reported UI service/port/ingress metadata. Current counts SHALL exclude deleted historical executors. Clickable UI links SHALL use the current application's published ingress with a verified HTTP(S) scheme and preserve its path. Missing or invalid links SHALL be explicit; observation SHALL NOT provision an ingress or proxy.

#### Scenario: Executor history contains deleted pods
- **WHEN** SparkApplication history includes executors whose owned pods no longer exist
- **THEN** they are not counted as current running executors

#### Scenario: Published ingress is usable
- **WHEN** a current owned application exposes a valid UI ingress address
- **THEN** the observation provides that driver's published link without inventing a host or stripping its path

#### Scenario: Unsafe or ambiguous address
- **WHEN** an address uses a non-HTTP scheme or its scheme cannot be established from matching ingress metadata
- **THEN** it is not exposed as a clickable link and the reason is available to administrators

### Requirement: Server health across restart
Server observations SHALL identify each process incarnation, build/start time, readiness, heartbeat freshness, and available background-loop/dependency observations. Recently missing servers SHALL be distinguishable from ready replicas. Expiry of an observational heartbeat SHALL NOT become a new source of job ownership or engine fencing.

#### Scenario: One server crashes
- **WHEN** one replica stops without graceful shutdown while another remains healthy
- **THEN** the stopped incarnation becomes stale after its heartbeat threshold and the surviving server continues to serve authoritative jobs

#### Scenario: All servers restart
- **WHEN** all Linha servers restart while PostgreSQL survives
- **THEN** new process observations coexist with bounded stale history, durable jobs/leases remain authoritative, and last-known engine details carry their original timestamps until refreshed

#### Scenario: Database heartbeat cannot be written
- **WHEN** PostgreSQL is unavailable to a responding process
- **THEN** its local health metrics remain available while shared heartbeat/detail collection is marked unavailable
