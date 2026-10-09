## 1. Server configuration and scoped transport

- [x] 1.1 Parse the optional OIDC TLS boolean with strict defaults and disabled-provider overrides; test those combinations.
- [x] 1.2 Wire an isolated OIDC HTTP transport into API verification and browser discovery/JWKS/token exchange, with a startup warning and no global TLS changes.

## 2. Deployment and operator guidance

- [x] 2.1 Add Helm values, JSON schema, Deployment environment wiring, and rendering/type validation tests.
- [x] 2.2 Document Helm/environment usage, release requirements, browser scope, and mounting a trusted CA.

## 3. Verification

- [x] 3.1 Test self-signed HTTPS discovery, delayed JWKS retrieval, certificate hostname behavior, strict unrelated clients, and continued token validation/owner isolation.
- [x] 3.2 Exercise cross-replica browser login over HTTPS, including token exchange, nonce/role/signature rejection and recreated server instances; run the durable independent-database variant with a disposable database.
- [x] 3.3 Run make check, race checks for authentication/configuration, chart tests, formatting checks, and strict OpenSpec validation; record verification evidence.

## Verification evidence (2026-10-09)

- `go test -race -count=1 ./server/internal/auth ./server/internal/config` passed with `LINHA_TEST_DATABASE` pointing to a disposable PostgreSQL 17 container. Both plain-HTTP legacy mappings and HTTPS inherited mappings ran with independent database pools; callback and session reuse after service recreation passed. The container was removed afterward.
- `make check` passed with the same database: all Go packages, enabled PostgreSQL integration scenarios, formatting and `go vet`.
- `uv run --no-project --with pyyaml python scripts/test-helm.py` passed all 15 tests, including TLS defaults/types/switches and inherited or legacy explicit issuers for both roles and rule types.
- Data/YAML/JSON formatting, Black formatting and `git diff --check` passed.
- `openspec validate --all --strict` passed all 25 current specs/changes.
- Built the static server binary and local image `linha-server:oidc-config` (`sha256:89cf8f4526714cc7315402cde1d42baad7aa6884bffc4f8a40ff1b1e1971a4ff`). Container version smoke test passed; the built image rejects malformed OIDC TLS boolean settings. No published release was changed.
