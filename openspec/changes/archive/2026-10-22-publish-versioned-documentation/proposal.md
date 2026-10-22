## Why

The documentation mixes setup, configuration facts and implementation history, making ordinary tasks hard to follow. Public examples and repository history must remain independent of private deployments.

## What Changes

- Rewrite documentation for SDK developers and Kubernetes operators using tutorials, how-to guides, reference and explanations.
- Add a searchable versioned documentation site and GitHub Pages build/deployment workflow.
- Replace private project and deployment identifiers with standalone examples throughout public files and reachable Git history.
- Add a privacy regression check and verify all documentation links and builds.

## Capabilities

No runtime capability changes. This documentation and tooling change opts out of specification deltas.

## Impact

README files, documentation, site dependencies/build scripts, GitHub workflows, public fixtures and planning artifacts. History sanitization changes commit and tag targets; published Maven artifacts remain immutable. Preserve uncommitted work while rewriting local refs. Hosted deployment follows publication of the reviewed source.
