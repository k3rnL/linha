# Linha worker SDK

The worker SDK registers raw handlers or typed entrypoint codecs, claims durable
attempts, reports progress, renews leases and publishes successful results.
The Spark module supplies one shared SparkContext with request sessions,
job-group cancellation and scoped cache/broadcast cleanup.

Managed workers use Linha-injected Kubernetes identity and credentials. These
internal worker tokens are separate from the application client's OAuth secret.

- [Worker bootstrap and job tutorial](../../docs/walkthrough.md)
- [Result files and datasets](../../docs/dataset-results.md)
- [SDK reference](../../docs/sdk-reference.md)

Include matching `com.k3rnl:linha-worker_2.12` and `linha-spark_2.12` artifacts in
the business image. The development version is `0.1.4-SNAPSHOT`; install binaries
and sources with Java 17 `mvn -B install` from the repository root.
