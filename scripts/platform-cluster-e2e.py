#!/usr/bin/env python3
"""Disposable Kind + Spark Operator release acceptance; never uses the current context."""
import json
import pathlib
import subprocess
import tempfile
import time

root = pathlib.Path(tempfile.mkdtemp(prefix="linha-platform-cluster-"))
cluster = "linha-platform-" + str(int(time.time()))
state = dict(
    cluster=cluster,
    context="kind-" + cluster,
    kubeconfig=str(root / "kubeconfig"),
    directory=str(root),
    postgres=cluster + "-pg",
    s3=cluster + "-s3",
)
(root / "state.json").write_text(json.dumps(state))
pathlib.Path("/tmp/linha-platform-cluster-state.json").write_text(json.dumps(state))


def run(*args, **kw):
    return subprocess.run(args, check=True, **kw)


run(
    "kind",
    "create",
    "cluster",
    "--name",
    cluster,
    "--image",
    "kindest/node:v1.33.1",
    "--kubeconfig",
    state["kubeconfig"],
)
for name, image, env, args in [
    (
        state["postgres"],
        "postgres:17",
        {
            "POSTGRES_USER": "linha",
            "POSTGRES_PASSWORD": "linha-test-only",
            "POSTGRES_DB": "linha",
        },
        [],
    ),
    (
        state["s3"],
        "linha/minio:validation-glibc",
        {
            "MINIO_ROOT_USER": "linha-test",
            "MINIO_ROOT_PASSWORD": "linha-test-only-secret",
        },
        ["server", "/data"],
    ),
]:
    command = [
        "docker",
        "run",
        "-d",
        "--name",
        name,
        "--network",
        "kind",
        "--label",
        "linha.validation=platform-cluster",
    ]
    for k, v in env.items():
        command += ["-e", k + "=" + v]
    run(*command, image, *args)
for image in [
    "linha/spark-example:platform-complete",
    "linha/server:platform-complete",
]:
    writer = subprocess.Popen(["docker", "save", image], stdout=subprocess.PIPE)
    reader = run(
        "docker",
        "exec",
        "-i",
        cluster + "-control-plane",
        "ctr",
        "-n",
        "k8s.io",
        "images",
        "import",
        "--all-platforms",
        "--digests",
        "-",
        stdin=writer.stdout,
        stdout=subprocess.DEVNULL,
    )
    writer.stdout.close()
    assert writer.wait() == 0
run(
    "kubectl",
    "--kubeconfig",
    state["kubeconfig"],
    "--context",
    state["context"],
    "create",
    "namespace",
    "linha-test",
)
run(
    "helm",
    "upgrade",
    "--install",
    "spark-operator",
    "/tmp/linha-spark-operator-v2.4.0/charts/spark-operator-chart",
    "--kubeconfig",
    state["kubeconfig"],
    "--kube-context",
    state["context"],
    "--namespace",
    "spark-operator",
    "--create-namespace",
    "--set",
    "spark.jobNamespaces={linha-test}",
    "--wait",
    "--timeout",
    "180s",
)
print("FIXTURE " + str(root / "state.json"), flush=True)
