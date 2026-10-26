# api-jwt-authentication Specification

## Purpose

Authenticate independent client applications against one Linha resource audience while keeping the server independent of credential acquisition methods.

## Requirements

### Requirement: Independent public API identity configuration
Linha SHALL configure public API JWT authentication independently of browser OIDC, including issuer, one resource audience, enablement and TLS policy. The audience SHALL default to linha. Shared legacy audience settings SHALL fail configuration validation. The server SHALL validate signature, exact issuer, audience, expiry and nonempty subject.

#### Scenario: Ten independent applications
- **WHEN** independent applications present valid tokens with the same configured API audience and different subjects
- **THEN** all can use Linha while retaining separate issuer/subject-based ownership

#### Scenario: Cross-owner retrieval
- **WHEN** one valid application requests another subject's job or result
- **THEN** owner checks reject access without exposing the other application's data

#### Scenario: Forged or mismatched token
- **WHEN** a token is forged, expired, has a different issuer/audience or has no subject
- **THEN** public API authentication rejects it

#### Scenario: Restart and stale worker
- **WHEN** replicas restart and a replaced worker later attempts completion
- **THEN** refreshed client credentials retain normal durable job access and stale worker completion remains fenced
