## 1. Chart scheduling

- [x] 1.1 Add native affinity and nullable topology spread values, preserve defaults, and validate scheduling value types.

## 2. Documentation

- [x] 2.1 Document default, replacement, and disabled scheduling with a values-file example and keep Helm reference coverage complete.

## 3. Validation

- [x] 3.1 Extend and run Helm render checks for defaults, null/empty overrides, custom placement, independent workers, multiple release selectors, and invalid types.
- [x] 3.2 Run Helm lint, reference and formatting checks, and strict OpenSpec validation; record check scope and the absence of live restart/HA failure injection for this chart-only change.

## Validation results

- `python3 scripts/test-helm.py -v`: 23 tests passed. The four scheduling tests also passed after formatting.
- `helm lint deploy/helm/linha --strict --set security.enabled=false`: passed.
- `python3 scripts/check-reference.py` and `python3 scripts/test-reference.py`: reference coverage passed; 14 tests passed.
- Pinned Black check for `scripts/test-helm.py`, `scripts/format-data.py --check`, and `git diff --check`: passed.
- `openspec validate customize-helm-scheduling --strict`: passed.
- Parsed default manifests matched the original chart for two release names, and the README scheduling example rendered unchanged.
- Validation covers rendering, schema types, and documentation. Live Kubernetes placement, PostgreSQL integration, and process restart/HA failure injection were not run for this chart-only change.
