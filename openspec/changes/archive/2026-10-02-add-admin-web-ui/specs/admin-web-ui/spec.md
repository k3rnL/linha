## Purpose

Provide administrators with a browser console for inspecting owned contexts across applications, their execution resources, durable requests, and published results.

## ADDED Requirements

### Requirement: Optional embedded administration console
Linha SHALL offer an optional console served by each server replica under `/ui/`, with contexts as its landing page, deep links, and a shared create-request page. Disabling the console SHALL disable its browser-session and administrative HTTP endpoints without affecting existing SDK APIs.

#### Scenario: Console is disabled
- **WHEN** Linha runs with the console disabled
- **THEN** its assets, login routes, and administrative APIs are unavailable while ordinary job APIs continue to work

#### Scenario: Open a detail URL directly
- **WHEN** an authorized administrator opens a context or request deep link through any server replica
- **THEN** the correct page loads and unknown API URLs remain API errors rather than returning frontend HTML

### Requirement: Explicit administrator authorization
With security enabled, the server SHALL require validated OIDC identity and explicit configured administrator membership for every administrative read or mutation. A viewer SHALL have cross-owner inspection/result access; an operator SHALL additionally create, replay, and cancel requests. Ordinary SDK APIs SHALL retain their owner scope. Enabling the UI with security enabled but OIDC disabled SHALL fail configuration validation. With the master security switch disabled, an explicitly enabled UI SHALL use anonymous operator access and attribution.

#### Scenario: Application identity is not an administrator
- **WHEN** an authenticated ordinary client calls an admin endpoint or supplies a forged owner/role header
- **THEN** access is denied without exposing another owner's details or applying a mutation

#### Scenario: Viewer attempts cancellation
- **WHEN** a viewer submits a creation, replay, or cancellation operation directly to the API
- **THEN** the server rejects it even if the browser control was bypassed

#### Scenario: Independent security settings
- **WHEN** the UI is enabled under each supported security configuration
- **THEN** master-disabled security permits anonymous operation, configured OIDC requires admin membership, and security-enabled/OIDC-disabled configuration does not grant anonymous administration

### Requirement: HA browser authentication
Browser authentication SHALL validate issuer, signature, audience, expiry, state, nonce, and the code exchange, and SHALL protect cookie-authenticated mutations against cross-site requests. Browser sessions and login transactions SHALL work across replicas without sticky sessions, expire within validated identity lifetime, and be revocable by logout. Authentication credentials SHALL NOT be exposed in frontend configuration or browser local storage.

#### Scenario: Login crosses replicas
- **WHEN** login begins on one replica, returns to another, and the first replica restarts
- **THEN** the valid administrator can finish login and use the same session through a healthy replica

#### Scenario: Invalid login or cross-site mutation
- **WHEN** a callback has invalid state/nonce/audience or a mutation lacks required origin/CSRF validation
- **THEN** the request is rejected without creating a session or modifying a job

### Requirement: Context-centred inspection
The console SHALL group contexts by owner and logical name and expose configuration versions, lifecycle state/conditions, image/configuration, result policy, client demand/expiry, engine instances, and related requests. Running or provisioning/draining contexts SHALL be the default view with stopped history explicitly available. Views SHALL be filtered, paginated, and observational only.

#### Scenario: Concurrent ensure and rolling versions
- **WHEN** API replicas concurrently ensure identical configuration and then one ensures a changed configuration under the same name
- **THEN** the UI shows one shared original version with independent client leases and a separate new version, including which work and clients keep each version alive

#### Scenario: Viewing an unused version
- **WHEN** an administrator opens or refreshes a stopped or draining context
- **THEN** the view creates no client lease, schedules no capacity, and does not delay retirement

#### Scenario: Same name under two owners
- **WHEN** different owners use the same logical context name
- **THEN** the console distinguishes their groups, versions, clients, and requests

### Requirement: Engine-specific details and published links
The console SHALL show generic engine readiness/capacity and supported engine details with observation timestamps. For Spark it SHALL show each driver/application, executor state/counts, configured and observed resource settings where available, and the current validated ingress link supplied by the operational read model. Linha SHALL NOT create an ingress or proxy Spark UI traffic as part of this capability.

#### Scenario: SparkApplication publishes an ingress
- **WHEN** a current owned SparkApplication reports a valid published UI ingress address
- **THEN** its driver row offers that URL in a separate tab and preserves the published path

#### Scenario: No ingress or replaced driver
- **WHEN** an application has no usable ingress, its driver disappears, or its observation becomes stale
- **THEN** the UI explains the unavailable link and available service details without presenting an old incarnation's URL as live

#### Scenario: Another engine or missing resource usage
- **WHEN** an engine does not supply Spark-specific fields or actual resource-usage measurements
- **THEN** unsupported fields are marked unavailable or omitted, and configured limits are not represented as measured usage

### Requirement: Request and server investigation
Administrators SHALL browse global or context-filtered requests by state and submission time and inspect input, progress, attempts, worker association, durable diagnostics, provenance, and terminal outcome. Server views SHALL expose available heartbeat/readiness/loop observations. Views SHALL distinguish empty results from unavailable or stale observations and SHALL remain useful without Prometheus or Grafana.

#### Scenario: Inspect a failure after all servers restart
- **WHEN** all Linha servers restart after a job fails
- **THEN** its original ID, input, attempts, exception/stack trace/truncation information, and audit provenance remain inspectable

#### Scenario: Observation refresh fails
- **WHEN** a live-data refresh fails or a server heartbeat expires
- **THEN** the console shows failure/staleness and last observation time instead of a healthy empty list

### Requirement: Primitive published result inspection
The console SHALL show committed result descriptors, published local paths or S3 URIs, sizes/content types/checksums where recorded, retention state, and paginated dataset parts. JSON previews SHALL be bounded and escaped. Authorized downloads SHALL reuse result ownership/publication rules. Staging references, delegated credentials, and unpublished attempts SHALL NOT be exposed as results.

#### Scenario: Retained result after context shutdown
- **WHEN** an administrator opens a successful request after its context stops
- **THEN** published file/dataset metadata and retained content remain accessible independently of the engine

#### Scenario: Oversized or expired result
- **WHEN** JSON exceeds the preview cap or result bytes have expired or become unavailable
- **THEN** the UI displays truncation or availability explicitly while preserving meaningful metadata and without loading the entire result into browser memory

#### Scenario: Result contains HTML or credentials are held internally
- **WHEN** a result includes executable-looking text or its provider uses temporary access credentials
- **THEN** the preview is inert text and credentials are absent from result-location responses
