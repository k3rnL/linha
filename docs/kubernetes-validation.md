# Run disposable Kubernetes validation

The cluster scripts exercise managed Spark, replicas, resources and retained
results. They are development fixtures, not a way to test an existing installation.
Use only their named disposable clusters and databases.

## Prepare the fixture inputs

Install Docker, Kind, Helm, kubectl, Java 17, Python 3 and the normal project build
requirements. The recorded Kind baseline is Kubernetes 1.33.1 with about eight
available CPUs and 32 GiB memory.

Build the server and connector-equipped worker:

```sh
make build
scripts/prepare-spark-connectors.sh
mvn -B package
docker build -f server/Dockerfile -t linha/server:platform-complete .
docker build -f examples/spark/Dockerfile -t linha/spark-example:platform-complete .
```

The current platform fixture also requires a compatible MinIO server image tagged
`linha/minio:validation-glibc`. Provide that local fixture image before starting;
it is a test input, not a published Linha artifact.

The scripts read Spark Operator's chart from a fixed temporary checkout:

```sh
git clone --branch v2.4.0 --depth 1   https://github.com/kubeflow/spark-operator.git /tmp/linha-spark-operator-v2.4.0
```

Do not replace a pre-existing temporary checkout without checking its purpose.

## Run the current platform sequence

```sh
python3 scripts/platform-fixtures.py start
python3 scripts/platform-results-e2e.py
python3 scripts/platform-fault-e2e.py
python3 scripts/platform-cluster-e2e.py
python3 scripts/platform-load-e2e.py
python3 scripts/platform-final-smoke.py
python3 scripts/platform-fixtures.py clean
```

Run sequentially: fault checks pause their fixture database. The cluster script
creates a dedicated `linha-platform-*` Kind cluster and kubeconfig, loads images,
and installs the operator. Its state is recorded in
`/tmp/linha-platform-cluster-state.json`; the local fixture state is
`/tmp/linha-release-fixtures.json`.

Capture needed evidence before `clean`. Cleanup removes only labelled fixture
containers and the cluster described by its validated state. Locally built images
remain. If a run stops early, inspect that fixture's state and logs before cleanup.

## Use the focused template fixture

`scripts/pod-template-e2e.py --state <state-file>` expects an already prepared
`linha-pods-*` Kind cluster, matching dedicated context/kubeconfig, loaded worker
image and disposable PostgreSQL container. It checks driver/executor templates,
resources, allocation, replacement and retained results. Read the script's state
validation before supplying a state file.

The older `cluster-e2e.py` and `restart-e2e.py` record earlier launch paths and
require their own preinstalled fixture. Prefer the current platform/template
sequence for SparkApplication and client-lease behavior.

A single Kind host and shared POSIX volume establish application recovery only.
See [validation scope](validation.md) for the limits of these checks.
