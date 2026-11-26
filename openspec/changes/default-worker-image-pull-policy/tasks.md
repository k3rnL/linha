# Tasks

## 1. Helm configuration

- [x] 1.1 Add `workerPodPullPolicy: Always` to Helm values and schema, and pass it to the server as `LINHA_WORKER_POD_PULL_POLICY`; verify Helm rejects invalid values and renders the setting.
- [x] 1.2 Add chart tests for the default and override, and document the setting in the Helm and environment references; verify with `python3 scripts/test-helm.py` and the reference checks.

## 2. Runtime policy application

- [x] 2.1 Parse and validate the shared policy, resolve template overrides consistently for both roles, and persist the resolved value; verify precedence and conflict errors with focused engine tests.
- [x] 2.2 Set the shared Spark configuration and `SparkApplication.spec.imagePullPolicy`, and assert context driver/executor behavior; verify with targeted Go engine tests and document the shared-policy limitation.

## 3. Integration checks

- [x] 3.1 Verify the completed change with `openspec validate default-worker-image-pull-policy`, `python3 scripts/test-helm.py`, targeted Go engine tests, `make check`, and `make docs-check`.
