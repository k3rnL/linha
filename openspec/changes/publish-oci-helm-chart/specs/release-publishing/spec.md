## ADDED Requirements

### Requirement: Public OCI Helm chart distribution
Successful version-tag releases SHALL publish the Linha Helm chart under the
GitHub owner's `charts/linha` GHCR package after the existing release checks pass.
The chart name, chart version, application version and default server image tag
SHALL match the intended release. Publication SHALL verify an anonymous pull of
the uploaded archive and SHALL report an actionable visibility error when the
package is private. Publisher credentials SHALL NOT be needed by consumers.

#### Scenario: Install without a source checkout
- **WHEN** an operator pulls the public chart with its release version
- **THEN** Helm retrieves the version-matched archive without registry credentials

#### Scenario: Release validation fails
- **WHEN** tag or release validation fails
- **THEN** that workflow does not publish a chart

#### Scenario: Private package
- **WHEN** registry upload succeeds but anonymous pull is denied
- **THEN** the job reports that package visibility must be public and does not claim public distribution succeeded

### Requirement: Independent chart publication recovery
A maintainer SHALL be able to publish the chart of an existing published GitHub
release without republishing SDKs or images. Recovery SHALL use the original
release archive, validate its SHA-256 checksum and identity/version, and reject
malformed version input, draft releases and mismatched archives before upload.
Concurrent chart publication for the same version SHALL be serialized.

#### Scenario: Backfill an existing release
- **WHEN** a maintainer selects an existing published release with a valid chart and checksum
- **THEN** only that original chart archive is uploaded to OCI

#### Scenario: Wrong or corrupted archive
- **WHEN** the archive checksum, chart name, chart version, application version or default image tag does not match the requested release
- **THEN** publication fails before registry writes

#### Scenario: Runtime contracts remain unchanged
- **WHEN** operators deploy the distributed chart
- **THEN** request acknowledgement, ownership, lease fencing, retry, results and restart guarantees retain their existing contracts
