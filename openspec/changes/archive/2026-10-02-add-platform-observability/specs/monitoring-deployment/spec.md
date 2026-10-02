## Purpose

Let Linha installations opt into Prometheus scraping and a useful Grafana operations dashboard through namespaced Helm resources without requiring a monitoring stack for ordinary operation.

## ADDED Requirements

### Requirement: Optional ServiceMonitor
The Helm chart SHALL optionally render a namespaced `monitoring.coreos.com/v1` ServiceMonitor selecting exactly the release's server Service and its named HTTP port at `/metrics`. It SHALL support selector metadata, scrape interval/timeout, target labels, relabeling, sample limits, and optional scrape transport configuration. Discovery SHALL target individual server endpoints, including unready endpoints needed for diagnosis, rather than a load-balanced service address.

#### Scenario: Two server replicas
- **WHEN** ServiceMonitor is enabled for a two-replica deployment
- **THEN** Prometheus discovers two distinct server targets with namespace/deployment/pod identity and shared totals remain subject to documented deduplication

#### Scenario: One replica becomes unready
- **WHEN** a server fails readiness but its metrics HTTP endpoint still responds
- **THEN** the monitor continues to expose its diagnostic target and failed readiness can be observed

#### Scenario: Invalid monitoring settings
- **WHEN** ServiceMonitor is enabled while metrics are disabled, timeout is not shorter than interval, or a reserved identity label is overwritten
- **THEN** Helm reports a clear configuration error

### Requirement: Optional GrafanaDashboard and portable JSON
The chart SHALL independently support a namespaced `grafana.integreatly.org/v1beta1` GrafanaDashboard using packaged dashboard JSON, a configured instance selector, folder, datasource selection, labels/annotations, and resync settings. The same dashboard SHALL be available for manual JSON import without an operator. No remote dashboard download or Grafana credentials SHALL be required by Linha.

#### Scenario: Operator-managed dashboard
- **WHEN** a compatible Grafana Operator and selected Grafana instance exist and the option is enabled
- **THEN** the rendered resource contains the packaged dashboard and configured target/datasource settings

#### Scenario: Manual import
- **WHEN** an operator imports the distributed JSON into Grafana with a Prometheus datasource
- **THEN** core panels work without GrafanaDashboard resources or Linha UI availability

### Requirement: Useful filtered operational dashboard
The proposed dashboard SHALL include service health, client/context demand, requests/outcomes, timings/recovery, workers/engines, Spark allocation, server/API/dependency health, and result/storage panels. It SHALL filter by applicable cluster/namespace/deployment, engine/logical-context, and server dimensions; provide optional admin UI links; and apply each family's correct HA aggregation. It SHALL distinguish unavailable data, successful zero counts, and incomplete engine observations.

#### Scenario: Shared counts and local traffic
- **WHEN** the same durable jobs are visible through multiple replicas while each handles different HTTP traffic
- **THEN** dashboard job totals are deduplicated and local traffic rates are combined correctly

#### Scenario: Dependency and engine observations fail
- **WHEN** shared collection is unavailable or some current engine observations are stale
- **THEN** affected panels display unavailable/incomplete state rather than zero jobs or deceptively complete pod counts

#### Scenario: Context drill-through
- **WHEN** the UI base URL is configured and an administrator follows a context/failure link
- **THEN** the URL preserves the relevant context filter and the UI still applies its own authorization

### Requirement: Optional external operators and namespace scope
Monitoring integrations SHALL default to disabled and SHALL NOT install CRDs/operators or require new cluster-scoped Linha RBAC. Enabling a resource SHALL require its compatible installed CRD; missing CRDs SHALL produce actionable validation errors. Documentation SHALL cover operator selectors, namespace watch/import settings, datasource configuration, multi-cluster labels, and offline Helm rendering.

#### Scenario: Operators are absent and options disabled
- **WHEN** Linha is installed without Prometheus Operator or Grafana Operator and both options are disabled
- **THEN** the chart renders no monitoring custom resources and ordinary namespace-only installation succeeds

#### Scenario: Enable one integration independently
- **WHEN** only ServiceMonitor or only GrafanaDashboard is enabled with its required CRD
- **THEN** that resource renders independently without requiring the other CRD

#### Scenario: Missing enabled CRD
- **WHEN** an enabled integration's CRD is unavailable
- **THEN** rendering or installation fails with the missing kind and required operator identified, rather than silently omitting the requested resource
