# Verification

Verified locally on 2026-10-13 before publication.

- Installed the technical documentation skill and applied Diátaxis with equal coverage for SDK developers and Kubernetes operators.
- Rewrote all public guides, reference/evidence pages, release notes and module READMEs with standalone examples. Typed tutorial snippets compiled against the actual SDK/Spark dependencies.
- Strict MkDocs navigation and local-link/anchor validation passed. Frozen-version checks exercised development updates, immutable release source identity, repeated release reuse, later/older stable publication order and release-specific SDK/image rendering.
- Excluded local validation logs/JSON and build hooks from the site; source, installed SDK sources, rendered output and all ten rewritten historical commits passed the public-content guard.
- Desktop and mobile browser checks passed for responsive layout, version selection, search and guide navigation. No overflow in reviewed 1440px and 390px layouts.
- GitHub Actions lint, Python/data formatting and whitespace checks passed. Existing source checks are recorded in the OAuth change verification.
- Sanitized all ten reachable historical commits and three release tag targets using an isolated mirror, with a private backup and expected-ref checks. Uncommitted work was preserved. Public repository metadata and existing release notes were also checked.

Hosting publication and final remote verification will be recorded after the complete reviewed source is committed and published. No previous released Maven artifact is overwritten. Local backups and external caches are outside the public source tree; no claim is made that external copies are erased.
