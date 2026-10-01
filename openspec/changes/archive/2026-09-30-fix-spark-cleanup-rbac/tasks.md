## 1. Correction

- [x] 1.1 Add a worker-only deletecollection grant for Pods/ConfigMaps and render regression checks covering authentication/RBAC modes and denied resource types.
- [x] 1.2 Document the Helm-only upgrade and verification commands.

## 2. Verification

- [x] 2.1 Pass Helm rendering/schema/lint and strict OpenSpec checks.
- [x] 2.2 Verify old-role denial, corrected collection cleanup and namespace/server/resource boundaries in a disposable cluster; retain evidence and remove the fixture.
