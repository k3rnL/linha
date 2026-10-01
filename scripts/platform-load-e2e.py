#!/usr/bin/env python3
"""Load/restart/result checks for the fixture created by platform-cluster-e2e.py."""
import json
import pathlib
import socket
import subprocess
import tempfile
import threading
import time
import urllib.request
import urllib.error

ROOT = pathlib.Path(__file__).resolve().parents[1]
s = json.loads(pathlib.Path("/tmp/linha-platform-cluster-state.json").read_text())
assert (
    s["cluster"].startswith("linha-platform-")
    and s["context"] == "kind-" + s["cluster"]
)
base = [
    "kubectl",
    "--kubeconfig",
    s["kubeconfig"],
    "--context",
    s["context"],
    "-n",
    "linha-test",
]
node = s["cluster"] + "-control-plane"
work = pathlib.Path(s["directory"])


def kube(*a):
    return subprocess.check_output(base + list(a), text=True)


def apply(*items):
    subprocess.run(
        base + ["apply", "-f", "-"],
        input=json.dumps({"apiVersion": "v1", "kind": "List", "items": items}),
        text=True,
        check=True,
        stdout=subprocess.DEVNULL,
    )


def obj(kind, name, **kw):
    return dict(apiVersion="v1", kind=kind, metadata={"name": name}, **kw)


def wait(fn, label, seconds=240):
    end = time.monotonic() + seconds
    last = None
    while time.monotonic() < end:
        try:
            last = fn()
            if last:
                return last
        except (OSError, RuntimeError) as e:
            last = str(e)
        time.sleep(0.5)
    raise AssertionError(f"timeout {label}: {last}")


procs = []
leases = {}
stop = threading.Event()
samples = []
evidence = {}
api = ""


def forward(target, remote):
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
    log = open(work / ("forward-" + str(port) + ".log"), "w")
    p = subprocess.Popen(
        base + ["port-forward", target, f"{port}:{remote}", "--address", "127.0.0.1"],
        stdout=log,
        stderr=log,
    )
    procs.append((p, log))
    time.sleep(1)
    return f"http://127.0.0.1:{port}"


def call(method, path, data=None, url=None):
    headers = {"Content-Type": "application/json"}
    if path.startswith("/v1/contexts/"):
        headers["X-Linha-Client-Lease"] = leases.get(path.split("/")[3], "")
    req = urllib.request.Request(
        (url or api) + path,
        method=method,
        data=None if data is None else json.dumps(data).encode(),
        headers=headers,
    )
    try:
        with urllib.request.urlopen(req, timeout=12) as r:
            raw = r.read()
            out = (
                json.loads(raw)
                if raw
                and r.headers.get("Content-Type", "").startswith("application/json")
                else raw
            )
            if isinstance(out, dict) and "clientLease" in out:
                leases[out["id"]] = out["clientLease"]["id"]
            return out
    except urllib.error.HTTPError as e:
        raise RuntimeError(f"{method} {path}: {e.code}: {e.read().decode()}") from e


def renew():
    while not stop.wait(15):
        for cid, lease in list(leases.items()):
            try:
                call("POST", f"/v1/contexts/{cid}/clients/{lease}/renew", {})
            except Exception:
                pass


def pods(role, cid=None):
    selector = "spark-role=" + role + (",linha.io/context=" + cid if cid else "")
    return [
        p
        for p in json.loads(kube("get", "pods", "-l", selector, "-o", "json"))["items"]
        if not p["metadata"].get("deletionTimestamp")
        and p.get("status", {}).get("phase") in ["Pending", "Running"]
    ]


def status(j):
    return call("GET", "/v1/jobs/" + j["id"])


def success(j):
    x = status(j)
    if x["state"] == "FAILED":
        raise AssertionError(x)
    return x if x["state"] == "SUCCEEDED" else None


def submit(c, key, handler="count-rows", payload=None):
    start = time.monotonic()
    j = call(
        "POST",
        f"/v1/contexts/{c['id']}/jobs",
        {
            "handler": handler,
            "version": 1,
            "payload": payload
            or {"size": 1000, "delayMillis": 8000, "cacheAndBroadcast": True},
            "idempotencyKey": key,
        },
    )
    evidence.setdefault("acceptanceSeconds", []).append(time.monotonic() - start)
    return j


try:
    subprocess.run(
        ["docker", "exec", node, "mkdir", "-p", "/var/linha-platform-results"],
        check=True,
    )
    subprocess.run(
        ["docker", "exec", node, "chown", "10001:10001", "/var/linha-platform-results"],
        check=True,
    )
    apply(
        obj(
            "PersistentVolume",
            "linha-platform-results",
            spec={
                "capacity": {"storage": "1Gi"},
                "accessModes": ["ReadWriteMany"],
                "storageClassName": "",
                "hostPath": {"path": "/var/linha-platform-results"},
            },
        ),
        obj(
            "PersistentVolumeClaim",
            "results",
            spec={
                "accessModes": ["ReadWriteMany"],
                "storageClassName": "",
                "volumeName": "linha-platform-results",
                "resources": {"requests": {"storage": "1Gi"}},
            },
        ),
        obj(
            "Secret",
            "linha-postgres",
            stringData={"username": "linha", "password": "linha-test-only"},
        ),
    )
    bucket = "linha-platform"
    # Create the owned MinIO bucket via cached connector jars; no extra image/tool download.
    source = work / "Bucket.java"
    source.write_text(
        'import com.amazonaws.services.s3.*;import com.amazonaws.auth.*;import com.amazonaws.client.builder.AwsClientBuilder;public class Bucket{public static void main(String[] a){AmazonS3 s=AmazonS3ClientBuilder.standard().withEndpointConfiguration(new AwsClientBuilder.EndpointConfiguration(a[0],"us-east-1")).withPathStyleAccessEnabled(true).withCredentials(new AWSStaticCredentialsProvider(new BasicAWSCredentials("linha-test","linha-test-only-secret"))).build();if(!s.doesBucketExistV2(a[1]))s.createBucket(a[1]);s.shutdown();}}'
    )
    subprocess.run(
        [
            "docker",
            "run",
            "--rm",
            "--network",
            "kind",
            "--user",
            "0",
            "-v",
            str(work) + ":/work",
            "--entrypoint",
            "bash",
            "linha/spark-example:platform-complete",
            "-c",
            'javac -cp "/opt/spark/jars/*" /work/Bucket.java && java -cp "/opt/spark/jars/*:/work" Bucket http://'
            + s["s3"]
            + ":9000 "
            + bucket,
        ],
        check=True,
    )
    destination = {
        "bucket": bucket,
        "prefix": "jobs",
        "endpoint": "http://" + s["s3"] + ":9000",
        "stsEndpoint": "http://" + s["s3"] + ":9000",
        "datasetRoleArn": "arn:aws:iam::123456789012:role/linha-dataset",
        "region": "us-east-1",
        "pathStyle": True,
    }
    apply(
        obj(
            "Secret",
            "linha-s3",
            stringData={"destinations": json.dumps({"test": destination})},
        )
    )
    values = {
        "image": {
            "repository": "linha/server",
            "tag": "platform-complete",
            "pullPolicy": "Never",
        },
        "security": {"enabled": True, "oidc": {"enabled": False}},
        "database": {"postgres": {"host": s["postgres"], "sslMode": "disable"}},
        "allowedImages": ["index.docker.io/linha/spark-example"],
        "local": {"enabled": True, "existingClaim": "results"},
        "s3": {"enabled": True},
        "workerVolumes": [
            {"name": "results", "persistentVolumeClaim": {"claimName": "results"}}
        ],
        "workerMounts": [{"name": "results", "mountPath": "/var/lib/linha/results"}],
        "extraEnv": [
            {"name": "AWS_ACCESS_KEY_ID", "value": "linha-test"},
            {"name": "AWS_SECRET_ACCESS_KEY", "value": "linha-test-only-secret"},
        ],
    }
    valuesFile = work / "values.json"
    valuesFile.write_text(json.dumps(values))
    subprocess.run(
        [
            "helm",
            "upgrade",
            "--install",
            "platform",
            str(ROOT / "deploy/helm/linha"),
            "--kubeconfig",
            s["kubeconfig"],
            "--kube-context",
            s["context"],
            "-n",
            "linha-test",
            "-f",
            str(valuesFile),
            "--wait",
            "--timeout",
            "180s",
        ],
        check=True,
    )
    api = forward("service/platform-linha", 8080)
    wait(lambda: call("GET", "/readyz"), "server")
    threading.Thread(target=renew, daemon=True).start()
    images = subprocess.check_output(
        ["docker", "exec", node, "ctr", "-n", "k8s.io", "images", "ls"], text=True
    )
    digest = next(
        l.split()[2]
        for l in images.splitlines()
        if l.startswith("docker.io/linha/spark-example:platform-complete ")
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
    settings = {
        "application": {
            "mainClass": "linha.example.SparkMain",
            "mainApplicationFile": "local:///opt/linha/linha-examples.jar",
        },
        "ui": {"enabled": True, "port": 4040},
        "drivers": {
            "minDrivers": 0,
            "maxDrivers": 2,
            "maxConcurrentRequestsPerDriver": 1,
            "memory": "512m",
            "memoryOverhead": "384m",
            "cores": 1,
            "queueThreshold": 1,
            "waitSeconds": 1,
            "cooldownSeconds": 1,
            "idleSeconds": 120,
        },
        "executors": {
            "dynamicAllocation": True,
            "minExecutors": 0,
            "initialExecutors": 0,
            "maxExecutors": 2,
            "cores": 1,
            "memory": "512m",
            "memoryOverhead": "384m",
            "executorIdleTimeout": "5s",
            "cachedExecutorIdleTimeout": "8s",
            "shuffleTrackingTimeout": "8s",
        },
    }

    def ensure(name, policy):
        return call(
            "POST",
            "/v1/contexts:ensure",
            {
                "name": name + "-" + str(time.time_ns()),
                "spec": {
                    "image": image,
                    "engine": {
                        "type": "spark",
                        "version": "3.5.6",
                        "settings": settings,
                    },
                    "results": policy,
                },
            },
        )

    c = ensure("load", {"type": "local", "root": "/var/lib/linha/results"})
    start = time.monotonic()
    jobs = [submit(c, "load-" + str(i)) for i in range(4)]

    def finished():
        states = [status(j) for j in jobs]
        sample = {
            "seconds": round(time.monotonic() - start, 2),
            "drivers": len(pods("driver", c["id"])),
            "executors": len(pods("executor")),
            "running": sum(j["state"] == "RUNNING" for j in states),
        }
        samples.append(sample)
        for j in states:
            if j["state"] == "FAILED":
                raise AssertionError(j)
        return states if all(j["state"] == "SUCCEEDED" for j in states) else None

    final = wait(finished, "four Spark load jobs", 360)
    assert (
        max(x["drivers"] for x in samples) == 2
        and max(x["executors"] for x in samples) >= 3
        and max(x["running"] for x in samples) == 2
    ), samples
    evidence["loadSamples"] = samples
    from datetime import datetime

    stamp = lambda v: datetime.fromisoformat(v.replace("Z", "+00:00"))
    evidence["queueWaitSeconds"] = [
        (
            stamp(
                call("GET", "/v1/jobs/" + j["id"] + "/attempts")["items"][0][
                    "startedAt"
                ]
            )
            - stamp(j["submittedAt"])
        ).total_seconds()
        for j in final
    ]
    (work / "evidence.json").write_text(json.dumps(evidence, indent=2))
    print(
        "PASS queue-driven driver scale 0 -> 2; executor growth across both drivers; two requests run concurrently",
        flush=True,
    )
    ui = forward("pod/" + pods("driver", c["id"])[0]["metadata"]["name"], 4040)
    apps = call("GET", "/api/v1/applications", url=ui)
    assert apps
    cached = call(
        "GET", "/api/v1/applications/" + apps[0]["id"] + "/storage/rdd", url=ui
    )
    assert cached == [], cached
    evidence["retainedRDDs"] = len(cached)
    shrink = time.monotonic()
    wait(lambda: len(pods("executor")) == 0, "idle executor retirement", 90)
    evidence["executorShrinkSeconds"] = round(time.monotonic() - shrink, 2)
    assert len(pods("driver", c["id"])) == 2
    print(
        "PASS Spark UI responds; request-owned RDD cache cleared; executors shrink to zero while drivers remain",
        flush=True,
    )
    retained = []
    for provider in ["local", "s3"]:
        context = (
            c
            if provider == "local"
            else ensure("s3", {"type": "s3", "destination": "test"})
        )
        job = submit(
            context,
            "dataset",
            handler="export-rows",
            payload={"size": 100, "fail": False},
        )
        wait(lambda: success(job), provider + " distributed dataset")
        page = call("GET", f"/v1/jobs/{job['id']}/parts?limit=1")
        part = page["items"][0]["path"]
        path = f"/v1/jobs/{job['id']}/part?path=" + urllib.parse.quote(part, safe="")
        assert call("GET", path)[:4] == b"PAR1"
        retained.append(path)
        print(
            "PASS Kubernetes "
            + provider
            + " partitioned Parquet with projected worker identity",
            flush=True,
        )
    # Scoped raw handler in the same Spark image.
    raw = submit(c, "raw", handler="raw-count", payload={"size": 7})
    wait(lambda: success(raw), "raw Spark handler")
    meta = call("GET", "/v1/jobs/" + raw["id"] + "/result")
    assert call(
        "GET", "/v1/jobs/" + raw["id"] + "/files/" + meta["files"][0]["allocationId"]
    ) == {"value": 7}
    kube("rollout", "restart", "deployment/platform-linha")
    kube("rollout", "status", "deployment/platform-linha", "--timeout=120s")
    api = forward("service/platform-linha", 8080)
    for path in retained:
        assert call("GET", path)[:4] == b"PAR1"
    print(
        "PASS two API replicas replaced; local/S3 dataset retrieval survives",
        flush=True,
    )
    subprocess.run(
        ["docker", "restart", s["postgres"], s["s3"]],
        check=True,
        stdout=subprocess.DEVNULL,
    )
    wait(lambda: call("GET", "/readyz"), "database restart")
    for path in retained:
        wait(lambda: call("GET", path)[:4] == b"PAR1", "storage restart retrieval")
    print(
        "PASS PostgreSQL and S3 container restarts preserve dataset metadata and bytes",
        flush=True,
    )
    # Remove result-directory access on this fixture only, then restore it.
    subprocess.run(
        ["docker", "exec", node, "chmod", "000", "/var/linha-platform-results"],
        check=True,
    )
    try:

        def unavailable():
            try:
                call("GET", "/readyz")
                return False
            except RuntimeError as e:
                return "503" in str(e)

        wait(unavailable, "missing result mount readiness", 15)
    finally:
        subprocess.run(
            ["docker", "exec", node, "chmod", "755", "/var/linha-platform-results"],
            check=True,
        )
    wait(lambda: call("GET", "/readyz"), "restored result mount")
    print(
        "PASS inaccessible local result mount removes server readiness and recovers after repair",
        flush=True,
    )
    idleStart = time.monotonic()
    wait(lambda: not pods("driver"), "idle driver scale-down with live clients", 180)
    evidence["driverIdleShrinkWaitSeconds"] = round(time.monotonic() - idleStart, 2)
    print(
        "PASS idle drivers scale back to zero with clients still attached", flush=True
    )
    for cid, lease in list(leases.items()):
        leases.pop(cid)
        call("DELETE", f"/v1/contexts/{cid}/clients/{lease}")
    wait(lambda: not pods("driver"), "last-client driver shutdown", 120)
    for path in retained:
        assert call("GET", path)[:4] == b"PAR1"
    print(
        "PASS all drivers stop after last client release; retained datasets remain readable",
        flush=True,
    )
    evidence["retainedPaths"] = retained
    evidence["cluster"] = s["cluster"]
    (work / "evidence.json").write_text(json.dumps(evidence, indent=2))
    print("EVIDENCE " + str(work / "evidence.json"), flush=True)
finally:
    for cid, lease in list(leases.items()):
        try:
            call("DELETE", f"/v1/contexts/{cid}/clients/{lease}")
        except Exception:
            pass
    stop.set()
    for p, log in procs:
        p.terminate()
        p.wait(timeout=10)
        log.close()
