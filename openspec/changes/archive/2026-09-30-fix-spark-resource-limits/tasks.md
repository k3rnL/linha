## 1. Resource enforcement

- [x] 1.1 Add regression tests for driver requests/limits and emitted executor settings across legacy/templates, default/custom resources and static/dynamic allocation; fix missing CPU limits.
- [x] 1.2 Document CPU enforcement, existing memory overhead and rollout behavior for running contexts.

## 2. Verification

- [x] 2.1 Run Go formatting/tests/vet and engine race checks; build a corrected local server image.
- [x] 2.2 Assert actual requests/limits on Spark driver and executor containers in a disposable cluster, including replacement after server restart; record evidence and clean up owned fixtures.
