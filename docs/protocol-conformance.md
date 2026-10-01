# Protocol identity and conformance

The base request is handler, positive version and JSON payload. Scala codecs
serialize arguments into that envelope; workers reconstruct only registered
classes already in the image. No closure or executable bytecode is transmitted.

`api/fixtures/canonical-protocol.json` is exercised by the Go domain and JVM
protocol suites. It covers equivalent raw/typed request envelopes, descriptors,
Unicode/HTML-sensitive strings, ordered object serialization, pinned image
identity and persisted local/S3 policies. PostgreSQL tests establish actual
logical-name/idempotency-key conflict and replay behavior.

The server is authoritative for backend versions: it normalizes engine defaults,
resource settings, durations, resolved image digest, result destination identity
and effective templates, encodes recursively sorted JSON object keys using Go's
JSON string escaping, then hashes those UTF-8 bytes with SHA-256. Arrays retain
order. This is the documented Linha encoding, not a claim of RFC 8785/JCS numeric
normalization. Clients need not compute the version themselves. Request replay
uses PostgreSQL JSONB equality and the accepted result policy; request IDs are
opaque generated identifiers, not content hashes.

`api/fixtures/path-templates.json` covers UTC date expansion across +14/-12 hour
zones, leap days and millisecond fields. Go/JVM validators also reject missing
identity segments, traversal (including encoded forms), reserved names and invalid
formats. PostgreSQL allocations preserve the original submitted timestamp across
retry, persist paths before returning them, replay lost responses, and reject
conflicting names/overlaps and stale attempts.

Results carry kind/schema/version descriptors. A restored typed handle rejects a
mismatch before decoding. Files and dataset parts are streamed, with their SHA-256
and byte count recorded. Dataset pages are ordered by relative part path with an
opaque next cursor; clients must use returned paths rather than constructing S3
keys. All public retrieval is authorized by owner, independent of engine lifetime.
