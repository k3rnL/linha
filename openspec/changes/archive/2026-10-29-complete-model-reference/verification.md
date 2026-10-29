# Verification

All 19 implementation tasks are complete. This change updates development
documentation and its checks; runtime source, SDK source, OpenAPI, database,
deployment values and existing runtime CI gates are unchanged.

## Automated checks

- Strict MkDocs build passed with categorized navigation, explicit model/field
  anchors and JSON/Scala content tabs.
- Reference coverage passed for all 63 named OpenAPI models, nested inline fields,
  selected Go wire/configuration structures, the union of chart schema/default
  paths, 46 server environment entries, 48 SDK families and 16 legacy anchors.
- All 14 coverage fixtures passed, including missing models/nested/array fields,
  broken pages/anchors, empty field definitions, chart-only schema/default fields,
  embedded Go structures, environment additions and missing SDK entries.
- The existing frozen-version suite passed: development updates preserve frozen
  release HTML, release ordering/aliases, version metadata and immutable source
  identity; local logs/JSON remain excluded.
- Public-content source/render checks and all six public-content fixtures passed.
- Pinned Python formatting, data formatting, actionlint and whitespace checks passed.
- Strict OpenSpec validation passed for all 32 active change/spec items.
- All ten new JSON examples parsed; the JSON and Scala backend tabs declare
  exactly the same backend configuration/name.
- Driver JVM options were traced to SparkApplication.spec.driver.javaOptions; the
  reference includes a searchable -D/Log4j example and container-file requirement.
- The Spark reference links to the Operator v2.4.0 SparkApplication API, upstream
  writing guide and Java options section; the upstream links were verified.
- All four standalone Scala reference examples compiled with Scala 2.12.20,
  pinned cached dependencies and existing local SDK classes. SDK binaries were
  not rebuilt as part of this check.

## Lookup and browser review

Chromium verified 11 model/SDK lookups from the reference entry/index: image-local
JAR, main arguments, driver overhead/concurrency, executor retention, Pod fields,
result retention, workload destinations, request deadline, admin batch selection
and restored typed handles. Answers were available without reading tutorials or
source. Model names now link directly to their canonical sections.

JSON/Scala tab switching worked on desktop and a 390-pixel mobile viewport.
Five representative legacy configuration/SDK/HTTP URLs and anchors worked in the
browser; structural checks cover all 16 preserved entry anchors. The rendered
desktop backend layout was visually inspected.

## Runtime scope and limits

Restart/ownership/fencing descriptions were cross-checked against Store acceptance,
completion/recovery and controller lifetime code, `scripts/e2e.py`,
`scripts/platform-fault-e2e.py`, and controller failover/draining regression tests.
The existing database/race/restart/container checks remain in the unchanged
runtime workflow; cluster failure injection remains a separate fixture surface.

PostgreSQL/S3/Spark/Kubernetes integration and failure-injection suites were not
rerun for this documentation-only change. Their existing evidence is not a new
passing run. Coverage checks establish structural presence, not complete prose
correctness or automatic discovery of arbitrary Scala method changes; source
review remains required. No documentation publication was performed during
implementation.

## Final review

Completeness: 19 of 19 tasks complete. Correctness: coverage, examples and manual
source review match the documented public surface. Coherence: categorized shared
JSON references, language-specific SDK pages and preserved entry links follow the
design. No unresolved review findings. Capability specs were intentionally skipped
because this change introduces no runtime contract.

The user subsequently authorized release 0.1.4. Release preparation aligns version
metadata and records publication checks separately from the implementation checks
above; runtime behavior and database schema remain unchanged from 0.1.3.

Local release preparation passed the documentation/reference/frozen-version and
generic source/history/rendered public-content checks, `make check`, Java 17
`mvn package`, 9 release tests, 7 chart tests, 19 Helm tests, source chart packaging,
version alignment, formatting/actionlint and all 31 remaining active OpenSpec specs
after archive. A disposable loopback publication rehearsal verified 21 SDK
artifacts, signatures, checksums, sources and API documentation without external
publication. Local Go checks did not configure PostgreSQL/S3 integration fixtures;
the unchanged hosted release checks provide PostgreSQL, restart and container gates.
