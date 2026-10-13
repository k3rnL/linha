# Protocol identity and conformance

The request identity is a handler name, positive handler version and JSON payload.
Typed codecs serialize parameters into that envelope. Workers execute only
registered classes already installed in the image.

## Context version encoding

The server normalizes defaults, resources, durations, result destination identity,
merged Pod templates and the resolved main-image digest. It recursively sorts JSON
object keys, preserves array order, encodes with Go JSON escaping and hashes the
UTF-8 bytes with SHA-256. Clients do not compute this version themselves.

This is Linha's documented encoding; it is not a claim of RFC 8785 numeric
canonicalization. Submission replay uses PostgreSQL JSONB equality and the
accepted result policy. Public request IDs are opaque identifiers, not hashes.

## Result identity

Results carry `kind`, `schema` and `version`. A typed restored handle rejects a
descriptor mismatch before decoding. Files and parts include size and SHA-256.
Dataset pages are ordered by relative path with opaque cursors; callers use those
returned paths rather than deriving object keys.

Path templates expand the original acceptance time in UTC and require separate
request and attempt identity segments. Persisted allocations make lost-response
replay safe and reject overlapping or stale-attempt output.

## Executable contract fixtures

The repository's [API fixtures](https://github.com/k3rnL/linha/tree/main/api/fixtures)
cover raw/typed equivalence, Unicode escaping, configuration hashing, Pod merging,
path/date expansion, descriptors and cross-language limits. Go/JVM suites consume
these fixtures; PostgreSQL tests exercise actual idempotency and allocation state.
