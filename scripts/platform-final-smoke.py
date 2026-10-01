#!/usr/bin/env python3
"""Verify final local image builds and retained results in the owned cluster."""
import json
import pathlib
import socket
import subprocess
import time
import urllib.request
import urllib.error

root = pathlib.Path(__file__).resolve().parents[1]
s = json.loads(pathlib.Path("/tmp/linha-platform-cluster-state.json").read_text())
assert s["cluster"].startswith("linha-platform-")
node = s["cluster"] + "-control-plane"
work = pathlib.Path(s["directory"])
base = [
    "kubectl",
    "--kubeconfig",
    s["kubeconfig"],
    "--context",
    s["context"],
    "-n",
    "linha-test",
]
for image in [
    "linha/server:platform-complete",
    "linha/spark-example:platform-complete",
]:
    p = subprocess.Popen(["docker", "save", image], stdout=subprocess.PIPE)
    subprocess.run(
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
        stdin=p.stdout,
        stdout=subprocess.DEVNULL,
        check=True,
    )
    p.stdout.close()
    assert p.wait() == 0
values = json.loads((work / "values.json").read_text())
values["podAnnotations"] = {"linha.io/acceptance-run": str(time.time_ns())}
(work / "final-values.json").write_text(json.dumps(values))
subprocess.run(
    [
        "helm",
        "upgrade",
        "platform",
        str(root / "deploy/helm/linha"),
        "--kubeconfig",
        s["kubeconfig"],
        "--kube-context",
        s["context"],
        "-n",
        "linha-test",
        "-f",
        str(work / "final-values.json"),
        "--wait",
        "--timeout",
        "120s",
    ],
    check=True,
)
with socket.socket() as sock:
    sock.bind(("127.0.0.1", 0))
    port = sock.getsockname()[1]
log = open(work / "final-forward.log", "w")
p = subprocess.Popen(
    base
    + [
        "port-forward",
        "service/platform-linha",
        f"{port}:8080",
        "--address",
        "127.0.0.1",
    ],
    stdout=log,
    stderr=log,
)
time.sleep(1)
url = f"http://127.0.0.1:{port}"
lease = ""
c = None


def call(method, path, body=None):
    r = urllib.request.Request(
        url + path,
        method=method,
        headers={"Content-Type": "application/json", "X-Linha-Client-Lease": lease},
        data=None if body is None else json.dumps(body).encode(),
    )
    with urllib.request.urlopen(r, timeout=15) as response:
        raw = response.read()
        return (
            json.loads(raw)
            if raw
            and response.headers.get("Content-Type", "").startswith("application/json")
            else raw
        )


try:
    evidence = json.loads((work / "evidence.json").read_text())
    for path in evidence["retainedPaths"]:
        assert call("GET", path)[:4] == b"PAR1"
    print(
        "PASS final server rollout retrieves retained local/S3 datasets after database/S3 restart and all drivers stopped",
        flush=True,
    )
    images = subprocess.check_output(
        ["docker", "exec", node, "ctr", "-n", "k8s.io", "images", "ls"], text=True
    )
    digest = next(
        x.split()[2]
        for x in images.splitlines()
        if x.startswith("docker.io/linha/spark-example:platform-complete ")
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
            "docker.io/linha/spark-example:platform-complete",
            "docker.io/" + image,
        ],
        check=True,
        stdout=subprocess.DEVNULL,
    )
    spec = {
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
                    "cores": 1,
                    "memory": "512m",
                    "memoryOverhead": "384m",
                },
                "executors": {
                    "instances": 1,
                    "cores": 1,
                    "memory": "512m",
                    "memoryOverhead": "384m",
                },
            },
        },
        "results": {"type": "s3", "destination": "test"},
    }
    c = call(
        "POST",
        "/v1/contexts:ensure",
        {"name": "final-" + str(time.time_ns()), "spec": spec},
    )
    lease = c["clientLease"]["id"]
    j = call(
        "POST",
        f"/v1/contexts/{c['id']}/jobs",
        {
            "handler": "count-rows",
            "version": 1,
            "payload": {"size": 17},
            "idempotencyKey": "final-build",
        },
    )
    until = time.monotonic() + 150
    renew = time.monotonic() + 20
    while time.monotonic() < until:
        current = call("GET", "/v1/jobs/" + j["id"])
        assert current["state"] != "FAILED", current
        if current["state"] == "SUCCEEDED":
            break
        if time.monotonic() > renew:
            call("POST", f"/v1/contexts/{c['id']}/clients/{lease}/renew", {})
            renew = time.monotonic() + 20
        time.sleep(0.5)
    else:
        raise AssertionError("final image did not finish")
    result = call("GET", "/v1/jobs/" + j["id"] + "/result")
    assert call(
        "GET", "/v1/jobs/" + j["id"] + "/files/" + result["files"][0]["allocationId"]
    ) == {"value": 17}
    print(
        "PASS final worker image handles legacy raw arguments and persists S3 result through SparkApplication",
        flush=True,
    )
    for name in [
        "linha/server:platform-complete",
        "linha/spark-example:platform-complete",
    ]:
        identity = subprocess.check_output(
            ["docker", "image", "inspect", name, "--format", "{{.Id}}"], text=True
        ).strip()
        print(name + " " + identity, flush=True)
finally:
    if c:
        try:
            call("DELETE", f"/v1/contexts/{c['id']}/clients/{lease}")
        except Exception:
            pass
    p.terminate()
    p.wait(timeout=10)
    log.close()
