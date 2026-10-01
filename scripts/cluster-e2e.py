#!/usr/bin/env python3
"""Validate an explicitly named disposable Kind installation. Never targets the current context."""
import datetime
import hashlib
import hmac
import json
import os
import socket
import subprocess
import tempfile
import time
import urllib.request
import urllib.error

CONTEXT = "kind-linha-validation"
NS = "linha-test"


def kube(*args):
    return subprocess.check_output(
        ["kubectl", "--context", CONTEXT, "-n", NS, *args], text=True
    )


def port():
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


processes = []
logs = []


def forward(service, target):
    p = port()
    log = tempfile.TemporaryFile()
    logs.append(log)
    processes.append(
        subprocess.Popen(
            [
                "kubectl",
                "--context",
                CONTEXT,
                "-n",
                NS,
                "port-forward",
                "service/" + service,
                f"{p}:{target}",
                "--address",
                "127.0.0.1",
            ],
            stdout=log,
            stderr=log,
        )
    )
    time.sleep(1)
    return f"http://127.0.0.1:{p}"


def request(base, method, path, value=None, headers=None):
    data = None if value is None else json.dumps(value).encode()
    req = urllib.request.Request(
        base + path,
        data=data,
        method=method,
        headers=headers or {"Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=10) as r:
            raw = r.read()
            return (
                json.loads(raw)
                if r.headers.get("Content-Type", "").startswith("application/json")
                else raw
            )
    except urllib.error.HTTPError as e:
        raise RuntimeError(f"{method} {path}: {e.code} {e.read().decode()}") from e


def wait(fn, description, timeout=180):
    deadline = time.monotonic() + timeout
    last = None
    while time.monotonic() < deadline:
        try:
            last = fn()
            if last:
                return last
        except (OSError, RuntimeError) as e:
            last = str(e)
        time.sleep(0.5)
    raise AssertionError(f"timeout: {description}; last={last}")


def create_bucket(base):
    now = datetime.datetime.now(datetime.timezone.utc)
    stamp = now.strftime("%Y%m%dT%H%M%SZ")
    date = stamp[:8]
    host = base.split("://")[1]
    body = hashlib.sha256(b"").hexdigest()
    signed = "host;x-amz-content-sha256;x-amz-date"
    canonical = (
        "PUT\n/linha-results\n\nhost:"
        + host
        + "\nx-amz-content-sha256:"
        + body
        + "\nx-amz-date:"
        + stamp
        + "\n\n"
        + signed
        + "\n"
        + body
    )
    scope = date + "/us-east-1/s3/aws4_request"
    sign = (
        "AWS4-HMAC-SHA256\n"
        + stamp
        + "\n"
        + scope
        + "\n"
        + hashlib.sha256(canonical.encode()).hexdigest()
    )
    key = b"AWS4linha-test-only-secret"
    for part in [date, "us-east-1", "s3", "aws4_request"]:
        key = hmac.new(key, part.encode(), hashlib.sha256).digest()
    signature = hmac.new(key, sign.encode(), hashlib.sha256).hexdigest()
    headers = {
        "Host": host,
        "X-Amz-Date": stamp,
        "X-Amz-Content-Sha256": body,
        "Authorization": f"AWS4-HMAC-SHA256 Credential=linha-test/{scope}, SignedHeaders={signed}, Signature={signature}",
    }
    try:
        request(base, "PUT", "/linha-results", headers=headers)
    except RuntimeError as e:
        if "BucketAlreadyOwnedByYou" not in str(e):
            raise


try:
    api = forward("validation-linha", 8080)
    s3 = forward("minio", 9000)
    wait(lambda: request(api, "GET", "/readyz"), "server readiness")
    create_bucket(s3)
    images = subprocess.check_output(
        [
            "docker",
            "exec",
            "linha-validation-control-plane",
            "ctr",
            "-n",
            "k8s.io",
            "images",
            "ls",
        ],
        text=True,
    )
    digest = next(
        line.split()[2]
        for line in images.splitlines()
        if line.startswith(
            "docker.io/linha/spark-example:"
            + os.getenv("LINHA_TEST_IMAGE_TAG", "validation")
            + " "
        )
    )
    image = "linha/spark-example@" + digest
    subprocess.run(
        [
            "docker",
            "exec",
            "linha-validation-control-plane",
            "ctr",
            "-n",
            "k8s.io",
            "images",
            "tag",
            "--force",
            "docker.io/linha/spark-example:"
            + os.getenv("LINHA_TEST_IMAGE_TAG", "validation"),
            "docker.io/" + image,
        ],
        check=True,
        capture_output=True,
    )
    suffix = str(int(time.time()))

    def ensure(name, results, minimum=1):
        spec = {
            "image": image,
            "engine": {
                "type": "spark",
                "version": "3.5.6",
                "settings": {
                    "drivers": {
                        "minDrivers": minimum,
                        "maxDrivers": 1,
                        "maxConcurrentRequestsPerDriver": 2,
                        "memoryMi": 512,
                        "cooldownSeconds": 1,
                        "idleSeconds": 5,
                    },
                    "executors": {"instances": 1, "memoryMi": 512, "cores": 2},
                },
            },
            "results": results,
        }
        return request(
            api,
            "POST",
            "/v1/contexts:ensure",
            {"name": name + "-" + suffix, "spec": spec},
        )

    c = ensure("local", {"type": "local", "root": "/var/lib/linha/results"})

    def submit(c, key, size=1000, delay=0, attempts=1):
        start = time.monotonic()
        j = request(
            api,
            "POST",
            f'/v1/contexts/{c["id"]}/jobs',
            {
                "handler": "count-rows",
                "version": 1,
                "payload": {"size": size, "delayMillis": delay},
                "idempotencyKey": key,
                "retry": {"maxAttempts": attempts, "backoffSeconds": 1},
            },
        )
        print(
            "accepted",
            j["id"],
            j["state"],
            f"{time.monotonic()-start:.3f}s",
            flush=True,
        )
        return j

    def status(j):
        return request(api, "GET", "/v1/jobs/" + j["id"])

    def state(j, wanted):
        current = status(j)
        if current["state"] == "FAILED" and wanted != "FAILED":
            raise AssertionError(current)
        return current if current["state"] == wanted else None

    slow = submit(c, "slow", delay=30000)
    fast = submit(c, "fast", size=1234, delay=1000)
    wait(lambda: state(slow, "RUNNING") and state(fast, "RUNNING"), "concurrent claims")
    request(api, "POST", "/v1/jobs/" + slow["id"] + ":cancel")
    wait(lambda: state(slow, "CANCELLED"), "targeted Spark cancellation")
    wait(lambda: state(fast, "SUCCEEDED"), "other concurrent Spark job succeeds")

    def value(j):
        metadata = request(api, "GET", "/v1/jobs/" + j["id"] + "/result")
        return request(
            api,
            "GET",
            "/v1/jobs/" + j["id"] + "/files/" + metadata["files"][0]["allocationId"],
        )

    assert value(fast) == {"value": 1234}
    print("PASS concurrent Spark jobs and scoped cancellation", flush=True)
    retry = submit(c, "retry", delay=10000, attempts=2)
    fail = submit(c, "fail", delay=10000)
    wait(
        lambda: state(retry, "RUNNING") and state(fail, "RUNNING"),
        "in-flight jobs before driver deletion",
    )
    pods = json.loads(
        kube("get", "pods", "-l", "linha.io/context=" + c["id"], "-o", "json")
    )["items"]
    old = pods[0]["metadata"]["uid"]
    kube("delete", "pod", pods[0]["metadata"]["name"], "--wait=false")
    wait(
        lambda: any(
            p["metadata"]["uid"] != old
            for p in json.loads(
                kube("get", "pods", "-l", "linha.io/context=" + c["id"], "-o", "json")
            )["items"]
        ),
        "automatic driver replacement",
    )
    wait(lambda: state(fail, "FAILED"), "default no-retry after driver loss")
    wait(lambda: state(retry, "SUCCEEDED"), "opt-in retry on replacement")
    assert status(retry)["attemptCount"] == 2
    print("PASS driver replacement and independent retry policies", flush=True)
    # Port-forward reconnects to a fresh replica after a full server rollout.
    kube("rollout", "restart", "deployment/validation-linha")
    kube("rollout", "status", "deployment/validation-linha", "--timeout=90s")
    api = forward("validation-linha", 8080)
    assert value(fast) == {"value": 1234}
    assert value(retry) == {"value": 1000}
    c3 = ensure(
        "s3",
        {
            "type": "s3",
            "destination": "validation",
            "path": {
                "version": 1,
                "template": "exports/{submittedAt:yyyy/MM/dd}/{requestId}/{attemptId}/{file}",
            },
        },
        minimum=0,
    )
    sj = submit(c3, "s3", size=42)
    wait(lambda: state(sj, "SUCCEEDED"), "S3 result after scale from zero")
    assert value(sj) == {"value": 42}
    wait(
        lambda: not json.loads(
            kube("get", "pods", "-l", "linha.io/context=" + c3["id"], "-o", "json")
        )["items"],
        "idle scale to zero",
        timeout=90,
    )
    assert value(sj) == {"value": 42}
    print(
        "PASS replica rollout, durable local/S3 results, scale from/to zero", flush=True
    )
    with open("/tmp/linha-cluster-evidence.json", "w") as f:
        json.dump(
            {
                "localContext": c["id"],
                "s3Context": c3["id"],
                "completed": fast["id"],
                "retried": retry["id"],
                "s3": sj["id"],
            },
            f,
        )
finally:
    for p in processes:
        p.terminate()
    for p in processes:
        p.wait(timeout=10)
    for f in logs:
        f.close()
