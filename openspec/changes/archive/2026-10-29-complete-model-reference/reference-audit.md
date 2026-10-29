# Reference source audit

The inventory in `scripts/fixtures/reference-coverage.json` records canonical
targets and selected implementation evidence. This audit records findings that
needed more than OpenAPI table transcription. Runtime code and schemas are
unchanged by this documentation work.

| Area | Evidence | Reference treatment |
| --- | --- | --- |
| Result size | `httpapi.Server.normalize` enforces 5 GiB; OpenAPI ResultPolicy.maxBytes currently advertises 1 TiB | State the enforced 5 GiB bound explicitly; do not expand runtime policy or silently change schema |
| Result and retry zero values | Result normalization and Store.Submit replace zero with defaults | Document zero/omission together for those fields |
| Spark numeric defaults | ParseSpark seeds defaults before JSON decoding | Document valid zero minimums separately from invalid zero maximums/concurrency |
| Resource aliases/defaults | normalizeResources rejects nonzero memoryMi plus memory, normalizes CPU strings and computes overhead | Include alias conflicts, heap/container distinction and computed defaults |
| Executor timeouts/UI | normalizeRuntimeSettings validates even when disabled, normalizes equivalent duration/default values | State both validation and conditional application |
| Managed application | ParseSpark permits internal legacy forms, but Kubernetes managed startup requires application | State the managed deployment requirement, not legacy parser permissiveness |
| Application storage | domain workload structs and HTTP routes have models absent from the named OpenAPI catalog | Document StorageSessionInput/StorageSession/StorageObject and routes; check selected Go tags |
| Captured fingerprints | WorkloadStoragePolicy.Fingerprints is server-computed and not listed in BackendSpec.storage OpenAPI properties | Document server output separately from caller destinations |
| Attempt history | Domain Attempt is returned in an items envelope without a named OpenAPI schema | Add a selected Go-backed model reference |
| Dataset pagination | Store.DatasetParts permits 1–200 items, HTTP default 100 | Document the actual bound and preserve cursor paths verbatim |
| Typed result size | JobHandle.result reads JSON at most 64 MiB, independent of larger server policy | Link to streaming download for larger output |
| Client lifecycle | LinhaClient shares attachments, renews leases and restores handles without attachment | Separate handle restoration from restoreContext, which acquires demand |
| Browser lifetime | Refresh extends provider authentication within an absolute configured lifetime | Replace the old configuration summary's unconditional token-expiry cap |
| Chart coverage | values.schema.json omits some top-level native/default settings | Inventory the union of declared schema paths and values.yaml paths |
| Pod fragments | Native Kubernetes v0.33.1 PodSpec with explicit Linha/Spark restrictions | Describe merge/protected fields and link versioned upstream types instead of claiming arbitrary SparkApplication support |

Public Scala constructors, factory defaults and method families were compared with
protocol, client, worker, Spark and Hadoop source. Package-private diagnostics,
staging and distributed-output implementation helpers are excluded from the
public SDK index. Generated case-class product/collection methods are not new
Linha APIs.

Source review covers acceptance/ownership/idempotency in Store, attempt fencing,
completion/recovery, client/controller lifetime, local/S3 result validation, and
storage-session authority. The documentation preserves their current boundaries.
