## Purpose

Allow clients to enable the Spark driver UI and control executor retention on long-lived managed contexts without rebuilding their worker image.

## ADDED Requirements

### Requirement: Per-context Spark UI configuration
Spark settings SHALL accept optional `ui.enabled` (default false) and `ui.port` (default 4040, integer 1024–65535). The configured values SHALL reach both SparkApplication submission and legacy driver configuration. Enabling the UI SHALL NOT create an externally accessible Ingress.

#### Scenario: Enabled UI on a custom port
- **WHEN** a client ensures a backend with `ui: {enabled: true, port: 4050}`
- **THEN** its driver receives `spark.ui.enabled=true` and `spark.ui.port=4050`

#### Scenario: Invalid port
- **WHEN** a client supplies port 0, a privileged port, a fractional port or a port above 65535
- **THEN** ensure fails with a validation error before provisioning

### Requirement: Configurable executor retention
Spark executor settings SHALL accept `executorIdleTimeout`, `cachedExecutorIdleTimeout`, and `shuffleTrackingTimeout`. Finite durations SHALL use a positive integer with `s`, `m`, `h`, or `d`, up to 2147483647 seconds. Cache and shuffle timeouts SHALL also accept `infinity`. Omitted or empty values SHALL preserve Spark defaults (60 seconds, unlimited, unlimited respectively). Timeouts SHALL apply only when dynamic allocation is enabled, and SHALL NOT override the configured minimum executor count.

#### Scenario: Finite retention
- **WHEN** dynamic allocation is enabled with timeouts `60s`, `2m`, and `2m`
- **THEN** both launch paths convey the corresponding Spark allocation settings without altering min/initial/max counts

#### Scenario: Invalid duration
- **WHEN** a timeout is negative, zero, fractional, unitless, too large, or uses an unsupported unit
- **THEN** ensure fails with a field-specific validation error

#### Scenario: Static allocation
- **WHEN** dynamic allocation is disabled but timeout settings are supplied
- **THEN** static executor count is retained and removal timeouts are inactive

### Requirement: Stable version identity and persistence
Equivalent durations and explicit defaults SHALL normalize identically. Omitted settings SHALL preserve existing normalized backend configuration. Changed effective settings SHALL participate in immutable context versioning and survive persistence and driver replacement.

#### Scenario: Concurrent equivalent ensures
- **WHEN** clients under the same owner and name concurrently ensure otherwise identical configurations using `2m` and `120s`
- **THEN** the configurations have the same version identity and share the existing lease rules

#### Scenario: Restart and stale worker
- **WHEN** a server restarts and replaces a lost driver with configured UI and retention values
- **THEN** persisted values are reapplied and stale workers remain subject to existing attempt fencing

#### Scenario: Another owner
- **WHEN** another owner supplies identical Spark settings
- **THEN** existing owner isolation still prevents access to the first owner's contexts, jobs and results
