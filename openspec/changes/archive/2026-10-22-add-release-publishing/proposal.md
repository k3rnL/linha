## Why

Linha has local build and validation workflows but no repeatable public release
pipeline. Users need versioned server/worker example images in GHCR and JVM SDK
artifacts with sources, API documentation and signatures in Maven Central.

## What Changes

- Reuse branch/PR validation as a required gate for version-tag releases.
- Build and publish the server and Spark example images to the current repository's GHCR namespace.
- Add a Maven Central release profile for the parent and four SDK modules, excluding example applications from Central.
- Supply release versions, provenance, signatures, sources, Scala API documentation and required public metadata.
- Document verified namespace ownership, Central token/GPG secrets and the release/retry procedure.
- Verify release packaging locally without pushing images or publishing Maven artifacts.

## Capabilities

### New Capabilities

- `release-publishing`: Tested, versioned GHCR and Maven Central publication with explicit publisher configuration.

### Modified Capabilities

None. Execution, ownership, client leases, result durability and backend lifecycle
semantics are unchanged.

## Impact

GitHub Actions, Maven POMs, Docker build inputs, release helpers and documentation.
The Go job server, engine-neutral JVM client/worker SDKs and Spark-specific binding
retain their existing boundaries. Public repository/namespace/license/maintainer
metadata and publishing credentials require publisher configuration.
