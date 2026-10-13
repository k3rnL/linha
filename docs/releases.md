# Publish a Linha release

A version tag starts the release workflow. After validation, it publishes server
and example-worker images to GHCR and signed SDK artifacts to Maven Central.
This guide is for maintainers with publishing access.

## 1. Configure public metadata

Keep `release-metadata.json`, all Maven POMs and the license aligned with the
public repository and verified Maven namespace. Linha uses `k3rnL/linha`,
`com.k3rnl` and Apache-2.0.

The published parent is `com.k3rnl:linha-jvm`; the Scala 2.12 modules are
`linha-protocol_2.12`, `linha-client_2.12`, `linha-worker_2.12` and
`linha-spark_2.12`. Examples are packaged for the worker image and excluded from
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

GHCR uses `GITHUB_TOKEN`; only the image job requests `packages: write`.
No separate registry PAT is needed.

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

The initial image platform is Linux amd64. The chart is packaged with the release.
The separate [documentation workflow](docs-contributing.md) captures a versioned
site snapshot when a new release tag is published.

## Recover a failed job

GHCR and Central publication are independent. Inspect the affected registry or
Portal deployment before rerunning failed jobs. An SDK timeout can occur after
Central accepted the bundle. Finish an already accepted valid deployment rather
than republishing an immutable version. Retry an image-only failure without
rerunning a successful SDK publication.

Do not move a published tag. A code or metadata correction requires a new version.
External registry permissions, namespace access and signing-key discovery still
need verification in the hosted environment.
