# Maintain the documentation

The site uses pinned MkDocs, Material and mike versions. Markdown stays in `docs/`;
GitHub Pages serves development docs and frozen snapshots from new release tags.
This guide is for contributors and maintainers.

## Preview and validate locally

```sh
python3 -m venv /tmp/linha-docs-env
/tmp/linha-docs-env/bin/python -m pip install -r docs/requirements.txt
/tmp/linha-docs-env/bin/mkdocs serve
```

Open the local URL printed by MkDocs. For a strict build and versioning check:

```sh
PATH=/tmp/linha-docs-env/bin:$PATH mkdocs build --strict
PATH=/tmp/linha-docs-env/bin:$PATH python3 scripts/test-reference.py
PATH=/tmp/linha-docs-env/bin:$PATH python3 scripts/check-reference.py
PATH=/tmp/linha-docs-env/bin:$PATH python3 scripts/check-docs.py
python3 scripts/check-public-content.py
```

The versioning check uses a temporary Git repository. It verifies release snapshot
preservation, development updates, version metadata, root redirects and rejection
of attempts to replace a release snapshot with a different source commit. It does
not write to GitHub or alter branches in your checkout.

Generated `site/` output is ignored. Local validation logs and JSON evidence are
excluded from the site even when they exist under `docs/validation/`.

## Audit public content

CI runs the generic workspace-path check and its synthetic fixture tests. It does
not contain organization-specific names, identifier hashes or a private denylist.
Fork pull requests run the same generic checks without receiving private policy.

Maintainers can perform an additional **local-only** identifier audit. Supply
`LINHA_PUBLIC_CONTENT_DENYLIST` as a JSON array of identifiers. For example, a
policy for an invented integration could contain:

```json
["internal-workload", "corp.example", "io.example"]
```

Keep the real policy outside the checkout and generated site. Load it from a
private local file and require it before inspecting source, history or output:

```sh
export LINHA_PUBLIC_CONTENT_DENYLIST="$(cat /tmp/linha-private-denylist.json)"
python3 scripts/check-public-content.py --require-denylist
python3 scripts/check-public-content.py --history --require-denylist
python3 scripts/check-public-content.py --directory site --require-denylist
unset LINHA_PUBLIC_CONTENT_DENYLIST
```

`--require-denylist` fails when the policy is absent, empty or invalid. Omitting
it permits the generic path check when no policy is configured; a supplied
malformed policy still fails. Identifier matching is case-insensitive and checks
whole names, separator components, domain suffixes and adjacent dotted pairs,
including package prefixes within longer class names. Diagnostics report only
file/line locations and never matched source text or policy values.

Do not commit the policy, embed hashes of its values or send it to CI. The local
identifier audit complements the generic CI check; CI success does not establish
that an organization-specific audit has run.

## Choose one page purpose

| Reader need | Page type | Writing pattern |
| --- | --- | --- |
| Learn through a working example | Tutorial | State prerequisites, give numbered steps and verify each outcome |
| Complete a known task | How-to guide | State assumptions, give focused actions and expected results |
| Look up a field or contract | Reference | Use consistent names, types, defaults and bounds |
| Understand a design choice | Explanation | Describe concepts, lifetime and trade-offs; link to procedures |

This follows [Diátaxis](https://diataxis.fr/). Cross-link related types rather than
mixing long explanations into a copyable task. Examples use reserved `.example`
hostnames and invented workloads. Public docs must not contain real integration
project names, corporate deployment identifiers, secrets or personal workspace paths.

## Maintain complete references

Start at the [reference index](reference/index.md). JSON defines shared model
fields; language tabs show equivalent usage, currently JSON and Scala. Add a
language SDK section when a supported SDK exists, rather than duplicating the
same field tables for each language. Use plain code blocks for language-specific
APIs and YAML-only operator settings.

The coverage inventory is `scripts/fixtures/reference-coverage.json`. It maps
OpenAPI models, selected Go wire/configuration structures, public SDK families,
and server environment variables to canonical page anchors. Chart coverage uses
both schema properties and default values, including inline objects/array items.
Dynamic maps, upstream Kubernetes fields and internal structures have explicit
boundaries in the inventory. Go fields tagged json:"-" are excluded; embedded
selected structs are expanded.

Each model uses an explicit heading anchor, and each field row has an explicit
span anchor derived from its model and dotted path. For example
`SparkSettings.drivers.memory` is `sparksettings-drivers-memory`. Each row names
the field, its type, presence/default, meaning and constraints. Do not add an
empty anchor just to satisfy a check.

When adding or changing an API/configuration field, update its definition, index
target, constraints and source review together. The checker detects missing
model mappings, newly declared nested fields, missing field rows, broken targets,
selected Go-structure changes, new server environment literals and preserved
entry-page anchors. Negative
fixtures exercise omitted fields and broken pages/anchors. CI and `make docs-check`
run both checks alongside strict MkDocs and the existing frozen-version suite.

The structural check does **not** establish that prose/defaults are correct or
automatically discover arbitrary Scala method changes. Review every public SDK
constructor/member/default against its source, including lifecycle behavior.
Review Go normalizers for omission versus explicit zero, computed defaults and
cross-field checks; schema defaults alone are insufficient. Record a discovered
schema/runtime discrepancy in the active change's audit and document the enforced
behavior; resolve any runtime/API change through its own authorized scope.

Keep established entry-page URLs and useful anchors as links to the canonical
reference. Reference changes update development docs through the normal workflow;
published release snapshots are preserved. Review representative lookups from the
home page and model index, including nested backend/Spark fields, before delivery.

## Publish on GitHub Pages

The `documentation` workflow validates every pull request. A main-branch push
updates `dev`; a new `vX.Y.Z` tag captures version `X.Y.Z`. The greatest stable release version owns
`latest`; publishing an older backport leaves that alias unchanged. Prereleases
move `prerelease`. The root redirects to `latest` when it
exists, otherwise to `dev` or the first version.

The workflow preserves generated snapshots on `docs-pages`, uploads the complete
site as a Pages artifact and deploys it through the `github-pages` environment.
Source branches and release tags are not rewritten by documentation publication.
Previously published HTML is retained rather than rebuilt using newer tooling.
The version selector reads `versions.json` generated by mike. Release builds
render their SDK/image versions and omit development-only notices; the source
Markdown keeps copyable examples for the current development checkout. Historical
release notes retain their original version fields.

The workflow requests `contents: write` only to preserve snapshots, `pages: write`
for the deployment and `id-token: write` for its Pages identity. Pull-request checks
have read-only repository permissions. Configure GitHub Pages to use **GitHub
Actions**; the workflow can enable Pages when the repository's settings permit it.
Environment branch/tag restrictions must permit the intended main branch and tags.

The first site publication does not rebuild older tags into versioned snapshots.
Their release notes remain available, and future tags capture their own docs.
A repeated publication of the same source/version reuses its frozen HTML. A
version already associated with a different source commit is rejected.

See [mike](https://github.com/jimporter/mike),
[Material versioning](https://squidfunk.github.io/mkdocs-material/setup/setting-up-versioning/)
and [GitHub Pages workflows](https://docs.github.com/en/pages/getting-started-with-github-pages/using-custom-workflows-with-github-pages)
for the hosting mechanics. The build intentionally pins MkDocs 1.6.1 and the
compatible plugin/theme versions; upgrades require a strict build and snapshot test.
