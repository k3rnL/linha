# Publish a Linha release

A version tag starts the release workflow. After validation, it publishes server
and example-worker images and the Helm chart to GHCR, plus signed SDK artifacts
to Maven Central.
This guide is for maintainers with publishing access.

## 1. Configure public metadata

Keep `release-metadata.json`, all Maven POMs and the license aligned with the
public repository and verified Maven namespace. Linha uses `k3rnL/linha`,
`com.k3rnl` and Apache-2.0.

The published parent is `com.k3rnl:linha-jvm`; the Scala 2.12 modules are
`linha-protocol_2.12`, `linha-client_2.12`, `linha-worker_2.12` and
`linha-spark_2.12` and `linha-hadoop_2.12`. Examples are packaged for the worker image and excluded from
Central. Each SDK publication includes binary, sources, API documentation,
checksums and signatures.

## 2. Configure the release environment

Create a GitHub Actions environment named `release` and set:

| Secret | Value |
| --- | --- |
| `MAVEN_CENTRAL_USERNAME` | Central Publisher Portal user-token username |
| `MAVEN_CENTRAL_PASSWORD` | Matching user-token password |
| `GPG_PRIVATE_KEY` | Complete ASCII-armored private signing-key export |
| `GPG_PASSPHRASE` | Its passphrase |

The Central account must have publishing access to `com.k3rnl`. These are Portal
token credentials, not an account login password. Maven uses server ID `central`.
A key listing is not a private-key export.

Set secrets interactively through GitHub or `gh secret set`. To transfer a key
without printing it or placing it in the repository:

```sh
set -o pipefail
gpg --armor --export-secret-keys YOUR_SIGNING_KEY_FINGERPRINT   | gh secret set GPG_PRIVATE_KEY --env release --repo k3rnL/linha
```

Publish the public key through a Central-supported keyserver. See Sonatype's
[Maven publishing](https://central.sonatype.org/publish/publish-portal-maven/)
and [signature requirements](https://central.sonatype.org/publish/requirements/gpg/).

GHCR uses `GITHUB_TOKEN`; only the image and chart jobs request `packages: write`.
No separate registry PAT is needed. A newly created GHCR package is private by
default, independently of the repository's visibility. After the first chart
upload, open the [chart package settings](https://github.com/users/k3rnl/packages/container/charts%2Flinha/settings)
and set its visibility to **Public**. The chart job verifies an anonymous pull and
fails with the settings URL until public access works. Retry only that job after
changing visibility. GitHub documents
[package access and visibility](https://docs.github.com/en/packages/learn-github-packages/configuring-a-packages-access-control-and-visibility).

## 3. Validate locally

Use Java 17 and Maven 3.9.9. Run the project checks, then rehearse publication:

```sh
python3 scripts/test-release.py
python3 scripts/check-release-artifacts.py
```

The rehearsal builds in a temporary workspace with disposable signing keys and
a loopback Portal fixture. It checks SDK/source/API-documentation artifacts,
checksums and signatures without publishing to Central.

## 4. Prepare and push the tag

Choose a new immutable version; do not reuse a published version. Align the
root/module development versions, commit the code and validate the planned tag:

```sh
python3 scripts/prepare-release.py --check   --tag vX.Y.Z --repository k3rnL/linha
git tag -a vX.Y.Z -m 'Linha X.Y.Z'
git push origin vX.Y.Z
```

Replace `X.Y.Z` with the version matching the development POMs. Prerelease tags
such as `vX.Y.Z-rc.1` are supported. A tag push is the publication action. The
workflow changes metadata only in its disposable workspace and waits for Central
to report publication.

Images receive version and `sha-<commit>` tags, with no implicit `latest` image:

```text
ghcr.io/k3rnl/linha/server:X.Y.Z
ghcr.io/k3rnl/linha/spark-example:X.Y.Z
```

The initial image platform is Linux amd64. The chart is distributed as:

```sh
helm pull oci://ghcr.io/k3rnl/charts/linha --version X.Y.Z
helm upgrade --install linha oci://ghcr.io/k3rnl/charts/linha \
  --version X.Y.Z --namespace linha --create-namespace --values values.yaml
```

Use Helm 3.8 or newer. No `helm repo add` or registry login is required to consume
the public chart. Chart/application versions and its default server image tag
must match the release. Keep the digest printed by Helm when you need a pinned
artifact. Existing GitHub release archives remain usable.
The separate [documentation workflow](docs-contributing.md) captures a versioned
site snapshot when a new release tag is published.

## Recover a failed job

GHCR and Central publication are independent. Inspect the affected registry or
Portal deployment before rerunning failed jobs. An SDK timeout can occur after
Central accepted the bundle. Finish an already accepted valid deployment rather
than republishing an immutable version. Retry an image-only failure without
rerunning a successful SDK publication.

For a release that already has `linha-X.Y.Z.tgz` and `SHA256SUMS` attached, run the
chart-only recovery path:

```sh
gh workflow run release.yml --repo k3rnL/linha -f chart_tag=vX.Y.Z
```

The workflow must be registered in the default branch. To use an updated revision
of an existing workflow, add `--ref BRANCH`. Manual execution downloads the original
published chart and validates its checksum, chart/application versions and image
tag. It uploads that archive and checks an anonymous pull; image and Maven jobs
are skipped. Draft releases and missing or mismatched artifacts are rejected.
This also backfills releases created before OCI chart publishing was introduced.

Do not move a published tag. A code or metadata correction requires a new version.
External registry permissions, namespace access and signing-key discovery still
need verification in the hosted environment.
