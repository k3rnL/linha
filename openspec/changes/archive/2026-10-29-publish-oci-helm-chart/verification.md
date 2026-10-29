# Verification

- Local release/chart tests, all 19 Helm checks, actionlint, formatting, strict
  documentation/frozen-version checks, OpenSpec and source/history public-content
  checks passed. Source preparation and original release archive validation passed.
- Chart-only workflow: https://github.com/k3rnL/linha/actions/runs/40363000688
  Chart job succeeded; preparation, full runtime checks, images and SDK publication
  were skipped as intended for a published-release backfill.
- An additional local pull used empty Helm and Docker credential files and rendered
  the chart successfully. Pulled archive bytes equal the original 0.1.3 release.
- Reference: `oci://ghcr.io/k3rnl/charts/linha`, version `0.1.3`.
- OCI digest: `sha256:94b50b93a32c46d77db0c438852ea1023874179ff11e24f326c4631c2e6f48ad`.
- Original archive SHA-256: `5798c4852e58b840b2c49f7fb83628b7222c70a7f88ec9ceabd8f5fdd045e6e2`.
- Review: https://github.com/k3rnL/linha/pull/2
