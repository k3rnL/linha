## MODIFIED Requirements

### Requirement: Explicit administrator authorization
With security enabled, the server SHALL require validated OIDC identity and explicit configured administrator membership for every administrative read or mutation. A viewer SHALL have cross-owner inspection/result access; an operator SHALL additionally create, replay, and cancel requests. Ordinary SDK APIs SHALL retain their owner scope. Enabling the UI with security enabled but OIDC disabled SHALL fail configuration validation. With the master security switch disabled, an explicitly enabled UI SHALL use anonymous operator access and attribution.

Subject and claim rules SHALL inherit the configured OIDC issuer when their issuer is omitted. Legacy explicit issuers SHALL be accepted only when they equal that configured issuer. Missing membership rules SHALL continue to deny administration; removing repeated issuer fields SHALL NOT authorize identities from another issuer.

#### Scenario: Application identity is not an administrator
- **WHEN** an authenticated ordinary client calls an admin endpoint or supplies a forged owner/role header
- **THEN** access is denied without exposing another owner's details or applying a mutation

#### Scenario: Viewer attempts cancellation
- **WHEN** a viewer submits a creation, replay, or cancellation operation directly to the API
- **THEN** the server rejects it even if the browser control was bypassed

#### Scenario: Independent security settings
- **WHEN** the UI is enabled under each supported security configuration
- **THEN** master-disabled security permits anonymous operation, configured OIDC requires admin membership, and security-enabled/OIDC-disabled configuration does not grant anonymous administration

#### Scenario: Issuer omitted from membership rules
- **WHEN** viewer or operator rules specify subjects or claim paths/values without an issuer
- **THEN** the rules match only identities validated against the configured OIDC issuer

#### Scenario: Legacy issuer configuration
- **WHEN** an existing rule specifies an explicit issuer
- **THEN** a matching configured issuer is accepted and a conflicting issuer fails configuration validation

#### Scenario: Session survives a configuration simplification
- **WHEN** a server restarts with equivalent issuer-inheriting rules replacing repeated issuers
- **THEN** retained sessions are evaluated against the same trusted issuer and administrator membership, without changing public job ownership
