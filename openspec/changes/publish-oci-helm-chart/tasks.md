## 1. Chart publication

- [x] 1.1 Add chart preparation and release-archive validation with negative-case checks for corrupt or mismatched inputs.
- [x] 1.2 Add gated OCI publication and chart-only recovery to the release workflow, with isolated anonymous pull verification.

## 2. Documentation and validation

- [x] 2.1 Document OCI install/pull, package visibility and independent chart retry.
- [x] 2.2 Validate release/chart checks, workflow syntax, documentation, public content and the OpenSpec change. Runtime contracts are unchanged; retain existing restart/HA CI gates.

## 3. Publish the existing release

- [ ] 3.1 Push the reviewed workflow change and run chart-only publication for the original 0.1.3 release archive.
- [ ] 3.2 Verify public package visibility and a credential-free pull/template with the same archive checksum.
