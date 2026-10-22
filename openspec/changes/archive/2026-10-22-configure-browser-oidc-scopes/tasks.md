## 1. Server configuration

- [x] 1.1 Add browser scope configuration, defaults and active-setting validation; test JSON parsing, token validity and disabled settings.
- [x] 1.2 Use configured scopes in authorization requests and verify custom-scope group membership, non-member/missing-claim denial and existing callback security checks.

## 2. Deployment and documentation

- [x] 2.1 Add Helm values, JSON environment rendering and schema validation with default/custom/invalid/disabled rendering checks.
- [x] 2.2 Document identity provider group scope configuration, administrator claim mappings, client group assignments, environment syntax and fresh-login/deployment requirements.

- [x] 2.3 Reject misplaced `security.oidc.admin` in the chart schema; regression-test both invalid placement and rendered group mappings, and document the diagnostic.

- [x] 2.4 Add safe callback membership-denial diagnostics and document/test missing rules, missing/wrong/nonmatching claims, and credential/value redaction.

## 3. Verification

- [x] 3.1 Run PostgreSQL-backed cross-replica/recreated-service group login and retained-session tests, Go checks/race validation, Helm/formatting/OpenSpec checks, and a rebuilt server image smoke; record results and complete verified tasks.
