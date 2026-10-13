## MODIFIED Requirements

### Requirement: Independent deployment and security settings
The chart SHALL expose PostgreSQL connection settings under database.postgres and read credentials using secretRef, usernameKey, and passwordKey. API JWT authentication SHALL be configurable under security.api and browser OIDC under security.oidc. A master security.enabled switch SHALL disable all Linha authentication without a deployment mode. Disabling API authentication SHALL NOT disable browser or worker authentication. Anonymous public access SHALL use one stable owner and SHALL NOT promise per-client isolation or remap retained ownership.

#### Scenario: Custom database Secret keys
- **WHEN** the administrator selects a Secret and custom username/password keys
- **THEN** the chart reads those keys independently and passwords containing URI syntax remain literal credentials

#### Scenario: Authentication disabled
- **WHEN** security.enabled is false
- **THEN** clients and workers operate without OIDC or bearer credentials using a stable anonymous owner and worker routing identities, while durable leases and attempt fencing still apply

#### Scenario: Only API authentication disabled
- **WHEN** security is enabled and security.api.enabled is false
- **THEN** public requests use the anonymous owner and worker requests still require verified assigned identities
