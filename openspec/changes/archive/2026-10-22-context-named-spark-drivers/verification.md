# Verification

All four tasks are complete. The two added requirements and their four scenarios are covered by implementation and focused regression checks.

- `server/internal/kube/driver_names_test.go`: readable names, unsupported characters, lowercase conversion, empty fallback, truncation, full ID preservation, determinism and distinct instance IDs.
- `server/internal/engine/spark_application_test.go`: explicit driver name in generated SparkApplication submissions, simultaneous ensure from independent adapters, restart reuse, rejection of a different context reusing the same instance, unchanged existing application names and observation of both original and named drivers.
- `server/internal/auth/production_test.go`: successful named driver authorization and rejected missing/mismatched application ownership, stale owner UID, wrong role, Pod UID, service account, context or instance, retired incarnation, unconfigured child name, deleting resources and unavailable application. Existing driver authentication tests also pass.
- Focused naming, engine and authentication tests pass with the Go race detector. `make check` passes with disposable PostgreSQL, including database/controller tests and vet. This change does not require external object storage integration checks.
- Strict OpenSpec validation, documentation build/version checks, source/generated public-content checks and whitespace checks pass.
- The rebuilt static server image starts as its non-root runtime user, migrates disposable PostgreSQL and passes readiness/liveness checks. The runtime build still rejects the original incompatible dynamically linked binary.

No external Kubernetes rollout or live operator execution is claimed. The naming field is supported by the Kubeflow SparkApplication API; generated names are checked against Kubernetes DNS-label validation. Existing resources keep their names; newly provisioned instances receive the context prefix.
