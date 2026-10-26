## Why

Linha releases attach the Helm archive to GitHub but do not publish an OCI chart.
Operators need a public chart reference usable from Helm and deployment tools
without a source checkout or registry credentials.

## What Changes

- Publish versioned charts at `oci://ghcr.io/k3rnl/charts/linha`, following the
  existing public registry layout used by other projects.
- Add a chart-only manual recovery path for an existing published release, using
  its original archive and checksum without republishing SDKs or images.
- Validate chart identity/version and verify an anonymous OCI pull after upload.
- Document installation, package visibility and retry procedures.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `release-publishing`: distribute Helm charts through OCI alongside images and
  SDKs, including recovery and verification of public access.

## Impact

Release workflow, chart preparation checks, installation/release documentation,
and one public GHCR package. No server, SDK, database, job, lease, retry or result
semantics change. GHCR package visibility is an external package-owner setting.
