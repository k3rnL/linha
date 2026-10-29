## Why

Developers can follow Linha's guides but cannot reliably look up every supported
field when declaring a backend or building a Spark worker. The reference mixes
deployment, backend and request settings, summarizes SDK types, and sends readers
to source files or OpenAPI to discover missing details.

## What Changes

- Organize Reference into backend configuration, requests and results, JVM SDKs,
  HTTP protocol, and server deployment, with an index of public models and types.
- Document every Linha-owned backend field and nested Spark structure, including
  requiredness, type, actual omission defaults, bounds, units, allowed values,
  interactions and a small example. Distinguish caller inputs from server outputs.
- Use JSON as the canonical model reference, with language tabs for equivalent
  usage examples (JSON and the existing Scala/JVM SDK today). Keep field
  definitions shared; organize language-specific SDK APIs by language instead of
  maintaining a separate copy of every model reference for each language.
- Extend the same consistent lookup format to public request/result models and
  supported client/worker SDK types and methods; link JSON models to their Scala
  APIs where a counterpart exists.
- Document the boundaries of Kubernetes Pod fragments and arbitrary application
  payloads without pretending Linha accepts every Spark or SparkApplication option.
- Give tutorials and guides direct links to the relevant model/field references,
  retain existing documentation URLs and useful anchors, and preserve published
  version snapshots.
- Add a maintainable coverage inventory and checks so newly added public models
  and configuration fields cannot silently disappear from the reference.

## Capabilities

### New Capabilities

None. This change documents existing public behavior.

### Modified Capabilities

None. Runtime contracts, supported fields and SDK APIs do not change. The change
uses `skip_specs: true`; implementation tasks and review criteria describe the
documentation work without introducing artificial runtime requirements.

## Impact

Development documentation in `docs/`, MkDocs navigation, documentation validation
scripts and their CI integration. API schemas, Go validators, Helm values/schema
and Scala source are evidence for the reference, not specifications to expand as
part of this work. No server, SDK, database, chart runtime or external publication
change is required.
