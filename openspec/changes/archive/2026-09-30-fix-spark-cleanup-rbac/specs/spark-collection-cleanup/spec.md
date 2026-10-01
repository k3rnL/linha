## Purpose

Allow managed Spark drivers to clean up executor resources without leaving the deployment namespace or requiring cluster-wide permissions.

## ADDED Requirements

### Requirement: Namespaced executor collection cleanup
The worker service account SHALL be authorized to delete collections of Pods and ConfigMaps in the release namespace, including when Linha authentication is disabled. This correction SHALL NOT grant collection deletion of other resource types, to the server service account, or in other namespaces. Namespace-only deployments SHALL continue without ClusterRoles or ClusterRoleBindings.

#### Scenario: Spark cleanup by application labels
- **WHEN** a worker deletes Pods or ConfigMaps matching its executor/application label selector
- **THEN** Kubernetes authorizes the collection request and resources outside the selector remain intact

#### Scenario: Namespace boundary
- **WHEN** a worker attempts collection deletion in another namespace
- **THEN** Kubernetes denies the operation

#### Scenario: Permission applies to existing worker identity
- **WHEN** the corrected worker Role is applied to an existing installation
- **THEN** subsequent cleanup calls from its existing worker service account are authorized without rebuilding images
