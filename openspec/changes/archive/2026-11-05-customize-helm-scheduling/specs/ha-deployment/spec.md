## ADDED Requirements

### Requirement: Configurable server Pod scheduling
The Helm chart SHALL expose server Pod `nodeSelector`, `tolerations`, `affinity`, and `topologySpreadConstraints` using native Kubernetes structures. Omitted or null topology spread configuration SHALL preserve the existing soft hostname spread with maxSkew 1 and a selector scoped to the release's server Deployment. An explicit topology spread list SHALL replace the default completely, including an empty list that disables chart-provided spreading. Affinity SHALL default to no additional placement rules. These settings SHALL affect server Pods independently of worker Pod templates, and chart validation SHALL reject incorrect top-level scheduling types.

#### Scenario: Existing installation retains default placement
- **WHEN** an operator renders or upgrades the chart without scheduling overrides
- **THEN** the server retains its soft hostname spread and release-specific selector, with no new affinity requirement

#### Scenario: Operator configures placement
- **WHEN** an operator supplies a node selector, tolerations, affinity, and multiple topology spread constraints
- **THEN** the server Pod contains those native values and the spread list replaces the default without adding or rewriting selectors
- **AND** worker Pod template values remain independent

#### Scenario: Operator disables default spreading
- **WHEN** an operator sets topologySpreadConstraints to an empty list
- **THEN** the chart renders an empty topology spread list rather than restoring the default

#### Scenario: Independent releases and replacement Pods
- **WHEN** two releases use default scheduling and their server Pods are replaced during a restart or upgrade
- **THEN** each Deployment retains a topology selector matching only its own server Pods

#### Scenario: Malformed scheduling values
- **WHEN** an operator supplies affinity as a list, topology spread constraints as an object, a non-string node selector value, or tolerations as an object
- **THEN** chart schema validation rejects the configuration before installation
