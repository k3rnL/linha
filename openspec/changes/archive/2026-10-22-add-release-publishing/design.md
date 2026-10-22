## Context

The repository already validates Go/PostgreSQL, Scala/JVM, React, Helm, OpenAPI
and PromQL. The publisher selected the verified namespace com.k3rnl. Development artifacts
use 0.1.0-SNAPSHOT. Public releases target k3rnL/linha with Apache-2.0 licensing. The existing server
Dockerfile consumes a built binary; the Spark example consumes a versioned jar
and prepared S3 connectors.

## Goals / Non-Goals

Deliver a repeatable tag release to GHCR and the Central Publisher Portal.
Preparation and local tests do not publish, create a GitHub release or change
application execution semantics. Durable acknowledgement, owner isolation,
idempotency, retries, attempt fencing, result publication and restart recovery
remain covered by the existing regression suite.

## Decisions

- Make checks reusable by the tag workflow, with read-only default permissions.
  Pull requests cannot use publishing credentials. Pin external Actions revisions.
- Accept vMAJOR.MINOR.PATCH with an optional prerelease suffix; validate it against
  the declared Maven development version. Use exact version and commit image tags;
  avoid an implicitly movable latest tag. Initial images target Linux amd64.
- Publish server and the existing Spark example. User business worker images remain
  the responsibility of their application projects.
- Keep public publishing metadata in a tracked release configuration. Validate
  repository identity, groupId and required license/developer values before any
  writes. Materialize release versions and literal POM metadata in the release
  workspace; use com.k3rnl coordinates and retain the development snapshot version.
- Use a release-only Maven profile: Scala documentation jars, PGP signatures and
  Sonatype's Central publishing extension. Select only the parent and four SDK
  modules for Central; retain example packaging for the Spark image.
- Use GITHUB_TOKEN with packages:write only in the image job. Central credentials
  and an ASCII-armored signing key/passphrase live in GitHub Actions secrets.
  A named release environment permits maintainers to configure their own policies.
- Document non-atomic registry publication and GitHub's failed-job retry flow.
  Never silently replace an already published Central version.

## Risks / Trade-offs

- Namespace/license/maintainer identity is a publisher decision: do not invent it.
  Missing configuration prevents publication while normal CI remains usable.
- Central may reject signatures or namespace access despite local artifact checks:
  document the required external verification and leave the first real publish
  as an operator step.
- Spark images are large and architecture-specific: retain the currently verified
  Spark 3.5.6 / Scala 2.12 / Java 17 baseline and explicit amd64 scope.
- Local tests cannot prove hosted Actions permissions or public registry access.
  Validate workflow syntax, artifacts, POMs and disposable signatures locally.

## Validation detail

The local rehearsal redirects the real Central publishing plugin to a temporary
loopback HTTP receiver, using disposable signatures and isolated settings. It
verifies the uploaded bundle rather than relying on the plugin's skipPublishing
mode, which omits module staging. A pre-existing overview test fixture now uses
a single statement timestamp so its exact processing-duration assertion is
deterministic under the race detector. Application timing behavior is unchanged.

Tests that emit API fixture files or collect performance measurements run with
`-count=1`: Go's restored test-result cache records success but does not recreate
temporary files in a fresh runner. Normal package checks retain build/test caching.

Hosted validation and publishing use Apache Maven 3.9.9, installed from the
official archive with a pinned SHA-512 checksum. The runner's Maven 3.10.0
stages `_remote.repositories` and local metadata into the Central bundle; the
strict artifact allowlist rejects these files. The signed loopback rehearsal
passes with 3.9.9, so both rehearsal and real publication use that same version.
