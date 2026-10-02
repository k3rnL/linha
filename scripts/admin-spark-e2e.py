#!/usr/bin/env python3
import json, pathlib, subprocess, socket, time, threading, urllib.request, urllib.parse, urllib.error
import argparse

parser = argparse.ArgumentParser(
    description="Exercise real Spark observations and admin request lifecycle"
)
parser.add_argument("--state", required=True, type=pathlib.Path)
args = parser.parse_args()
s = json.loads(args.state.read_text())
assert s["cluster"].startswith("linha-ui-") and s["context"] == "kind-" + s["cluster"]
nonce = str(int(time.time()))
base = [
    "kubectl",
    "--kubeconfig",
    s["kubeconfig"],
    "--context",
    s["context"],
    "-n",
    "linha-ui-test",
]
work = pathlib.Path(s["directory"])
processes = []
stop = threading.Event()
evidence = {}


def run(*args, **kw):
    return subprocess.run(args, check=True, **kw)


def forward(resource, port):
    sock = socket.socket()
    sock.bind(("127.0.0.1", 0))
    local = sock.getsockname()[1]
    sock.close()
    p = subprocess.Popen(
        base + ["port-forward", resource, f"{local}:{port}", "--address", "127.0.0.1"],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    processes.append(p)
    time.sleep(1)
    return "http://127.0.0.1:" + str(local)


def call(method, path, body=None, url=None):
    r = urllib.request.Request(
        (url or api) + path,
        method=method,
        data=None if body is None else json.dumps(body).encode(),
        headers={"Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(r, timeout=30) as out:
            data = out.read()
            return (
                json.loads(data)
                if data and "json" in out.headers.get("Content-Type", "")
                else data
            )
    except urllib.error.HTTPError as e:
        raise AssertionError(f"{method} {path}: {e.code}: {e.read().decode()}")


def wait(f, label, timeout=180):
    end = time.monotonic() + timeout
    last = None
    while time.monotonic() < end:
        try:
            value = f()
            if value:
                return value
        except Exception as e:
            last = e
        time.sleep(1)
    raise AssertionError(f"{label}: {last}")


def resources(kind):
    return json.loads(
        subprocess.check_output(base + ["get", kind, "-o", "json"], text=True)
    )["items"]


try:
    (work / "spark-ingress.json").write_text(
        json.dumps(
            {
                "controller": {
                    "uiIngress": {
                        "enable": True,
                        "urlFormat": "{{ $appName }}.spark.fixture.example",
                    }
                }
            }
        )
    )
    # The operator owns ingress creation. Linha only discovers its existing metadata.
    run(
        "helm",
        "upgrade",
        "spark-operator",
        s["sparkChart"],
        "--kubeconfig",
        s["kubeconfig"],
        "--kube-context",
        s["context"],
        "-n",
        "spark-operator",
        "--reuse-values",
        "--set",
        "controller.uiIngress.enable=true",
        "-f",
        str(work / "spark-ingress.json"),
        "--wait",
        "--timeout",
        "120s",
        stdout=subprocess.DEVNULL,
    )
    api = forward("service/ui-linha", 8080)
    settings = {
        "application": {
            "mainClass": "linha.example.SparkMain",
            "mainApplicationFile": "local:///opt/linha/linha-examples.jar",
        },
        "ui": {"enabled": True},
        "drivers": {
            "minDrivers": 2,
            "maxDrivers": 2,
            "maxConcurrentRequestsPerDriver": 2,
            "memory": "512m",
            "memoryOverhead": "384m",
            "coreRequest": "100m",
            "coreLimit": "2",
            "cores": 1,
            "queueThreshold": 1,
            "waitSeconds": 1,
            "cooldownSeconds": 1,
            "idleSeconds": 60,
        },
        "executors": {
            "dynamicAllocation": True,
            "minExecutors": 0,
            "initialExecutors": 0,
            "maxExecutors": 2,
            "memory": "512m",
            "memoryOverhead": "384m",
            "coreRequest": "100m",
            "coreLimit": "1",
            "cores": 1,
            "executorIdleTimeout": "5s",
            "cachedExecutorIdleTimeout": "8s",
            "shuffleTrackingTimeout": "8s",
        },
    }
    c = call(
        "POST",
        "/v1/contexts:ensure",
        {
            "name": "ui-spark-acceptance",
            "clientId": "fixture",
            "spec": {
                "image": s["workerImage"],
                "engine": {"type": "spark", "version": "3.5.6", "settings": settings},
                "results": {
                    "type": "local",
                    "root": "/var/lib/linha/results",
                    "maxBytes": 1048576,
                    "retentionSeconds": 3600,
                },
            },
        },
    )
    s["contextId"] = c["id"]
    args.state.write_text(json.dumps(s))

    def renew():
        while not stop.wait(20):
            try:
                call(
                    "POST",
                    f"/v1/contexts/{c['id']}/clients/{c['clientLease']['id']}/renew",
                    {},
                )
            except Exception:
                pass

    thread = threading.Thread(target=renew, daemon=True)
    thread.start()
    wait(
        lambda: call("GET", f"/v1/admin/contexts/{c['id']}")["readyWorkers"] >= 2,
        "two registered drivers",
    )
    observations = wait(
        lambda: [
            i
            for i in call("GET", f"/v1/admin/contexts/{c['id']}/instances")["items"]
            if i["available"] and not i["stale"] and i.get("detail", {}).get("links")
        ],
        "fresh Spark observations and ingress links",
    )
    assert len(observations) == 2, observations
    evidence["ingressLinks"] = [i["detail"]["links"] for i in observations]
    print(
        "PASS two registered Spark drivers and validated operator ingress links",
        flush=True,
    )

    def submit(key, size, delay=0, source=None):
        body = {
            "handler": "count-rows",
            "version": 1,
            "payload": {"size": size, "delayMillis": delay},
            "idempotencyKey": key,
            "retry": {"maxAttempts": 2, "backoffSeconds": 1},
        }
        if source:
            body["replayedFrom"] = source
        return call("POST", f"/v1/admin/contexts/{c['id']}/jobs", body)

    jobs = [submit("ui-spark-" + nonce + "-" + str(n), 100 + n, 3000) for n in range(4)]
    observedMax = 0

    def finished():
        global observedMax
        current = call("GET", f"/v1/admin/contexts/{c['id']}/instances")["items"]
        observedMax = max(
            observedMax,
            sum(
                i.get("detail", {}).get("executorStates", {}).get("Running", 0)
                for i in current
                if i["available"] and not i["stale"]
            ),
        )
        states = [call("GET", "/v1/admin/jobs/" + j["id"])["state"] for j in jobs]
        assert not any(x in ("FAILED", "CANCELLED") for x in states), states
        return all(x == "SUCCEEDED" for x in states)

    wait(finished, "real Spark successful results")
    wait(
        lambda: not [
            p
            for p in resources("pods")
            if p["metadata"].get("labels", {}).get("spark-role") == "executor"
        ],
        "executor retirement",
        90,
    )
    wait(
        lambda: all(
            sum(i.get("detail", {}).get("executorStates", {}).values()) == 0
            for i in call("GET", f"/v1/admin/contexts/{c['id']}/instances")["items"]
            if i["state"] == "READY"
        ),
        "observations discard deleted executors",
    )
    evidence["executorMax"] = observedMax
    result = call("GET", "/v1/admin/jobs/" + jobs[0]["id"] + "/result")
    assert result["files"][0]["location"].startswith("/var/lib/linha/results/")
    preview = call("GET", "/v1/admin/jobs/" + jobs[0]["id"] + "/preview")
    assert preview["validJSON"]
    print(
        "PASS current executor counts shrink to zero; retained published result location and JSON preview",
        flush=True,
    )
    # Real cancellation of one concurrent request leaves other work and driver alive.
    slow = submit("ui-cancel-" + nonce, 500, 15000)
    other = submit("ui-other-" + nonce, 40, 15000)
    wait(
        lambda: all(
            call("GET", "/v1/admin/jobs/" + j["id"])["state"] == "RUNNING"
            for j in [slow, other]
        ),
        "concurrent requests",
    )
    cancelled = call("POST", "/v1/admin/jobs/" + slow["id"] + "/cancel", {})
    assert cancelled["state"] == "CANCELLING"
    wait(
        lambda: call("GET", "/v1/admin/jobs/" + slow["id"])["state"] == "CANCELLED",
        "cancellation ack",
    )
    wait(
        lambda: call("GET", "/v1/admin/jobs/" + other["id"])["state"] == "SUCCEEDED",
        "unrelated concurrent job success",
    )
    assert call("GET", f"/v1/admin/contexts/{c['id']}")["readyWorkers"] >= 2
    print(
        "PASS request cancellation preserves unrelated shared-engine work", flush=True
    )
    replay = submit("ui-replay-" + nonce, 321, source=jobs[0]["id"])
    wait(
        lambda: call("GET", "/v1/admin/jobs/" + replay["id"])["state"] == "SUCCEEDED",
        "edited replay",
    )
    assert (
        call("GET", "/v1/admin/jobs/" + replay["id"])["replayedFrom"] == jobs[0]["id"]
    )
    evidence["replayJob"] = replay["id"]
    evidence["sourceJob"] = jobs[0]["id"]
    evidence["allJobs"] = [j["id"] for j in jobs] + [
        slow["id"],
        other["id"],
        replay["id"],
    ]
    # Delete an owned application to verify its replacement and stale-link removal.
    old = call("GET", f"/v1/admin/contexts/{c['id']}/instances")["items"][0]
    run(
        *base,
        "delete",
        "sparkapplication",
        old["detail"]["application"],
        "--wait=false",
        stdout=subprocess.DEVNULL,
    )
    wait(
        lambda: call("GET", f"/v1/admin/contexts/{c['id']}")["readyWorkers"] >= 2
        and old["id"]
        not in [
            i["id"]
            for i in call("GET", f"/v1/admin/contexts/{c['id']}/instances")["items"]
            if i["state"] == "READY"
        ],
        "replacement driver",
    )
    oldNow = next(
        i
        for i in call("GET", f"/v1/admin/contexts/{c['id']}/instances")["items"]
        if i["id"] == old["id"]
    )
    assert not oldNow.get("detail", {}).get("links")
    print(
        "PASS lost driver replaced; retired instance has no live Spark UI link",
        flush=True,
    )
    stop.set()
    call("DELETE", f"/v1/contexts/{c['id']}/clients/{c['clientLease']['id']}")
    wait(lambda: not resources("sparkapplications"), "last-client retirement", 90)
    # Explicitly reactivate the exact stopped version without a browser lease.
    body = {
        "handler": "count-rows",
        "version": 1,
        "payload": {"size": 222},
        "idempotencyKey": "ui-reactivate-" + nonce,
        "activateIfStopped": True,
        "replayedFrom": jobs[0]["id"],
    }
    final = call("POST", f"/v1/admin/contexts/{c['id']}/jobs", body)
    wait(
        lambda: call("GET", "/v1/admin/jobs/" + final["id"])["state"] == "SUCCEEDED",
        "reactivated job",
    )
    wait(lambda: not resources("sparkapplications"), "reactivated version drains", 90)
    assert call("GET", "/v1/admin/jobs/" + jobs[0]["id"] + "/preview")["validJSON"]
    evidence["reactivatedJob"] = final["id"]
    print(
        "PASS explicit reactivation completes without client demand and drains; earlier result remains readable",
        flush=True,
    )
    (work / "spark-ui-evidence.json").write_text(json.dumps(evidence, indent=2))
    print("EVIDENCE", work / "spark-ui-evidence.json", flush=True)
finally:
    stop.set()
    for p in processes:
        p.terminate()
