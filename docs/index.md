# Linha documentation

Linha lets an application submit work now and retrieve the outcome later.
A job ID remains useful across application, server and worker restarts. You provide
business code in a worker image; Linha records requests and manages the engine.

These pages serve application developers and Kubernetes operators equally.

<!-- development-only:start -->
!!! note "Development version"
    This version documents the current source checkout, including OAuth 2.0 client
    credentials support. Build the `0.1.2-SNAPSHOT` SDKs from source until that
    feature is released. Published `0.1.2` artifacts do not contain this API.
    Use the version selector for documentation captured with a newer release.

<!-- development-only:end -->

| I want to… | Start here |
| --- | --- |
| Try Linha locally | [Run your first job](quickstart.md) |
| Write a job and submit it | [Build a Scala client and worker](walkthrough.md) |
| Deploy an HA server | [Deploy on Kubernetes](operations.md) |
| Connect an application securely | [Configure OAuth 2.0 client credentials](client-authentication.md) |
| Configure Spark resources and scaling | [Manage Spark applications](versioned-spark-applications.md) |
| Inspect contexts and replay requests | [Enable the administrator console](admin-ui.md) |
| Build a Grafana view | [Monitor queries and resources](observability.md) |
| Find a field or endpoint | [Configuration](configuration.md), [SDK](sdk-reference.md), [HTTP API](http-api.md) |
| Understand durability and context lifetime | [Architecture](architecture.md) |

## Learn, operate, look up, understand

**Tutorials** walk through a working outcome. **How-to guides** solve a specific
operational or development task. **Reference** pages list contracts and defaults.
**Explanation** pages describe why the system behaves as it does.

The documentation site keeps a development version and independent release
snapshots. Existing snapshots remain available when the development docs change.
[Release notes](releases/index.md) describe the behavior of earlier releases.
