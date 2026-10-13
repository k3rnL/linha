## Context

Documentation currently lives in Markdown files and module READMEs. There is no hosted version selector. See proposal.md for the readability and privacy goals.

## Goals / Non-Goals

Goals: equally support JVM SDK developers and Kubernetes operators, make setup and lookup easy, retain tested behavior descriptions, publish frozen version snapshots and remove private deployment linkage.

Non-goals: runtime changes, rebuilding previously published artifacts, adding private integration instructions or replacing the platform API.

## Decisions

1. Apply Diátaxis: tutorials teach a complete working example, how-to pages solve concrete tasks, reference pages list exact interfaces/settings, and explanation pages describe lifecycle/authentication/durability. Each page has one purpose and links to the others.
2. Use pinned MkDocs, Material and mike to render searchable Markdown with a version selector. A documentation branch stores built snapshots; the Actions Pages artifact contains all versions. Main publishes development docs, release tags publish the corresponding version, and aliases distinguish latest released docs from development. Never rebuild unsanitized historical tags automatically.
3. Keep site validation separate from publishing. Pull requests build strictly and validate links without write credentials. Publication requires trusted repository events and scoped Pages/content permissions. Examples use reserved domains and fictitious application identifiers; published source and history are audited before activation.
4. Rewrite reachable local history in an isolated mirror, preserve a private backup and uncommitted work, audit every rewritten commit and tag, then replace refs with expected-old-value checks. Rewritten published refs use lease checks; existing release-publishing automation must not rebuild immutable artifacts during ref cleanup.
5. Add a public-content regression check for known private identifiers without embedding those identifiers in the repository. Keep official project ownership, artifact coordinates and license attribution intact.

## Risks / Trade-offs

- Historical commit URLs change: record the rewrite map privately and keep the public release/version identity.
- Private references can remain in external caches or old clones: clean reachable repository refs and state the limit without claiming erasure outside repository control.
- Documentation dependency drift: pin dependencies and build strictly in CI.
- Development APIs differ from published artifacts: label development documentation and link to release snapshots.
- Hosting needs GitHub Pages activation and published workflow source: prepare and validate the complete site before activation.
