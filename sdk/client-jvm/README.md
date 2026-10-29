# Linha client SDK

The Scala/JVM client submits JSON or typed entrypoints, attaches to managed
contexts, renews client leases and restores job handles by durable ID. Business
code is installed in the worker image.

Use OAuth 2.0 client credentials to obtain and replace API access tokens. Mount
only the OAuth client secret; no pre-generated JWT file is required.

- [Client and worker tutorial](../../docs/walkthrough.md)
- [OAuth configuration](../../docs/client-authentication.md)
- [SDK reference](../../docs/sdk-reference.md)

The current source artifacts are `com.k3rnl:linha-client_2.12:0.1.4-SNAPSHOT`.
Install with Java 17 `mvn -B install` from the repository root; matching sources
are installed too. Published applications can use version `0.1.4` from Maven Central.
