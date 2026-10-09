# Verification: simplify-admin-issuer-mappings

Reviewed on 2026-10-09 before release v0.1.1.

| Dimension | Result |
| --- | --- |
| Completeness | 3/3 tasks complete; 1 requirements implemented |
| Correctness | 1/1 requirements reviewed; 6 scenarios matched to code and validation |
| Coherence | Implementation follows the recorded design and existing ownership/transport boundaries |

- `ValidateAdminConfig` accepts absent/matching issuers and rejects conflicts; role evaluation binds every identity to the configured issuer before matching membership.
- Authorization and configuration tests cover both viewer/operator roles and subject/claim rules, including cross-issuer and non-member denial.
- Helm schema and public examples no longer require per-rule issuers; matching legacy fields remain supported.
- The database-backed browser flow now explicitly removes a legacy rule issuer before recreating the callback service and validates the retained session.

Fresh authentication/configuration race checks and `make check` passed with PostgreSQL after adding the session-migration coverage. Release preparation contracts and chart formatting/rendering checks passed. The earlier local image smoke passed; hosted release validation will rebuild from the exact tag.

No critical issues or unresolved warnings. Ready for archive.

Release preflight additionally reproduced the hosted Helm 4.3.0 diagnostic-format difference and verified the corrected schema-error assertions with all 15 tests on both Helm 3.18.4 and Helm 4.3.0. Publishing had not started on the initial failed CI run.
