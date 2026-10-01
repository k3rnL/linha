#!/usr/bin/env python3
"""Install/test templates only in an explicitly supplied, disposable linha-pods-* Kind fixture.
State JSON: cluster, context, kubeconfig, directory, postgres (Docker container name).
Requires Spark Operator v2.4.0 in the fixture and images linha/server:versioned-spark and linha/spark-example:validation3 in Docker.
The Spark image must already be imported into the Kind node. See docs/pod-templates.md.
"""
from decimal import Decimal
import threading
import argparse
import base64
import json
import os
import pathlib
import socket
import subprocess
import tempfile
import time
import urllib.request
import urllib.error

ROOT = pathlib.Path(__file__).resolve().parents[1]
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--state", required=True)
parser.add_argument(
    "--server-tag", default="versioned-spark", help="Local linha/server image tag"
)
args = parser.parse_args()
s = json.loads(pathlib.Path(args.state).read_text())
assert s["cluster"].startswith("linha-pods-") and s["context"] == "kind-" + s["cluster"]
assert s["postgres"].startswith(s["cluster"] + "-")
node = s["cluster"] + "-control-plane"
ns = "linha-test"
base = ["kubectl", "--kubeconfig", s["kubeconfig"], "--context", s["context"], "-n", ns]
processes, logs = [], []
resource_evidence = []
leases = {}
renew_stop = threading.Event()
run_id = str(time.time_ns())


def kube(*args):
    return subprocess.check_output(base + list(args), text=True)


def apply(*objects):
    subprocess.run(
        base + ["apply", "-f", "-"],
        input=json.dumps({"apiVersion": "v1", "kind": "List", "items": list(objects)}),
        text=True,
        check=True,
        stdout=subprocess.DEVNULL,
    )


def obj(kind, name, **body):
    return dict(apiVersion="v1", kind=kind, metadata={"name": name}, **body)


def wait(fn, description, timeout=180):
    deadline = time.monotonic() + timeout
    last = None
    while time.monotonic() < deadline:
        try:
            last = fn()
            if last:
                return last
        except (OSError, RuntimeError) as error:
            last = str(error)
        time.sleep(0.5)
    raise AssertionError(f"timeout: {description}: {last}")


def forward():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
    log = tempfile.TemporaryFile()
    logs.append(log)
    p = subprocess.Popen(
        base
        + [
            "port-forward",
            "service/templates-linha",
            f"{port}:8080",
            "--address",
            "127.0.0.1",
        ],
        stdout=log,
        stderr=log,
    )
    processes.append(p)
    time.sleep(1)
    return f"http://127.0.0.1:{port}"


def request(method, path, data=None):
    req = urllib.request.Request(
        api + path,
        method=method,
        data=None if data is None else json.dumps(data).encode(),
        headers={
            "Content-Type": "application/json",
            "X-Linha-Client-Lease": (
                leases.get(path.split("/")[3], {}).get("id", "")
                if path.startswith("/v1/contexts/")
                else ""
            ),
        },
    )
    try:
        with urllib.request.urlopen(req, timeout=10) as response:
            raw = response.read()
            out = json.loads(raw) if raw else None
            if isinstance(out, dict) and "clientLease" in out:
                leases[out["id"]] = out["clientLease"]
            return out
    except urllib.error.HTTPError as error:
        raise RuntimeError(
            f"{method} {path}: {error.code}: {error.read().decode()}"
        ) from error


def install(default):
    values["podAnnotations"] = {"linha-test/run": run_id + "-" + default}
    values["workerPodTemplates"] = {
        "driverPodTemplate": {
            "spec": {
                "containers": [
                    {
                        "name": "main",
                        "env": [{"name": "DEPLOYMENT_DEFAULT", "value": default}],
                    }
                ]
            }
        }
    }
    path = pathlib.Path(s["directory"]) / "values.json"
    path.write_text(json.dumps(values))
    subprocess.run(
        [
            "helm",
            "upgrade",
            "--install",
            "templates",
            str(ROOT / "deploy/helm/linha"),
            "--kubeconfig",
            s["kubeconfig"],
            "--kube-context",
            s["context"],
            "--namespace",
            ns,
            "-f",
            str(path),
            "--wait",
            "--timeout",
            "120s",
        ],
        check=True,
        stdout=subprocess.DEVNULL,
    )


def driver():
    pods = json.loads(
        kube(
            "get",
            "pods",
            "-l",
            "spark-role=driver,linha.io/context=" + context["id"],
            "-o",
            "json",
        )
    )["items"]
    return next(
        (
            p
            for p in pods
            if not p["metadata"].get("deletionTimestamp")
            and p.get("status", {}).get("phase") == "Running"
        ),
        None,
    )


def executor(driver_uid):
    return next(
        (
            p
            for p in json.loads(
                kube("get", "pods", "-l", "spark-role=executor", "-o", "json")
            )["items"]
            if p.get("status", {}).get("phase") == "Running"
            and any(
                r["uid"] == driver_uid for r in p["metadata"].get("ownerReferences", [])
            )
        ),
        None,
    )


def check_pod(pod, role, default="original"):
    spec = pod["spec"]
    name = pod["metadata"]["name"]
    main = next(
        c for c in spec["containers"] if c["name"] == "spark-kubernetes-" + role
    )
    settings = context["spec"]["engine"]["settings"][role + "s"]

    def quantity(value):
        value = str(value)
        for unit, multiplier in [
            ("Ki", 1024),
            ("Mi", 1024**2),
            ("Gi", 1024**3),
            ("m", Decimal(".001")),
        ]:
            if value.endswith(unit):
                return Decimal(value[: -len(unit)]) * multiplier
        return Decimal(value)

    heap = int(settings["memory"].removesuffix("m"))
    memory_mi = heap + int(settings["memoryOverhead"].removesuffix("m"))
    for field in ["requests", "limits"]:
        resources = main["resources"][field]
        assert quantity(resources["cpu"]) == quantity(
            settings["coreRequest" if field == "requests" else "coreLimit"]
        ), (role, field, resources)
        assert quantity(resources["memory"]) == memory_mi * 1024**2, (
            role,
            field,
            resources,
        )
    resource_evidence.append(
        {"pod": name, "role": role, "resources": main["resources"]}
    )
    print(
        "PASS " + role + " resources " + json.dumps(main["resources"], sort_keys=True),
        flush=True,
    )
    env = {v["name"]: v for v in main["env"]}
    assert env["APP_ENV"]["value"] == role
    assert env["APP_PASSWORD"]["valueFrom"]["secretKeyRef"] == {
        "name": "app-secret",
        "key": "password",
    }
    assert pod["metadata"]["annotations"]["example.com/template-role"] == role
    assert {"name": "app-pull"} in spec["imagePullSecrets"]
    assert spec["nodeSelector"]["kubernetes.io/os"] == "linux"
    assert any(
        v.get("configMap", {}).get("name") == "app-config" for v in spec["volumes"]
    )
    assert any(v["mountPath"] == "/etc/app" for v in main["volumeMounts"])
    assert (
        kube(
            "exec",
            name,
            "-c",
            "spark-kubernetes-" + role,
            "--",
            "cat",
            "/etc/app/message",
        ).strip()
        == "template-mounted"
    )
    assert (
        kube(
            "exec",
            name,
            "-c",
            "spark-kubernetes-" + role,
            "--",
            "printenv",
            "APP_PASSWORD",
        ).strip()
        == "fixture-only"
    )
    if role == "driver":
        assert env["DEPLOYMENT_DEFAULT"]["value"] == default
        command = kube(
            "exec", name, "-c", "spark-kubernetes-driver", "--", "ps", "-eo", "args"
        )
        assert "-Xmx" + str(heap) + "m" in command, command
        resource_evidence[-1]["heapArgument"] = "-Xmx" + str(heap) + "m"
        print(
            "PASS actual driver JVM heap "
            + str(heap)
            + "m with overhead "
            + settings["memoryOverhead"],
            flush=True,
        )


def template(role):
    return {
        "metadata": {"annotations": {"example.com/template-role": role}},
        "spec": {
            "imagePullSecrets": [{"name": "app-pull"}],
            "nodeSelector": {"kubernetes.io/os": "linux"},
            "volumes": [{"name": "app-config", "configMap": {"name": "app-config"}}],
            "initContainers": [
                {
                    "name": "check-config",
                    "image": image,
                    "command": [
                        "/bin/sh",
                        "-c",
                        'test "$(cat /etc/app/message)" = template-mounted',
                    ],
                    "volumeMounts": [
                        {
                            "name": "app-config",
                            "mountPath": "/etc/app",
                            "readOnly": True,
                        }
                    ],
                }
            ],
            "containers": [
                {
                    "name": "main",
                    "env": [
                        {"name": "APP_ENV", "value": role},
                        {
                            "name": "APP_PASSWORD",
                            "valueFrom": {
                                "secretKeyRef": {
                                    "name": "app-secret",
                                    "key": "password",
                                }
                            },
                        },
                    ],
                    "volumeMounts": [
                        {
                            "name": "app-config",
                            "mountPath": "/etc/app",
                            "readOnly": True,
                        }
                    ],
                }
            ],
        },
    }


def submit(key, size):
    return request(
        "POST",
        f'/v1/contexts/{context["id"]}/jobs',
        {
            "handler": "count-rows",
            "version": 1,
            "payload": {"size": size, "delayMillis": 0},
            "idempotencyKey": key + "-" + run_id,
        },
    )


def succeeded(job):
    state = request("GET", "/v1/jobs/" + job["id"])
    if state["state"] == "FAILED":
        raise AssertionError(state)
    return state["state"] == "SUCCEEDED"


def result(job):
    metadata = request("GET", "/v1/jobs/" + job["id"] + "/result")
    return request(
        "GET",
        "/v1/jobs/" + job["id"] + "/files/" + metadata["files"][0]["allocationId"],
    )


try:
    writer = subprocess.Popen(
        ["docker", "save", "linha/server:" + args.server_tag], stdout=subprocess.PIPE
    )
    reader = subprocess.run(
        [
            "docker",
            "exec",
            "-i",
            node,
            "ctr",
            "-n",
            "k8s.io",
            "images",
            "import",
            "--all-platforms",
            "--digests",
            "-",
        ],
        stdin=writer.stdout,
        stdout=subprocess.DEVNULL,
    )
    writer.stdout.close()
    assert writer.wait() == 0 and reader.returncode == 0
    images = subprocess.check_output(
        ["docker", "exec", node, "ctr", "-n", "k8s.io", "images", "ls"], text=True
    )
    digest = next(
        line.split()[2]
        for line in images.splitlines()
        if line.startswith("docker.io/linha/spark-example:validation3 ")
    )
    image = "linha/spark-example@" + digest
    subprocess.run(
        [
            "docker",
            "exec",
            node,
            "ctr",
            "-n",
            "k8s.io",
            "images",
            "tag",
            "--force",
            "docker.io/linha/spark-example:validation3",
            "docker.io/" + image,
        ],
        check=True,
        stdout=subprocess.DEVNULL,
    )
    apply(obj("Namespace", ns))
    subprocess.run(
        ["docker", "exec", node, "mkdir", "-p", "/var/linha-test-results"], check=True
    )
    subprocess.run(
        ["docker", "exec", node, "chown", "10001:10001", "/var/linha-test-results"],
        check=True,
    )
    apply(
        obj(
            "PersistentVolume",
            "linha-template-results",
            spec={
                "capacity": {"storage": "1Gi"},
                "accessModes": ["ReadWriteOnce"],
                "storageClassName": "",
                "hostPath": {"path": "/var/linha-test-results"},
            },
        ),
        obj(
            "PersistentVolumeClaim",
            "results",
            spec={
                "accessModes": ["ReadWriteOnce"],
                "storageClassName": "",
                "volumeName": "linha-template-results",
                "resources": {"requests": {"storage": "1Gi"}},
            },
        ),
        obj(
            "Secret",
            "linha-postgres",
            stringData={"username": "linha", "password": "linha-test-only"},
        ),
        obj("ConfigMap", "app-config", data={"message": "template-mounted"}),
        obj("Secret", "app-secret", stringData={"password": "fixture-only"}),
        obj(
            "Secret",
            "app-pull",
            type="kubernetes.io/dockerconfigjson",
            stringData={".dockerconfigjson": '{"auths":{}}'},
        ),
    )
    values = {
        "image": {
            "repository": "linha/server",
            "tag": args.server_tag,
            "pullPolicy": "Never",
        },
        "security": {"enabled": True, "oidc": {"enabled": False}},
        "rbac": {"namespaceOnly": True},
        "database": {
            "postgres": {
                "host": s["postgres"],
                "sslMode": "disable",
                "database": "linha",
            }
        },
        "allowedImages": ["index.docker.io/linha/spark-example"],
        "local": {"enabled": True, "existingClaim": "results"},
        "registrySecrets": {"allowedNames": ["app-pull"]},
    }
    install("original")
    api = forward()
    wait(lambda: request("GET", "/readyz"), "server readiness")
    submitted = {
        "name": "pod-templates-" + run_id,
        "clientId": run_id,
        "spec": {
            "image": image,
            "engine": {
                "type": "spark",
                "version": "3.5.6",
                "settings": {
                    "application": {
                        "mainClass": "linha.example.SparkMain",
                        "mainApplicationFile": "local:///opt/linha/linha-examples.jar",
                    },
                    "drivers": {
                        "minDrivers": 1,
                        "maxDrivers": 1,
                        "cores": 2,
                        "memory": "512m",
                        "memoryOverhead": "512m",
                        "cooldownSeconds": 1,
                    },
                    "executors": {
                        "instances": 1,
                        "cores": 1,
                        "memory": "512m",
                        "memoryOverhead": "512m",
                    },
                    "kubernetes": {
                        "driverPodTemplate": template("driver"),
                        "executorPodTemplate": template("executor"),
                    },
                },
            },
            "results": {"type": "local", "root": "/var/lib/linha/results"},
        },
    }
    context = request("POST", "/v1/contexts:ensure", submitted)

    def renew_loop():
        while not renew_stop.wait(20):
            for cid, lease in list(leases.items()):
                try:
                    request(
                        "POST", f'/v1/contexts/{cid}/clients/{lease["id"]}/renew', {}
                    )
                except Exception as error:
                    print("heartbeat retry: " + str(error), flush=True)

    threading.Thread(target=renew_loop, daemon=True).start()
    replica = request(
        "POST",
        f'/v1/contexts/{context["id"]}/clients',
        {"clientId": run_id + "-replica"},
    )
    request(
        "DELETE", f'/v1/contexts/{context["id"]}/clients/{context["clientLease"]["id"]}'
    )
    context = replica
    job = submit("before-restart", 1234)
    d = wait(driver, "driver")
    e = wait(lambda: executor(d["metadata"]["uid"]), "executor")
    check_pod(d, "driver")
    check_pod(e, "executor")
    wait(lambda: succeeded(job), "Spark result")
    assert result(job) == {"value": 1234}
    print(
        "PASS real driver/executor ConfigMap mounts, Secret env, pull refs, init containers, scheduling, Spark result",
        flush=True,
    )
    install("changed")
    api = forward()
    wait(lambda: request("GET", "/readyz"), "restarted server readiness")
    repeated = request("POST", "/v1/contexts:ensure", submitted)
    assert repeated["id"] != context["id"] and repeated["version"] != context["version"]
    old_context = context
    # An existing client attaches to its immutable version across a server rollout.
    context = request(
        "POST",
        f'/v1/contexts/{old_context["id"]}/clients',
        {"clientId": run_id + "-replica"},
    )
    assert context["spec"] == old_context["spec"]
    assert result(job) == {"value": 1234}
    old = d["metadata"]["uid"]
    kube("delete", "pod", d["metadata"]["name"], "--wait=false")

    def replacement():
        p = driver()
        return p if p and p["metadata"]["uid"] != old else None

    d = wait(replacement, "replacement driver")
    e = wait(lambda: executor(d["metadata"]["uid"]), "replacement executor")
    check_pod(d, "driver")
    check_pod(e, "executor")
    next_job = submit("after-restart", 42)
    wait(lambda: succeeded(next_job), "replacement Spark result")
    assert result(next_job) == {"value": 42} and result(job) == {"value": 1234}
    print(
        "PASS two-replica server rollout, stable logical name versions, old defaults preserved, driver replacement, durable results",
        flush=True,
    )
    accepted = request(
        "POST",
        f'/v1/contexts/{context["id"]}/jobs',
        {
            "handler": "count-rows",
            "version": 1,
            "payload": {"size": 77, "delayMillis": 4000},
            "idempotencyKey": "drain-" + run_id,
        },
    )
    old_id = context["id"]
    old_lease = leases.pop(old_id)
    request("DELETE", f'/v1/contexts/{old_id}/clients/{old_lease["id"]}')
    wait(
        lambda: request("GET", f"/v1/contexts/{old_id}")["state"] == "DRAINING",
        "last-client drain",
    )
    wait(lambda: succeeded(accepted), "accepted Spark work finishes during drain")
    wait(
        lambda: request("GET", f"/v1/contexts/{old_id}")["state"] == "STOPPED",
        "old version stops despite minDrivers",
    )
    assert result(accepted) == {"value": 77} and result(job) == {"value": 1234}
    context = repeated
    duplicate = submit("before-restart", 1234)
    assert duplicate["id"] == job["id"]
    print(
        "PASS last-client release drains accepted jobs and cross-version retries return the original ID",
        flush=True,
    )
    cid = context["id"]
    lease = leases.pop(cid)
    request("DELETE", f'/v1/contexts/{cid}/clients/{lease["id"]}')
    wait(
        lambda: request("GET", f"/v1/contexts/{cid}")["state"] == "STOPPED",
        "unused new version stops",
    )
    evidence = {
        "context": context["id"],
        "beforeRestart": job["id"],
        "afterRestart": next_job["id"],
        "cluster": s["cluster"],
    }
    dynamic = json.loads(json.dumps(submitted))
    dynamic["name"] = "pod-templates-dynamic-" + run_id
    dynamic_settings = dynamic["spec"]["engine"]["settings"]
    dynamic_settings["drivers"]["cores"] = 1
    dynamic_settings["executors"].update(
        {
            "cores": 2,
            "dynamicAllocation": True,
            "minExecutors": 0,
            "initialExecutors": 1,
            "maxExecutors": 1,
        }
    )
    context = request("POST", "/v1/contexts:ensure", dynamic)
    dynamic_job = submit("dynamic-resources", 123)
    d = wait(driver, "dynamic driver")
    e = wait(lambda: executor(d["metadata"]["uid"]), "dynamic executor")
    # This context was created after the defaults-changing rollout.
    check_pod(d, "driver", default="changed")
    check_pod(e, "executor")
    wait(lambda: succeeded(dynamic_job), "dynamic executor result")
    assert result(dynamic_job) == {"value": 123}
    print(
        "PASS dynamic allocation uses configured CPU and memory requests/limits",
        flush=True,
    )
    evidence.update(
        {
            "dynamicContext": context["id"],
            "dynamicJob": dynamic_job["id"],
            "resources": resource_evidence,
        }
    )
    expired = context["id"]
    leases.pop(expired)
    print(
        "Waiting for the final client heartbeat to expire (90 seconds)...", flush=True
    )
    wait(
        lambda: request("GET", f"/v1/contexts/{expired}")["state"] == "STOPPED",
        "missing client expires and stops backend",
        timeout=150,
    )
    assert result(dynamic_job) == {"value": 123}
    for _ in range(5):
        time.sleep(1)
        assert request("GET", f"/v1/contexts/{expired}")["state"] == "STOPPED"
    print(
        "PASS client disappearance stops the backend and retained results remain readable",
        flush=True,
    )
    (pathlib.Path(s["directory"]) / "evidence.json").write_text(
        json.dumps(evidence, indent=2) + "\n"
    )
finally:
    renew_stop.set()
    for p in processes:
        p.terminate()
    for p in processes:
        p.wait(timeout=10)
    for log in logs:
        log.close()
