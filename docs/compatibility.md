# Tested development baseline

| Component | Baseline |
| --- | --- |
| Go server/toolchain | Go 1.27.1, Linux amd64 |
| SDKs | Scala 2.12.20, Java 17, Maven 3.9.9 |
| Spark Operator | Kubeflow 2.4.0, SparkApplication v1beta2 |
| Spark integration | Classic Spark 3.5.6, Scala 2.12 |
| Database | PostgreSQL 16/17, schema 10 |
| Admin build | Node.js 22.17.1, React 19.3.0, Vite 8.3.2, TypeScript 7.0.2 |
| Monitoring | Prometheus 3.7.1, Prometheus Operator 0.94.1 |
| Dashboard operator | Grafana 13.2.3, Grafana Operator 5.25.0 |
| Disposable cluster | Kubernetes 1.33.1 (Kind) |
| Result providers | Persistent POSIX filesystem, AWS S3 API (tested with MinIO) |

METOC currently declares Scala 2.12.20 in `bigdata-metoc-parent/pom.xml` and
Spark 3.5.6 in `spark/pom.xml`, matching this baseline. This does not establish
compatibility with all METOC dependencies, job code, or deployment images.
A Scala 3 or Scala 2.13 build needs its own published bindings and compatible
Spark distribution; the JSON protocol is independent of Scala binary versions.

The example application shades Cats, Circe, Jawn, and Shapeless. Spark's bundled
Cats version otherwise conflicts with Circe at runtime. User images must preserve
this isolation or demonstrate a compatible complete dependency set. Spark itself
is provided by the image and is not bundled in the application assembly.

The Docker example retains Spark's native driver/executor entrypoint. New contexts
use SparkApplication submission with a declared main class and local JAR. Legacy
persisted contexts still invoke `/opt/linha/bin/worker` while draining. Keep the
same logical name across configuration/image changes; the server derives versions.
See [migration and leases](versioned-spark-applications.md).

Database migrations run under a PostgreSQL advisory transaction lock. The current
schema version is 10. Additive migrations do not imply arbitrary binary rollback:
older binaries reject newer schema versions at startup. Back up the database and
result stores together and rehearse upgrades before use with retained jobs.

Schema 10 adds optional client hostnames and admin overview indexes. Upgrade the
server before JVM clients that report hostnames; old clients remain supported.
