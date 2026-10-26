## 1. Driver naming and authentication

- [x] 1.1 Implement bounded context-readable Kubernetes driver names and supply them in new SparkApplication submissions.
- [x] 1.2 Accept named drivers through the existing role, owner, service-account and incarnation fences; retain existing driver support.

## 2. Verification and delivery

- [x] 2.1 Verify normalization, distinct instance names, repeated ensure, named/existing driver observation and rejected worker identity mismatches.
- [x] 2.2 Document naming and upgrade behavior, run required Go and spec checks, then rebuild and smoke-test the local server image.
