#!/usr/bin/env python3
"""Restart only the explicitly named disposable Linha validation installation."""
import json
import socket
import subprocess
import tempfile
import time
import urllib.request

context = "kind-linha-validation"
ns = "linha-test"


def kube(*args):
    return subprocess.check_output(
        ["kubectl", "--context", context, "-n", ns, *args], text=True
    )


processes = []
logs = []


def connect():
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        port = s.getsockname()[1]
    log = tempfile.TemporaryFile()
    logs.append(log)
    p = subprocess.Popen(
        [
            "kubectl",
            "--context",
            context,
            "-n",
            ns,
            "port-forward",
            "service/validation-linha",
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


def call(method, path, body=None):
    req = urllib.request.Request(
        api + path,
        data=json.dumps(body).encode() if body is not None else None,
        method=method,
        headers={"Content-Type": "application/json"},
    )
    with urllib.request.urlopen(req, timeout=10) as r:
        return json.load(r)


def wait(fn, label, seconds=180):
    until = time.monotonic() + seconds
    while time.monotonic() < until:
        try:
            result = fn()
            if result:
                return result
        except OSError:
            pass
        time.sleep(0.5)
    raise AssertionError("timeout " + label)


try:
    evidence = json.load(open("/tmp/linha-cluster-evidence.json"))
    api = connect()
    contextId = evidence["localContext"]
    jobs = []
    for i in range(3):
        j = call(
            "POST",
            f"/v1/contexts/{contextId}/jobs",
            {
                "handler": "count-rows",
                "version": 1,
                "payload": {"size": 99 + i, "delayMillis": 10000},
                "idempotencyKey": "all-restart-" + str(time.time_ns()),
                "retry": {"maxAttempts": 2, "backoffSeconds": 1},
            },
        )
        jobs.append(j)

    def statuses():
        return [call("GET", "/v1/jobs/" + j["id"]) for j in jobs]

    before = wait(
        lambda: (
            (
                s
                if [x["state"] for x in s].count("RUNNING") == 2
                and [x["state"] for x in s].count("QUEUED") == 1
                else None
            )
            if (s := statuses())
            else None
        ),
        "running and queued requests",
    )
    cancellation = call("POST", "/v1/jobs/" + jobs[0]["id"] + ":cancel")
    assert cancellation["state"] == "CANCELLING"
    print(
        "Before restart:",
        [x["state"] for x in before],
        "; cancel:",
        cancellation["state"],
        flush=True,
    )
    kube("scale", "deployment/validation-linha", "--replicas=0")
    kube("wait", "--for=delete", "pod", "-l", "app=validation-linha", "--timeout=60s")
    kube("delete", "pods", "-l", "app.kubernetes.io/managed-by=linha", "--wait=false")
    kube("rollout", "restart", "deployment/minio")
    subprocess.run(
        ["docker", "restart", "linha-implementation-postgres"],
        check=True,
        capture_output=True,
    )
    kube("scale", "deployment/validation-linha", "--replicas=2")
    kube("rollout", "status", "deployment/validation-linha", "--timeout=90s")
    kube("rollout", "status", "deployment/minio", "--timeout=90s")
    api = connect()
    final = wait(
        lambda: (
            (
                s
                if s[0]["state"] == "CANCELLED"
                and all(x["state"] == "SUCCEEDED" for x in s[1:])
                else None
            )
            if (s := statuses())
            else None
        ),
        "restored request outcomes",
    )
    for key, expected in [("completed", 1234), ("retried", 1000), ("s3", 42)]:
        job = evidence[key]
        meta = call("GET", f"/v1/jobs/{job}/result")
        assert call(
            "GET", f"/v1/jobs/{job}/files/" + meta["files"][0]["allocationId"]
        ) == {"value": expected}
    assert final[1]["attemptCount"] == 2
    assert final[2]["attemptCount"] == 1
    print(
        "PASS full database/server/driver/S3 restart: cancellation, running retry, queued request and retained local/S3 results",
        flush=True,
    )
finally:
    for p in processes:
        p.terminate()
    for p in processes:
        p.wait(timeout=10)
    for log in logs:
        log.close()
