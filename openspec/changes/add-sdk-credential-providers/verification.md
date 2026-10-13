# Verification

Verified locally on 2026-10-13. Built-in application credential acquisition uses OAuth 2.0 client credentials. No identity-provider-specific client acquisition is retained.

## SDK and identity provider

- Java 17 full JVM reactor install passed: 25 ordinary tests executed; the conditional integration test was subsequently executed successfully with its real fixture.
- Disposable Keycloak 26.5.7 verified client-credentials issuance and renewal, audience linha, RSA JWT signatures, stable subject across replica/provider recreation and distinct subjects for independent applications.
- Protocol/TLS tests cover shared acquisition under 32 concurrent callers, expiry and outage recovery, client-secret rotation, bounded response/deadline, no redirects, secure defaults, trusted CA, hostname verification, isolated verification bypass and secret-safe errors.
- Signed release dry-run verified seventeen artifacts, signatures, checksums, sources and generated API docs against a loopback test receiver. Installed SDK source JARs passed the public-content guard. SDK binaries and source JARs installed locally under com.k3rnl version 0.1.2-SNAPSHOT. This development feature has not been published to Maven Central.
- Typed tutorial business, worker and client snippets compiled against the actual SDK and Spark dependencies.

## Server, deployment and recovery

- make check passed with disposable PostgreSQL, including integration tests and vet. Auth/config race checks passed.
- Signed JWT tests retain issuer/audience/expiry/subject validation, ten application identities with one audience, cross-owner separation and browser administrator isolation.
- Nineteen Linha Helm tests passed under Helm 3.18.4 and Helm 4.3.0; schema diagnostics accept dotted and JSON-pointer forms. Frontend types, formatting, production build and eleven browser tests passed.
- Forty PromQL expressions, seventeen real admin OpenAPI response contracts, retained-history metrics checks and OpenAPI validation passed.
- Real process-restart smoke tests passed for immediate durable IDs, typed/raw requests, files, retained results and nested/suppressed failure traces.
- Local server image linha-server:oauth-only reports Linha dev-oauth. No live cluster deployment performed.

## Scope limits

API JWT and browser OIDC settings are deliberately independent. Matching new chart/server/SDK builds are required; older deployed applications that read client JWT files must be rebuilt. Internal Spark worker projected credentials remain unchanged. No owner migration, legacy configuration fallback or immutable artifact replacement is implemented. Disposable identity/database fixtures were removed after checks.
