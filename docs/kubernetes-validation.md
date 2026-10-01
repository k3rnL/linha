# Disposable Kubernetes validation

`scripts/cluster-e2e.py` and `scripts/restart-e2e.py` target only the explicit
`kind-linha-validation` context and `linha-test` namespace. They do not use the
current kubectl context. The restart script intentionally stops those test
components and restarts the Docker container `linha-implementation-postgres`.
Do not point these fixture names at retained or production workloads.

The validation performed during development used:

- Kind Kubernetes 1.33.1; eight available CPUs and approximately 32 GiB memory.
- A separately running disposable PostgreSQL 17 container attached to Kind's network.
- MinIO built from `RELEASE.2025-04-22T22-12-26Z`, with a durable fixture volume.
- Caller-created `results` and `minio` PVCs backed by directories on the Kind node.
- Two Helm server replicas, the former development-token configuration, projected Kubernetes worker tokens,
  and the example image built from `examples/spark/Dockerfile`.

The scripts currently assume the fixture is already installed. For a repeat run,
create the cluster using a dedicated kubeconfig (to avoid changing your usual
context), load the built images, install MinIO and PostgreSQL, and supply the
chart values below. The current fixture disables public OIDC while keeping worker authentication enabled.
The disposable MinIO credentials are `linha-test` / `linha-test-only-secret`.
Never reuse them elsewhere. Historical logs predate the configuration migration.

```yaml
security:
  enabled: true
  oidc: {enabled: false}
rbac: {namespaceOnly: true}
database:
  postgres:
    host: linha-implementation-postgres
    port: 5432
    database: linha
    sslMode: disable
    secretRef: linha-postgres
    usernameKey: username
    passwordKey: password
image: {repository: linha/server, tag: validation, pullPolicy: Never}
allowedImages: [index.docker.io/linha/spark-example]
local: {enabled: true, existingClaim: results}
s3: {enabled: true, secretRef: linha-config, destinationsKey: s3-destinations}
workerVolumes:
  - name: results
    persistentVolumeClaim: {claimName: results}
workerMounts:
  - {name: results, mountPath: /var/lib/linha/results}
extraEnv:
  - {name: AWS_ACCESS_KEY_ID, value: linha-test}
  - {name: AWS_SECRET_ACCESS_KEY, value: linha-test-only-secret}
```

Provide `linha-postgres` with `username` and `password` for the disposable database.
Provide `linha-config` with `s3-destinations` selecting bucket `linha-results`, prefix `jobs`, endpoint
`http://minio:9000`, region `us-east-1`, and `pathStyle: true` under the destination
name `validation`. The MinIO Service must be named `minio`, port 9000.
Install the Helm release as `validation` in namespace `linha-test`.

Image tags default to `validation`; set `LINHA_TEST_IMAGE_TAG` when using another
worker tag. The test registers its loaded image's immutable digest in containerd.
On a disk-constrained machine, stream image transfer instead of creating a large
intermediate archive:

```sh
docker save linha/server:validation linha/spark-example:validation |
  docker exec -i linha-validation-control-plane \
  ctr -n k8s.io images import --all-platforms --digests -
python3 scripts/cluster-e2e.py
python3 scripts/restart-e2e.py
```

The second script consumes `/tmp/linha-cluster-evidence.json` from the first.
These historical scripts predate schema v5 and SparkApplication submission; use
`scripts/pod-template-e2e.py` with an isolated Spark Operator 2.4.0 fixture for
the current resource/version/lease contract. Prefer a fresh disposable database
and cluster for each run. Delete only the cluster and fixtures you created
when validation finishes. Curated results are in [the validation summary](validation/README.md); raw run output stays local.

## Complete platform fixture (2026-09-30)

Build the server and the connector-equipped Spark example locally as
`linha/server:platform-complete` and `linha/spark-example:platform-complete`.
The scripts require Docker, Kind (`kindest/node:v1.33.1`), Helm, kubectl, Java 17
and Python 3. The local MinIO image is `linha/minio:validation-glibc`; provide a
compatible MinIO binary/image under that tag. The Spark Operator v2.4.0 chart is
read from `/tmp/linha-spark-operator-v2.4.0/charts/spark-operator-chart` (clone the
operator's `v2.4.0` source there). No default kubeconfig/context is used.

```sh
python3 scripts/platform-fixtures.py start
python3 scripts/platform-results-e2e.py
python3 scripts/platform-fault-e2e.py
python3 scripts/platform-cluster-e2e.py
python3 scripts/platform-load-e2e.py
python3 scripts/platform-final-smoke.py
python3 scripts/platform-fixtures.py clean
```

Run these in sequence: the fault script pauses its PostgreSQL container. Fixture
state is under `/tmp/linha-release-fixtures.json` and
`/tmp/linha-platform-cluster-state.json`. The cluster/state names identify a new
isolated Kind installation; labelled fixture containers are removed by `clean`.
Logs/evidence remain in their named temporary directories. Copy the evidence
before cleanup; local images are retained. A single Kind host/POSIX volume does
not establish multi-node storage HA or production capacity.
