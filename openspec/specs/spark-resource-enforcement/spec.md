# spark-resource-enforcement Specification

## Purpose

Make the CPU budgets selected for managed Spark drivers and executors visible and enforced as Kubernetes container requests and limits.

## Requirements


### Requirement: CPU requests and limits follow engine settings
Each newly provisioned driver runtime container SHALL have CPU request and limit equal to `drivers.cores` by default, with explicit `coreRequest` and `coreLimit` overriding those container budgets. Each executor runtime container SHALL have CPU request and limit equal to `executors.cores` by default, with explicit `coreRequest` and `coreLimit` overriding those container budgets. Executor task parallelism SHALL continue to use that executor core count. These rules SHALL apply to static and dynamic executor allocation and to both legacy and template-based contexts.

#### Scenario: Distinct resource budgets
- **WHEN** a client selects 2 driver cores and 3 executor cores
- **THEN** the generated driver requests and limits 2 CPUs and each executor requests and limits 3 CPUs

#### Scenario: Dynamic allocation
- **WHEN** dynamic allocation provisions an executor
- **THEN** its runtime container uses the configured executor CPU request and limit independently of the number of executors

### Requirement: Resource enforcement preserves memory and durable context behavior
The correction SHALL preserve existing JVM heap settings, memory overhead and memory requests/limits. Replacements SHALL apply the CPU budgets from the persisted context without requiring resubmission or changing context identity. Existing running Pods SHALL not be automatically restarted as part of applying this server correction.

#### Scenario: Driver replacement after server restart
- **WHEN** a context is recovered by a corrected server and a replacement driver is provisioned
- **THEN** the replacement and its executors enforce the persisted CPU settings and retained job results remain retrievable

#### Scenario: Memory accounting
- **WHEN** a client specifies driver and executor memoryMi values
- **THEN** the worker JVM heap configuration retains those values and memory requests and limits retain their existing overhead accounting
