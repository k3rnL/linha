## Context

The release workflow validates tagged commits before publishing images and SDKs.
The 0.1.3 Helm archive and checksum are already attached to a public GitHub release.
See proposal.md for the missing distribution path.

## Goals / Non-Goals

**Goals:** Follow the shared `ghcr.io/<owner>/charts/<name>` layout, automate future
tag publication and permit a chart-only backfill of an existing release.

**Non-Goals:** No runtime or database changes. Request acknowledgement, ownership,
retry, lease fencing, result publication and restart recovery remain unchanged.
Do not rebuild or republish already released SDKs/images during a chart backfill.

## Decisions

- Extend the existing release workflow with a manual `chart_tag` input. Normal
  prepare/check/image/SDK jobs run only for tag pushes. The chart job requires
  successful release validation on tag pushes; manual runs consume an already
  published release instead. This permits recovery without rerunning Central.
- Use pinned Helm setup and the job-scoped `GITHUB_TOKEN` with `packages: write`.
  Package through Helm, validate chart/application/image versions, and push into
  the owner's shared charts namespace. Package uploads require no stored PAT.
- Backfill downloads the original archive plus SHA256SUMS from a published release
  and validates both before upload. It never repackages the old version.
- Verify a pull with empty Helm and Docker credential configurations and compare
  the downloaded archive bytes. Document GHCR's independent package visibility.
  The workflow must not report success while the package is private.
- Serialize release runs by the selected release tag for both normal and manual
  execution. Keep the separate release environment and existing validation gates.

## Risks / Trade-offs

- New GHCR packages default to private -> require owner visibility setup and fail
  anonymous verification with the package settings URL until it is public.
- Cross-registry publication is not atomic -> chart-only retry avoids duplicate
  Central publication. Existing OCI tags should be treated as release versions;
  consumers can also pin the digest printed by Helm.
- Manual dispatch requires a registered workflow -> push the reviewed change to
  a branch and dispatch that revision through the existing release workflow;
  if GitHub requires default-branch activation, expose that requirement explicitly.

## Migration Plan

Run validation, publish the unchanged 0.1.3 chart through the manual path, set its
GHCR package public if necessary, verify anonymous pull/template, and document the
reference. Keep existing release downloads and source-checkout installs usable.
No published Git tag is moved and no application release number changes.
