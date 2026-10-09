## 1. Issuer inheritance

- [x] 1.1 Accept missing subject/claim issuers using the configured issuer, preserve compatible explicit values and reject conflicting issuers; test authorization and config parsing.
- [x] 1.2 Remove repeated issuer requirements from the chart and deployment examples; test both role/rule types through Helm rendering.

## 2. Verification

- [x] 2.1 Verify inherited mappings during HTTPS browser login across replicas and after recreation with independent PostgreSQL pools; run Go/chart checks and strict OpenSpec validation, and record evidence.

## Verification evidence (2026-10-09)

- `go test -race -count=1 ./server/internal/auth ./server/internal/config` passed with `LINHA_TEST_DATABASE` pointing to a disposable PostgreSQL 17 container. Both plain-HTTP legacy mappings and HTTPS inherited mappings ran with independent database pools; callback and session reuse after service recreation passed. The container was removed afterward.
- `make check` passed with the same database: all Go packages, enabled PostgreSQL integration scenarios, formatting and `go vet`.
- `uv run --no-project --with pyyaml python scripts/test-helm.py` passed all 15 tests, including TLS defaults/types/switches and inherited or legacy explicit issuers for both roles and rule types.
- Data/YAML/JSON formatting, Black formatting and `git diff --check` passed.
- `openspec validate --all --strict` passed all 25 current specs/changes.
- Built the static server binary and local image `linha-server:oidc-config` (`sha256:89cf8f4526714cc7315402cde1d42baad7aa6884bffc4f8a40ff1b1e1971a4ff`). Container version smoke test passed; the built image rejects malformed OIDC TLS boolean settings. No published release was changed.
