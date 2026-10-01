# Contributing to Linha

Use Java 17, Maven 3, the Go version in `go.mod`, and Python 3.12. The formatting
commands use [uv](https://docs.astral.sh/uv/) to run pinned Python tools without
installing them globally. Scala formatting uses the pinned Maven Spotless plugin
and `.scalafmt.conf`; editor indentation defaults are in `.editorconfig`.

```sh
make format
make format-check
make check
mvn -B install
openspec validate --all --strict
```

`mvn install` runs JVM tests and installs binaries with matching sources JARs.
`make check` runs Go tests and vet. Database and S3 integration tests require
`LINHA_TEST_DATABASE` and `LINHA_TEST_S3_ENDPOINT`; without them the corresponding
tests explicitly skip. Use disposable services. The CI workflow also checks the
Helm render variants and OpenAPI contract. See [validation](docs/validation.md)
and [Kubernetes validation](docs/kubernetes-validation.md) for the broader tests.

`make format` covers Go, Scala, Python, SQL, Maven POMs, JSON and plain YAML.
Helm templates contain Go template syntax and are edited manually; validate them
with `scripts/test-helm.py`. Avoid changing runtime behavior in formatting edits.
OpenSpec is used for features, and completed changes remain in the archive.

Commit source, tests, specs, chart defaults and curated documentation. Keep these
local: `.env` files, `deploy/myvalues.yaml`, IDE metadata, generated JARs/binaries,
Python caches and raw validation logs/JSON output. The ignore rules cover them.
The shared validation summaries record the outcomes and limits; individual runs
produce disposable IDs and machine-specific paths that do not belong in commits.

Before staging, run `git status --short` and review `git diff --cached --check`
and `git diff --cached --stat`. Use your own deployment values and credentials
outside the tracked chart defaults.
