## 1. Establish the reference inventory

- [x] 1.1 Inventory named/inline OpenAPI models, all nested backend/Spark settings, public Scala SDK type/member families, Helm values and server/storage settings; map each to its canonical page/anchor and evidence source, with explicit dynamic/internal exclusions.
- [x] 1.2 Audit baseline defaults, requiredness and bounds against Go validators/normalizers and Scala declarations; record discrepancies for explicit documentation treatment without changing runtime APIs.

## 2. Complete backend and Spark configuration reference

- [x] 2.1 Add the backend category/index and complete JSON references for ensure input, backend/engine specs, context versions/client leases, result policies/path templates and workload-storage declarations, with linked nested types.
- [x] 2.2 Document every Spark application, driver, executor and UI field, including omission versus zero/false, computed defaults, bounds/units, dynamic-allocation conditions, resource interactions and deprecated aliases.
- [x] 2.3 Document Pod fragment boundaries, merge rules, protected fields, size/image/Secret constraints and compatible upstream definitions; link directly from backend/Spark models.
- [x] 2.4 Enable and validate content tabs in the pinned documentation stack; add equivalent JSON/Scala backend examples using existing SDK APIs and shared field definitions.

## 3. Complete request, result and protocol models

- [x] 3.1 Add consistent references for submit/retry/deadline, job states, progress/failure, attempts and pagination, with field definitions, state/ownership semantics and minimal JSON examples.
- [x] 3.2 Document result descriptors, status-only output, file/dataset models, publication/retention and application-storage sessions/objects/locks; distinguish request artifacts from independently managed application files.
- [x] 3.3 Complete the HTTP model catalog for public, managed-worker and administrator surfaces, including route/header placement, attempt fencing, error envelopes and batch actions; index every public schema or document its extension boundary.

## 4. Complete the Scala/JVM SDK reference

- [x] 4.1 Document client/context/handle APIs and public credential types/options with signatures, argument defaults, return types, errors and lifecycle behavior; link relevant JSON models.
- [x] 4.2 Document entrypoints/codecs, result types/writers, worker/job-context APIs and Spark runtime/helpers, including registration, cleanup, cancellation and supported result descriptors.
- [x] 4.3 Document PodTemplate, result-path/storage helpers, application StorageSession and Hadoop APIs; verify all inventoried public SDK families are reachable by exact name and label Json-based configuration accurately.

## 5. Separate operator configuration

- [x] 5.1 Complete the operator reference for Helm values, standalone environment and S3 destination settings, with defaults/constraints and links between equivalent surfaces; distinguish deployment policy from caller backend settings.

## 6. Make lookup and existing links work

- [x] 6.1 Add the reference landing page, categorized MkDocs navigation and global model/type index; update home/tutorial/how-to links to specific canonical references and keep procedures in the appropriate category.
- [x] 6.2 Preserve old configuration/SDK/HTTP URLs and useful anchors as concise linked entry points, retain version-bearing examples used by release rendering, and keep published snapshots unchanged.

## 7. Verify completeness and maintainability

- [x] 7.1 Add a schema/inventory documentation coverage check with negative fixtures for missing model/nested-field coverage and missing page/anchor targets; document exclusions and the limits of structural checks for prose and SDK signatures.
- [x] 7.2 Integrate coverage into documentation validation/CI and contributor guidance; run the coverage fixtures, strict MkDocs build, existing frozen-version suite, public-content checks and strict OpenSpec validation, recording actual outcomes.
- [x] 7.3 Review representative lookup tasks from the home page and model index: application JAR/arguments, driver memory/overhead/concurrency, executor retention, Pod environment, result retention, storage destinations, retries/deadlines, restored handles and admin batch models. Each answer must be reachable without reading a tutorial or source file.
- [x] 7.4 Cross-check documented restart and HA claims against the existing restart/failure-injection suites and recorded evidence; retain their CI gates, verify no runtime/API/schema changes, and report any unrun integration checks accurately.
