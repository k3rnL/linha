# Verification

Verified locally on 2026-10-13 before publication.

- Installed the technical documentation skill and applied Diátaxis with equal coverage for SDK developers and Kubernetes operators.
- Rewrote all public guides, reference/evidence pages, release notes and module READMEs with standalone examples. Typed tutorial snippets compiled against the actual SDK/Spark dependencies.
- Strict MkDocs navigation and local-link/anchor validation passed. Frozen-version checks exercised development updates, immutable release source identity, repeated release reuse, later/older stable publication order and release-specific SDK/image rendering.
- Excluded local validation logs/JSON and build hooks from the site; source, installed SDK sources, rendered output and all ten rewritten historical commits passed the public-content guard.
- Desktop and mobile browser checks passed for responsive layout, version selection, search and guide navigation. No overflow in reviewed 1440px and 390px layouts.
- GitHub Actions lint, Python/data formatting and whitespace checks passed. Existing source checks are recorded in the OAuth change verification.
- Sanitized all ten reachable historical commits and three release tag targets using an isolated mirror, with a private backup and expected-ref checks. Uncommitted work was preserved. Public repository metadata and existing release notes were also checked.

GitHub Pages is active at https://k3rnl.github.io/linha/. The documentation workflow passed and the development site and versions.json returned HTTP 200. Clean main and all three historical tags were published atomically with explicit expected-ref leases; release jobs triggered by tag rewrites were cancelled at the check stage before artifact publication. Release automation was restored. The repository homepage links to the site. Main source CI revealed a Helm 4 diagnostic-format assertion mismatch; the corrected test passed all nineteen tests under both Helm 3.18.4 and Helm 4.3.0. No previous released Maven artifact is overwritten. Local backups and external caches are outside the public source tree; no claim is made that external copies are erased.

## Cached history boundary

After the rewrite, GitHub still resolves an old commit by its SHA. Current branches and tags are clean; cached old views and existing clones are not erased by changing refs. A local support-request draft was prepared but not sent. GitHub determines whether the information meets its sensitive-data removal policy. No full cache erasure is claimed.

## Local identifier audit

The public-content guard contains no organization-specific identifiers or derived hashes. CI runs the generic path check and six synthetic fixtures. An additional local audit uses a private JSON policy kept outside the repository; it checks source, generated documentation and every reachable commit. The two implementation commits were rewritten to remove the former embedded digest policy; historical release tags did not change during this final cleanup. The isolated cleaned history contained 641 unique blobs and passed both identifier and derived-hash audits. Eight release-workflow fixture tests, workflow lint, formatting and strict versioned-documentation checks passed after the guard change.
