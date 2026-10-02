## 1. Release artifacts

- [x] 1.1 Add validated public release metadata and version/POM preparation without choosing unconfirmed publisher identity.
- [x] 1.2 Add release-only Central publishing, signatures and Scala API documentation for the parent and four SDK modules.
- [x] 1.3 Make server version/revision and Spark example jar inputs release-aware.

## 2. GitHub Actions

- [x] 2.1 Reuse the full existing validation suite for branches, PRs and the exact release commit with read-only permissions.
- [x] 2.2 Add version-tag GHCR and Central jobs with scoped credentials, pinned Actions, preflight checks and serialized tag releases.

## 3. Verification and setup

- [x] 3.1 Test malformed/mismatched tags, incomplete metadata, artifact selection, workflow syntax, local release packaging and disposable signatures without public writes.
- [x] 3.2 Run relevant formatting/build/checks and a restart smoke; document namespace/token/GPG setup, release commands and partial-failure recovery.
