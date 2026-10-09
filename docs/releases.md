# Publishing Linha

Branch pushes and pull requests run `.github/workflows/check.yml` without publisher
secrets. A `vMAJOR.MINOR.PATCH` tag (or prerelease such as `v0.1.1-rc.1`) starts
`.github/workflows/release.yml`. It validates publishing setup, runs the same full
suite at the tagged commit, then publishes images and SDKs in separate jobs.
Tag releases are serialized; there is no implicit `latest` image tag.

## One-time public configuration

The repository uses `k3rnL/linha`, the Apache-2.0 license, and the `com.k3rnl`
namespace. `release-metadata.json` and `LICENSE` are public, committed files:

- `repository`: the GitHub `owner/repository`, matching the repository running CI.
- `groupId`: `com.k3rnl`, the publisher's verified Maven Central namespace.
- `license`: the selected license's `name`, HTTPS `url`, and SPDX identifier in `spdx`.
- `developer`: maintainer `id`, `name`, and public contact `email` for the Maven POM.

The release helper rejects incomplete configuration. Normal CI works before this
setup is complete. Maven coordinates use `com.k3rnl`; Scala imports remain
`linha.*`. Projects previously using local `io.linha` artifacts must change their
Maven/SBT dependency group, rebuild with `mvn install`, and reload dependencies.

The parent is `com.k3rnl:linha-jvm`. Published Scala 2.12 artifacts are:

- `com.k3rnl:linha-protocol_2.12`
- `com.k3rnl:linha-client_2.12`
- `com.k3rnl:linha-worker_2.12`
- `com.k3rnl:linha-spark_2.12`

The examples are packaged for the Spark example image and excluded from Central.
The SDK release includes sources and Scala API documentation JARs, PGP signatures,
checksums, and literal project/license/developer/SCM metadata in each POM.

## GitHub environment and credentials

Create the GitHub repository matching `release-metadata.json` and push the project
to it. Create a GitHub Actions environment named `release`. Add these environment secrets
(repository Actions secrets also work). Use GitHub's secret editor or `gh secret
set`; do not commit values or paste private-key exports into issues or logs.

| Secret | Value |
| --- | --- |
| `MAVEN_CENTRAL_USERNAME` | Username from a Central Publisher Portal user token |
| `MAVEN_CENTRAL_PASSWORD` | Password from that same token |
| `GPG_PRIVATE_KEY` | Complete ASCII-armored private-key export |
| `GPG_PASSPHRASE` | Passphrase protecting the exported signing key |

Generate the token in the [Central Publisher Portal](https://central.sonatype.com/).
The account must have publishing access to the verified `com.k3rnl` namespace.
These are token credentials, not the account login password. Actions generates a
Maven settings entry with ID `central`; do not use a literal `${server}` ID.
See Sonatype's [Maven publishing setup](https://central.sonatype.org/publish/publish-portal-maven/).

A `gpg --list-secret-keys` listing is not an export. To send the selected key straight
to the GitHub environment secret without printing or saving it in the repository,
run locally after replacing `OWNER/REPOSITORY`:

```sh
set -o pipefail
gpg --armor --export-secret-keys DFB329E6253D9E07EB9CCC0138EB15D6C7746E79 \
  | gh secret set GPG_PRIVATE_KEY --env release --repo OWNER/REPOSITORY
```

GPG prompts for the passphrase. The secret must begin with
`-----BEGIN PGP PRIVATE KEY BLOCK-----`. Set the other secrets through GitHub's
editor or interactive `gh secret set NAME --env release --repo OWNER/REPOSITORY`.
Maven's BC signer reads the key and passphrase from environment variables during
the publishing step; it does not require a persistent GPG keyring on the runner.

Publish the **public** key to a Central-supported keyserver so Central and users
can verify signatures:

```sh
gpg --keyserver hkps://keyserver.ubuntu.com \
  --send-keys DFB329E6253D9E07EB9CCC0138EB15D6C7746E79
```

Use your current signing-key fingerprint if it changes. See Sonatype's
[signature requirements](https://central.sonatype.org/publish/requirements/gpg/).
Configure environment deployment rules to allow your release tags; required
reviewers are an optional maintainer policy.

GHCR uses the workflow's short-lived `GITHUB_TOKEN`; no GHCR PAT is required.
Only the image publishing job requests `packages: write`. Published image paths
are lowercase:

```text
ghcr.io/owner/repository/server:0.1.1
ghcr.io/owner/repository/spark-example:0.1.1
```

Each also receives `sha-<full-commit-sha>` and source/revision/version/license OCI
labels. The Helm chart defaults to the GHCR server repository. Initial images target `linux/amd64`, with Spark 3.5.6 / Scala 2.12 / Java 17
for the example. The server image embeds the admin console. User business worker
images are built by their application projects.

## Create a release

1. Finish the public configuration and secrets above. Keep the root version and
   every module's parent version aligned, for example `0.1.1-SNAPSHOT`.
2. Run the local checks and commit the release configuration/code changes.
3. Validate the intended tag against the configured repository:

   ```sh
   python3 scripts/prepare-release.py --check --tag v0.1.1 --repository OWNER/REPOSITORY
   ```

4. Create and push the version tag when you intend to publish:

   ```sh
   git tag -a v0.1.1 -m 'Linha 0.1.1'
   git push origin v0.1.1
   ```

The workflow changes versions and metadata in its disposable workspace. It does
not commit generated POMs back to the branch. The declared development version
must match the release (`0.1.1-SNAPSHOT` also permits `v0.1.1-rc.1`). Update all
parent versions together before releasing the next minor/patch version.

Central publication is automatic after checks pass and waits for Central to
report `PUBLISHED`. A tag push is therefore the publication action, without a
separate manual Portal step unless your GitHub environment policy requires one.

## Local publication rehearsal

Use Java 17, Maven 3.9.9, Python 3 and GnuPG:

```sh
python3 scripts/test-release.py
python3 scripts/check-release-artifacts.py
```

The second command copies source/POM files to a temporary directory, supplies
fixture metadata and a disposable signing key, then sends the Central bundle to a temporary HTTP receiver bound to loopback.
The receiver simulates a completed Portal deployment. It verifies the parent
and four SDKs, every signature/checksum, and nonempty sources/API documentation.
It uses isolated Maven settings and skips Maven installation. No real signing
key, publishing token, local artifact replacement or public registry write is needed.
CI validation and publishing install Apache Maven 3.9.9 with a pinned SHA-512
checksum. The hosted runner's Maven 3.10.0 stages repository bookkeeping files
that our bundle verification rejects; do not weaken that artifact allowlist.
Dependency/plugin downloads still require network access. CI also runs this
rehearsal, workflow lint, the Go/PostgreSQL and JVM suites, browser tests,
Helm/observability contracts, and the all-process restart smoke.

## Recovery

GHCR and Central are independent; the release is not an atomic transaction.
If a job fails, inspect its log and the corresponding registry/Portal deployment.
Use GitHub's **Re-run failed jobs**, preserving the original tag and commit. An
image job can safely repeat both builds if its first image was already pushed.

Before retrying a failed Central job, check the Portal: a timeout may happen after
Central accepted or published the deployment. Finish a pending valid deployment
in the Portal if necessary. Do not rerun Central when that version is already
published, and do not move a published version tag. Published Central versions
are immutable; a code or metadata correction requires a new version. If an image
job alone failed, retry that job without republishing successful Maven artifacts.

Local checks cannot prove GitHub environment permissions, GHCR access, namespace
access or public-key discovery by Central. The first real release verifies these
external settings.

## Local validation evidence (2026-10-02)

- Eight release-preparation contracts pass, including invalid tags, incomplete
  metadata, namespace/repository mismatches and consistent module coordinates.
- The real Central plugin submitted a bundle to the loopback fixture: 17 artifacts
  verified with PGP signatures, four checksum algorithms, sources and API docs.
- Java 17 `mvn package`, Go checks/vet and the PostgreSQL race suite pass. The
  corrected overview timing fixture also passes 20 consecutive race runs.
- The process-restart smoke passes with the new Maven coordinates and a JAR name
  derived from the project version.
- Thirteen Helm rendering checks, actionlint, formatting and strict OpenSpec
  validation pass.
- Both Docker images build locally; the server reports version `0.1.0`, and the
  Spark image contains the versioned example JAR, connectors and native entrypoint.

GitHub-hosted execution and actual GHCR/Central publication are verified below.

The first tagged validation restored a cached Go test success without restoring
its generated `/tmp` API fixture file. The workflow now runs fixture generation
and measured load checks with `-count=1`. Verified by reproducing the cached-pass
case with a deleted output, regenerating all 17 responses twice, and validating
them against OpenAPI along with the 40 PromQL expressions and load fixture.

## First hosted release (2026-10-02)

Release `v0.1.0` at `bd12e85531e6f1975d174331f9bf14a08905ef69` completed
[all validation and publishing jobs](https://github.com/k3rnL/linha/actions/runs/37000431814).
The permanent fixture-cache correction on `main` is `0de744e` and also passed
[its branch CI](https://github.com/k3rnL/linha/actions/runs/37007633896). The release
tag stayed unchanged; the failed validation was retried after removing the one
stale Go cache. No artifacts had been published before that retry.

Both GHCR tags allow anonymous pulls:

- `ghcr.io/k3rnl/linha/server:0.1.0` —
  `sha256:600526e385a2affd34e7628314dd2064aec4b9f8556b450c3e098d218a0c7581`
- `ghcr.io/k3rnl/linha/spark-example:0.1.0` —
  `sha256:6b11842523bd5eb33d7562375a0b8580f0fe722949c94dcac35c8767fce0ed71`

The published server image was pulled and reports `Linha 0.1.0`. Central confirmed
the deployment as published, and HTTP checks confirmed all 17 parent/SDK artifacts
and their 17 detached signatures at `repo.maven.apache.org`: the parent POM and
four SDK modules, each with POM, binary, sources and API-documentation JARs.
