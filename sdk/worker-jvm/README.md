# Linha worker SDK (Scala/JVM)

Planned module. Supplies the Linha JVM process entrypoint included in user images,
handler registration, registration/readiness, work claims, leases/heartbeats,
cancellation, progress and completion reporting, and result publication. The Spark
integration supplies a shared SparkContext and request-scoped execution resources.
See `worker-runtime` and `managed-spark-engine` in the active OpenSpec change.
