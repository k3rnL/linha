## Context

See proposal.md for the lookup problem. The site already separates tutorials,
how-to guides, reference and explanation, but Reference contains a few broad
summary pages. `configuration.md` combines Helm, Spark, results and environment
variables; `sdk-reference.md` summarizes APIs without documenting every public
type and member. `application-storage.md` mixes reference and procedures.

The user's preferred reference is generic JSON, with language-specific usage
examples where useful. Backend declarations are currently Circe `Json` in the
Scala client; a Go server type is not necessarily a typed Scala SDK model.

OpenAPI describes public models, including inline Spark settings. Actual Go
normalizers/validators define omission behavior and cross-field constraints.
Scala declarations define SDK signatures, and Helm values/schema plus server
configuration define deployment settings. Existing frozen site versions must
remain untouched. This is a documentation-only change with specs skipped.

## Goals / Non-Goals

**Goals:** A developer can locate a model by its public name or purpose, follow
nested types, and determine supported fields without reverse-engineering a guide
or source file. Backend and Spark worker configuration receive the first complete
increment. JSON is the shared definition; SDK usage has language-specific homes.

**Non-Goals:** Add backend fields, introduce typed backend builders, redesign the
SDKs, publish another language SDK, duplicate the Kubernetes/Spark manuals, change
runtime semantics, or replace the existing documentation hosting/versioning tools.

## Decisions

### 1. Use a reference landing page and five categories

Create a reference index with links by task vocabulary and exact model/type name.
Group pages by the product surface instead of putting all settings in one table:

| Category | Contents |
| --- | --- |
| Backend configuration | Ensure input, backend spec, engine spec, Spark application, driver pool/resources, executors, UI, Pod fragments, result policy/path templates, workload storage |
| Requests and results | Submission, retry/deadline, job states, progress/failures, attempts, descriptors, files, datasets, pagination |
| SDKs → Scala/JVM | Dependencies; client/context/handles; entrypoints/codecs; worker/job context; Spark helpers; credentials; storage/Hadoop |
| HTTP protocol | Public, managed worker and administrator routes; model catalog; authentication, headers and error envelopes |
| Server deployment | Helm values, server environment, S3 destination configuration and operator-only policy |

Use a page per coherent model family, with an anchor for each named model and
nested structure. A separate page for every small receipt would add navigation
noise. Global and category indexes link directly to model anchors; exact public
names and field paths are included in searchable text. Advanced managed-worker
and admin models live under HTTP, so everyday backend lookup stays focused.

### 2. Give every model and field a consistent definition

Start each model with purpose, where it appears, input/output direction, JSON
shape, and links to its parent and nested types. Each field has its exact name or
path, type, requiredness, default or omission behavior, allowed values/bounds,
units, meaning, and relevant interactions. Use "not applicable" or "no default"
where appropriate instead of inventing a value for response fields.

Document omission separately from explicit zero, false, empty and null whenever
the implementation treats them differently. Include computed defaults,
conditional requiredness, aliases/deprecations, server-assigned fields and
validation conflicts. For example, distinguish heap from container memory, driver
count from request concurrency, and application file storage from retained results.

The backend reference covers every nested Linha-owned setting. For Kubernetes
fragments, document accepted metadata/spec boundaries, merge identities, protected
fields, size limits and image/Secret policy, then link to the compatible upstream
PodSpec reference. Arbitrary handler payloads and dynamic map keys have explicit
extension boundaries. Do not imply a generic `sparkConf` or arbitrary
SparkApplication passthrough exists.

SDK entries include public constructors, method signatures, argument defaults,
return types, exceptions, lifecycle/close behavior and links to related JSON
contracts. Keep engine-independent APIs separate from Spark helpers. List Scala
types only when they actually exist; label raw JSON APIs explicitly.

### 3. Use tabs for equivalent examples, with shared field definitions

Add the content-tabs extension supported by the pinned Material/MkDocs stack and
use JSON/Scala tabs where both demonstrate the same operation. JSON defines the
backend/model shape; Scala shows the real SDK call and its Json argument. Keep
examples complete enough to identify required imports and surrounding setup.

Do not duplicate field tables by language or fabricate Java/Go/Python APIs. Pure
JSON model snippets, YAML deployment settings and Scala-only APIs can use ordinary
code blocks. A future SDK can add its own SDK category and example tab while
reusing the JSON contract. This avoids maintaining parallel model definitions.

### 4. Audit coverage against public contracts and actual implementation

Build a checked-in inventory mapping public schemas, nested field paths,
deployment settings and SDK type/member families to documentation anchors and
their evidence sources. Traverse named and inline OpenAPI properties and Helm
schema/values; resolve references and array item objects without expanding
arbitrary map keys or upstream PodSpec into an invented finite list.

Review the baseline against Go model tags, backend/Spark normalization and
validation, storage policies, HTTP handling and public Scala declarations.
OpenAPI alone does not prove defaults or SDK lifecycle behavior. Record
source/schema discrepancies for explicit treatment; do not silently alter runtime
code or assert an unverified contract to make a table look complete.

Add a lightweight documentation coverage check: derived model/field inventories
must have reference targets, targets must exist, and exclusions must explain
dynamic or internal boundaries. Cover both newly added undocumented schema fields
and broken targets with negative fixtures. Keep SDK families in the inventory and
review public signatures against source as part of maintenance. The automated
structural check does not prove prose accuracy or discover arbitrary Scala API
changes; state those limits in the contributor guidance.

Prefer this combination of schema checks and curated descriptions to generated
tables alone: readers need interactions and lifecycle semantics that schemas do
not fully encode. Avoid introducing a new documentation generator/framework.

### 5. Preserve links, page roles and version behavior

Keep `configuration.md`, `sdk-reference.md`, `http-api.md` and existing useful
anchors as concise entry/compatibility pages linking to canonical sections.
Preserve version-bearing SDK coordinates used by documentation builds. Move
step-by-step application-storage content into the how-to category and link to its
new model reference. Tutorials keep their working sequence and link directly to
the relevant fields instead of accumulating larger tables.

Update the home-page lookup links, MkDocs navigation and contributor guidance.
Ship changes to development docs through the existing publication workflow;
previously published snapshots remain frozen. Publishing or rewriting historical
site content is not an implementation step for this change.

### 6. Keep durable execution and identity semantics explicit

Reference descriptions retain the existing contracts: acceptance returns after
the database commit; ownership comes from authenticated identity; retries are
opt-in and bounded; worker updates require the live fenced attempt; results are
public only after verified successful publication; persisted IDs and metadata
survive restarts, subject to database and storage durability. User side effects
are not exactly-once. Client context leases, worker attempt leases, application
OAuth credentials and browser/admin identities remain distinct.

## Risks / Trade-offs

- More pages can make discovery harder → use a short category tree, model index,
  stable field anchors and direct guide links; review realistic lookup tasks.
- Copied defaults can drift → tie the coverage inventory to source evidence,
  check schema fields structurally and review omission semantics manually.
- Language examples can diverge → share model definitions and use equivalent
  examples only through existing supported APIs.
- Reorganization can break bookmarks or frozen-version checks → retain old
  entry points/anchors and run strict builds plus the existing snapshot suite.
- Exhaustive upstream APIs would overwhelm the reference → document Linha's
  accepted boundaries and link to version-compatible upstream definitions.

## Migration Plan

1. Inventory existing public surfaces and establish canonical reference targets.
2. Add backend/Spark references and JSON/Scala example tabs, then cover remaining
   model families, SDK APIs and operator configuration.
3. Add indexes, navigation, old-anchor compatibility and focused guide links.
4. Run coverage checks, strict builds, frozen-version checks and public-content
   checks. Review lookup tasks from the home page and by exact model name.
5. Publish development docs through the normal reviewed workflow. Rollback is a
   documentation commit revert; no database or runtime migration is involved.
