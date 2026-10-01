#!/usr/bin/env python3
"""Pause only the named disposable PostgreSQL fixture; exercise durable lease recovery."""
import concurrent.futures
import hashlib
import json
import os
import pathlib
import socket
import subprocess
import tempfile
import time
import urllib.request
import urllib.error

root = pathlib.Path(__file__).resolve().parents[1]
state = json.loads(pathlib.Path("/tmp/linha-release-fixtures.json").read_text())
assert state[0]["name"] == "linha-release-pg"
work = pathlib.Path(tempfile.mkdtemp(prefix="linha-fault-e2e-"))
results = work / "results"
results.mkdir()
with socket.socket() as s:
    s.bind(("127.0.0.1", 0))
    port = s.getsockname()[1]
url = f"http://127.0.0.1:{port}"
lease = ""
worker = None


def call(method, path, data=None, expected=200, timeout=8):
    headers = {"Content-Type": "application/json", "X-Linha-Client-Lease": lease}
    if worker:
        headers.update(
            {
                "X-Linha-Context-Id": worker["contextId"],
                "X-Linha-Worker-Id": worker["id"],
                "X-Linha-Worker-Incarnation": worker["incarnation"],
            }
        )
    req = urllib.request.Request(
        url + path,
        method=method,
        headers=headers,
        data=None if data is None else json.dumps(data).encode(),
    )
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            raw = r.read()
            assert r.status == expected, (r.status, raw)
            return json.loads(raw) if raw else None
    except urllib.error.HTTPError as e:
        raw = e.read()
        assert e.code == expected, (method, path, e.code, raw)
        return json.loads(raw)


def wait(fn, seconds=30):
    end = time.monotonic() + seconds
    while time.monotonic() < end:
        try:
            value = fn()
            if value:
                return value
        except (OSError, AssertionError):
            pass
        time.sleep(0.2)
    raise AssertionError("timeout")


env = dict(
    os.environ,
    LINHA_DATABASE_URL=f"postgres://postgres:linha-release-only@127.0.0.1:{state[0]['port']}/linha?sslmode=disable",
    LINHA_SECURITY_ENABLED="false",
    LINHA_ENABLE_FAKE_ENGINE="true",
    LINHA_LOCAL_ROOT=str(results),
    LINHA_LISTEN=f"127.0.0.1:{port}",
)
log = open(work / "server.log", "w")
server = subprocess.Popen(
    [str(root / "bin/linha-server")], env=env, stdout=log, stderr=log
)
paused = False
try:
    wait(lambda: call("GET", "/readyz"))
    c = call(
        "POST",
        "/v1/contexts:ensure",
        {
            "name": "fault-" + str(time.time_ns()),
            "spec": {
                "image": "fixture",
                "engine": {"type": "fake", "version": "1", "settings": {}},
                "results": {"type": "local", "root": str(results)},
            },
        },
    )
    lease = c["clientLease"]["id"]
    worker = call("POST", f"/v1/contexts/{c['id']}/dev-workers", {}, 201)["identity"]
    descriptor = {"kind": "json", "schema": "fault-result", "version": 1}
    call(
        "POST",
        "/v1/workers/register",
        dict(
            id=worker["id"],
            contextId=c["id"],
            incarnation=worker["incarnation"],
            capacity=2,
            capabilities=[{"handler": "fault", "version": 1, "result": descriptor}],
        ),
    )
    body = {
        "handler": "fault",
        "version": 1,
        "payload": {},
        "idempotencyKey": "no-retry",
    }
    jobs = [
        call("POST", f"/v1/contexts/{c['id']}/jobs", body, 202),
        call(
            "POST",
            f"/v1/contexts/{c['id']}/jobs",
            dict(
                body,
                idempotencyKey="retry",
                retry={"maxAttempts": 2, "backoffSeconds": 1},
            ),
            202,
        ),
    ]
    assignments = [
        call("POST", "/v1/workers/claim", {}),
        call("POST", "/v1/workers/claim", {}),
    ]
    updates = [
        dict(
            workerId=worker["id"],
            incarnation=worker["incarnation"],
            attemptId=a["attemptId"],
            fence=a["fence"],
        )
        for a in assignments
    ]
    allocations = [
        call(
            "POST",
            f"/v1/jobs/{a['job']['id']}/outputs",
            dict(u, name="value.json", kind="json", contentType="application/json"),
        )
        for a, u in zip(assignments, updates)
    ]
    subprocess.run(
        ["docker", "pause", state[0]["name"]], check=True, stdout=subprocess.DEVNULL
    )
    paused = True
    call("GET", "/livez")
    call("GET", "/readyz", expected=503)
    rejected = dict(body, idempotencyKey="during-outage")
    try:
        call("POST", f"/v1/contexts/{c['id']}/jobs", rejected, 202, timeout=2)
        raise AssertionError("false durable acceptance")
    except (TimeoutError, OSError):
        pass
    print(
        "PASS database outage keeps liveness and drops readiness; submission not acknowledged",
        flush=True,
    )
    # Both workers are intentionally silent for longer than the actual 30-second lease.
    time.sleep(33)
    subprocess.run(
        ["docker", "unpause", state[0]["name"]], check=True, stdout=subprocess.DEVNULL
    )
    paused = False
    wait(lambda: call("GET", "/readyz"))
    for a, u, allocation in zip(assignments, updates, allocations):
        call("POST", f"/v1/jobs/{a['job']['id']}/renew", u, 409)
        f = {
            "allocationId": allocation["id"],
            "name": "value.json",
            "size": 2,
            "sha256": hashlib.sha256(b"{}").hexdigest(),
            "contentType": "application/json",
        }
        call(
            "POST",
            f"/v1/jobs/{a['job']['id']}/complete",
            dict(u, descriptor=descriptor, files=[f]),
            409,
        )
    terminal = wait(
        lambda: (
            j
            if (j := call("GET", "/v1/jobs/" + jobs[0]["id"]))["state"] == "FAILED"
            else None
        )
    )
    assert terminal["failure"]["code"] == "ATTEMPT_LOST", terminal
    call("POST", "/v1/workers/heartbeat", {})
    retried = wait(lambda: call("POST", "/v1/workers/claim", {}))
    assert retried["job"]["id"] == jobs[1]["id"] and retried["fence"] == 2, retried
    recovered = call("POST", f"/v1/contexts/{c['id']}/jobs", rejected, 202)
    repeated = call("POST", f"/v1/contexts/{c['id']}/jobs", rejected, 202)
    assert recovered["id"] == repeated["id"]
    print(
        "PASS expired workers cannot renew or complete; default fails, opt-in retries same public ID; outage retry is idempotent",
        flush=True,
    )
finally:
    if paused:
        subprocess.run(
            ["docker", "unpause", state[0]["name"]],
            check=True,
            stdout=subprocess.DEVNULL,
        )
    server.terminate()
    try:
        server.wait(timeout=15)
    except subprocess.TimeoutExpired:
        server.kill()
        server.wait()
    log.close()
    print("EVIDENCE " + str(work), flush=True)
