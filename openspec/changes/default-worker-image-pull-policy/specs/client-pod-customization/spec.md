# Spec Delta

## ADDED Requirements

### Requirement: Managed worker pull policy is shared across roles
The Helm chart SHALL expose one worker image pull policy defaulting to `Always` for both driver and executor containers. An explicit administrator or client Pod-template policy SHALL override this default for both roles when it resolves to one common value; conflicting driver and executor template policies SHALL be rejected. Accepted context versions SHALL retain the resolved policy across replacement.

#### Scenario: New context uses the Helm worker policy
- **WHEN** a context is ensured without administrator or client policy overrides using the Helm chart defaults
- **THEN** its driver and executor main containers use `imagePullPolicy: Always`

#### Scenario: One template policy overrides the default
- **WHEN** an administrator or client configures one common main-container pull policy in either or both worker Pod templates
- **THEN** the resolved policy applies to both driver and executor containers in the new context

#### Scenario: Conflicting template policies are rejected
- **WHEN** the driver and executor templates specify different main-container pull policies
- **THEN** ensure returns a validation error rather than silently applying one policy to both roles

#### Scenario: Existing context is replaced after configuration changes
- **WHEN** a pull-policy setting changes after a context version has been accepted and its driver is replaced
- **THEN** the replacement uses the context's persisted resolved policy rather than the new setting
