## 1. Template contract and persistence

- [x] 1.1 Add native template decoding, deterministic merging, size limits and managed-field/image/reference validation with adversarial tests.
- [x] 1.2 Persist requested and effective backend specs through an additive migration; test repeat/concurrent ensure, changed defaults, restart and legacy contexts with PostgreSQL.

## 2. Kubernetes and registry integration

- [x] 2.1 Apply persisted driver templates and render immutable executor ConfigMaps for Spark; test idempotency, conflicts and replacement without live client/default dependencies.
- [x] 2.2 Resolve private worker images using permitted namespaced registry Secrets; verify authentication, rotation, rejection and no credential persistence or disclosure.
- [x] 2.3 Expose Helm template defaults and scoped Secret/ConfigMap permissions; verify namespace-only chart rendering and configuration validation.

## 3. SDK and verification

- [x] 3.1 Add Scala JSON/YAML template loading and immutable helpers, with shared raw-protocol fixtures and tests.
- [x] 3.2 Document the JSON/Scala contracts, managed fields, defaults, registry configuration and migration; update OpenAPI and examples.
- [x] 3.3 Run Go/PostgreSQL race checks, JVM tests, Helm/OpenAPI/OpenSpec checks, restart smoke test and real Spark template propagation/replacement verification.
