## Purpose

Distribute verified Linha container images and JVM SDK releases through public
registries with consistent versions, useful metadata and accountable publishers.

## ADDED Requirements

### Requirement: Validation before publication
Branch and pull-request builds SHALL validate the Go server, JVM SDKs, browser
console, persistence/restart behavior and deployment contracts without publishing.
Version-tag releases SHALL run the same checks for the exact tagged commit before
any registry writes. Concurrent release attempts for one tag SHALL be serialized.

#### Scenario: Pull request from another repository
- **WHEN** a fork submits a pull request
- **THEN** validation uses no publisher credentials and cannot publish packages

#### Scenario: Failed release checks
- **WHEN** tests or tag/public-metadata validation fails
- **THEN** that workflow performs no publication

#### Scenario: Existing execution guarantees
- **WHEN** release validation exercises concurrent ensure, stale worker callbacks, cross-owner access and all-process restart
- **THEN** existing identity, lease fencing, ownership and retained-result regression checks remain required

### Requirement: Versioned container distribution
A successful release SHALL publish Linux amd64 server and Spark example images
under the configured GitHub repository's GHCR namespace, with the release version
and source revision recorded. The server image SHALL contain the built browser
assets. The Spark example image SHALL contain its version-matched SDK/application
and connectors while preserving Spark's native entrypoint.

#### Scenario: Release image identity
- **WHEN** a valid version tag is released
- **THEN** both images identify the same source revision and release version

#### Scenario: Arbitrary version input
- **WHEN** a tag is malformed or does not match the intended project release
- **THEN** publishing fails before creating misleading image tags or artifacts

### Requirement: Complete Maven Central SDK release
Publication SHALL target the Central Publisher Portal under a verified namespace.
It SHALL include the parent POM and protocol/client/worker/Spark SDKs with matching
versions, required project/license/developer/SCM metadata, source and documentation
JARs, checksums and verifiable PGP signatures. Example applications SHALL be
excluded from the Central release.

#### Scenario: Consumer resolves SDKs
- **WHEN** a consumer resolves a published client or Spark SDK
- **THEN** its parent and Linha dependencies use resolvable matching coordinates and sources/documentation are attached

#### Scenario: Missing publisher configuration
- **WHEN** required public metadata, namespace configuration, token or signing key is missing
- **THEN** release setup fails with actionable missing-setting names and without logging secret values

### Requirement: Explicit release credentials and recovery
Publishing SHALL use scoped GitHub credentials for GHCR and configured Central
token/signing secrets only in release jobs. Documentation SHALL explain setup,
namespace verification, public-key distribution, immutable Central versions and
retry after partial cross-registry failure. Local release verification SHALL NOT
require real publishing credentials or perform public registry writes.

#### Scenario: One registry publication fails
- **WHEN** one registry accepts a release and another fails
- **THEN** completed publications remain explicit and operators can retry the failed job without claiming cross-registry atomicity

#### Scenario: Local release rehearsal
- **WHEN** a maintainer checks release artifacts with disposable signing credentials
- **THEN** completeness and signature checks run without uploading to GHCR or Central
