# Linha client SDK (Scala/JVM)

Planned module. Provides `LinhaContext`, typed entrypoints and result handles,
idempotent backend ensure, job submission, owner-scoped listing, status, cancellation,
and result retrieval. See the `typed-client-sdk` capability in the active OpenSpec
change. User-defined entrypoints are encoded as versioned data; executable code is
supplied in the worker image.
