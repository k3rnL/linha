# Run your first job

In this tutorial you will build Linha, submit raw and typed jobs, restart the
processes, and retrieve the original results. The fixture uses a lightweight JVM
worker rather than Spark, so no Kubernetes cluster is required.

## Before you start

Install Git, Go 1.27.1, Java 17, Maven 3.9.9, Node.js 22.12+ with npm, Python 3 and
Docker. Docker must be running and your account must be able to start containers.
Allow the fixture to download the PostgreSQL image. See the
[compatibility matrix](compatibility.md) for the pinned engine dependencies.

## 1. Get the source

```sh
git clone https://github.com/k3rnL/linha.git
cd linha
java -version
```

The Java command should report Java 17. Select a Java 17 installation using
`JAVA_HOME` if your machine has several versions.

## 2. Build the server and SDKs

```sh
mvn -B install
make build
```

Maven installs the SDK binaries and source JARs in your local Maven repository.
`make build` builds the console and embeds it in `bin/linha-server`. Check it:

```sh
bin/linha-server --version
```

## 3. Submit jobs and retrieve their results

```sh
python3 scripts/e2e.py
```

The fixture starts PostgreSQL and Linha, submits requests through the JVM client,
then starts the worker. It checks JSON results, file results and a failing result
callback. Successful output contains two `PASS:` lines covering persisted diagnostics and
raw/typed jobs, file output and restart retrieval. A nonzero exit code means one
of its assertions failed.

The fixture explicitly disables authentication for its local processes. It does
not change any existing deployment or use a permanent database.

## 4. Verify retrieval across restarts

The same fixture restarts the client, server and worker and retrieves results
using the original identifiers. A successful run confirms that acceptance and
retained result metadata survive those process restarts. The script removes the
containers and temporary result directory when it finishes.

You now have a working server and locally installed SDKs. Continue with
[writing client and worker code](walkthrough.md), or
[deploy on Kubernetes](operations.md) to run the Spark engine.
