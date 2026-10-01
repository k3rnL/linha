# spark-application-provisioning Specification

## Purpose
Provision managed Spark workers through operator submission with consistent heap and container resources, while preserving ownership and user Pod customization.

## Requirements

### Requirement: Managed Spark submission
New managed Spark contexts SHALL declare a main class and local application JAR and SHALL run through SparkApplication cluster submission with no operator restart loop.
#### Scenario: Heap plus overhead
- **WHEN** a client requests memory 10g and memoryOverhead 2g
- **THEN** Spark submission SHALL configure a 10g JVM heap and a 12Gi container request and limit
#### Scenario: Native customization
- **WHEN** a client supplies validated volumes, environment or pull secrets
- **THEN** the driver and executor templates SHALL preserve them with managed identities protected
### Requirement: Fenced lifecycle
Linha SHALL observe SparkApplication and driver identity separately and SHALL only delete resources it owns using UID preconditions.
#### Scenario: Pending driver
- **WHEN** a SparkApplication exists before its driver Pod
- **THEN** the instance SHALL remain starting without duplicate submission
#### Scenario: Lost driver
- **WHEN** a driver disappears while capacity is needed
- **THEN** Linha SHALL retire its application and provision a replacement instance
#### Scenario: Legacy stored context
- **WHEN** a previously persisted context lacks application settings
- **THEN** Linha SHALL retain its legacy provisioning path while draining accepted work
