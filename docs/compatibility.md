# Compatibility matrix

These versions define the tested baseline, rather than a promise that every
newer or older dependency combination works.

| Component | Baseline |
| --- | --- |
| Server/toolchain | Go 1.27.1, Linux amd64 |
| SDK | Scala 2.12.20, Java 17, Maven 3.9.9 |
| Spark | Classic Spark 3.5.6, Scala 2.12 |
| Spark Operator | Kubeflow 2.4.0, SparkApplication v1beta2 |
| PostgreSQL | 16/17, schema 10 |
| Web build | Node.js 22.12+, React/TypeScript with committed lockfile |
| Cluster fixture | Kubernetes 1.33.1, Kind |
| Result storage | Shared POSIX filesystem, AWS S3 API tested with MinIO |
| Monitoring fixture | Prometheus 3.7.1 / Operator 0.94.1; Grafana 13.2.3 / Operator 5.25.0 |
| S3 connectors | Hadoop AWS 3.3.4, AWS SDK bundle 1.12.262, Spark Hadoop Cloud 3.5.6 |
| Documentation | MkDocs 1.6.1, Material 9.7.7, mike 2.2.0 |

The protocol is JSON and independent of Scala binary versions. The current JVM
artifacts target Scala 2.12; Scala 2.13 or 3 requires compatible bindings and Spark
libraries. Do not mix Spark/Hadoop connector versions arbitrarily.

The example assembly shades Cats, Circe, Jawn and Shapeless to avoid conflicts
with Spark's bundled dependencies. Spark is provided by the image rather than
bundled into the application JAR. Preserve equivalent isolation in custom images.

The image retains Spark's native driver/executor entrypoint and contains the JAR
named in `application.mainApplicationFile`. Linha does not transfer business code
from the client at submission.

Database migrations are serialized through a PostgreSQL advisory lock. Current
schema 10 includes dataset metadata, observations, durable accounting, admin
sessions/audit, hostnames and overview indexes. Older server binaries reject
newer schemas; additive migration does not imply arbitrary binary rollback.
