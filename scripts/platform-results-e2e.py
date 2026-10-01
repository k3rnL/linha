#!/usr/bin/env python3
"""Local isolated Go/PostgreSQL/Spark/S3 acceptance. Requires release fixture state.
Uses only owned localhost test containers; no Kubernetes context is accessed.
"""
import json
import os
import pathlib
import subprocess
import tempfile
import time
import urllib.request
import urllib.error
import socket
import threading
import shutil

ROOT = pathlib.Path(__file__).resolve().parents[1]
state = json.loads(pathlib.Path("/tmp/linha-release-fixtures.json").read_text())
assert state[0]["name"] == "linha-release-pg" and state[1]["name"] == "linha-release-s3"
work = pathlib.Path(tempfile.mkdtemp(prefix="linha-release-e2e-"))
work.chmod(0o777)
results = work / "results"
results.mkdir(mode=0o777)
logs = []
processes = []
containers = []
leases = {}
stop = threading.Event()
with socket.socket() as s:
    s.bind(("127.0.0.1", 0))
    port = s.getsockname()[1]
url = f"http://127.0.0.1:{port}"
bucket = "linha-platform-" + str(time.time_ns())


# Create the test bucket via a short Go test helper is unnecessary: MinIO's mc image
# is not required; use the cached AWS Java SDK through Spark's connector jars below.
def call(method, path, data=None, worker=None, expected=None):
    headers = {"Content-Type": "application/json"}
    if path.startswith("/v1/contexts/") and "/jobs" in path:
        headers["X-Linha-Client-Lease"] = leases[path.split("/")[3]]
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
        with urllib.request.urlopen(req, timeout=45) as r:
            raw = r.read()
            return (
                json.loads(raw)
                if raw
                and r.headers.get("Content-Type", "").startswith("application/json")
                else raw
            )
    except urllib.error.HTTPError as e:
        body = e.read().decode()
        if expected and e.code == expected:
            return json.loads(body)
        raise AssertionError(f"{method} {path}: {e.code}: {body}")


def wait(fn, description, seconds=180):
    end = time.monotonic() + seconds
    last = None
    while time.monotonic() < end:
        try:
            last = fn()
            if last:
                return last
        except (OSError, AssertionError) as e:
            last = str(e)
        time.sleep(0.3)
    raise AssertionError(f"timeout {description}: {last}")


env = os.environ.copy()
env.update(
    {
        "LINHA_DATABASE_URL": f"postgres://postgres:linha-release-only@127.0.0.1:{state[0]['port']}/linha?sslmode=disable",
        "LINHA_SECURITY_ENABLED": "false",
        "LINHA_ENABLE_FAKE_ENGINE": "true",
        "LINHA_LOCAL_ROOT": str(results),
        "LINHA_LISTEN": f"127.0.0.1:{port}",
        "AWS_ACCESS_KEY_ID": "linha-test",
        "AWS_SECRET_ACCESS_KEY": "linha-test-only-secret",
        "LINHA_S3_DESTINATIONS": json.dumps(
            {
                "test": {
                    "bucket": bucket,
                    "prefix": "exports",
                    "endpoint": f"http://127.0.0.1:{state[1]['port']}",
                    "stsEndpoint": f"http://127.0.0.1:{state[1]['port']}",
                    "datasetRoleArn": "arn:aws:iam::123456789012:role/linha-dataset",
                    "region": "us-east-1",
                    "pathStyle": True,
                }
            }
        ),
    }
)


def start():
    log = open(work / f"server-{len(processes)}.log", "w")
    logs.append(log)
    p = subprocess.Popen(
        [str(ROOT / "bin/linha-server")], env=env, stdout=log, stderr=log
    )
    processes.append(p)
    wait(lambda: call("GET", "/readyz"), "server ready")
    return p


def renew():
    while not stop.wait(15):
        for context, lease in list(leases.items()):
            try:
                call("POST", f"/v1/contexts/{context}/clients/{lease}/renew", {})
            except Exception:
                pass


def worker(c, provider):
    identity = call("POST", f"/v1/contexts/{c['id']}/dev-workers", {})["identity"]
    name = "linha-release-worker-" + provider
    containers.append(name)
    args = [
        "docker",
        "run",
        "-d",
        "--name",
        name,
        "--network",
        "host",
        "--user",
        "0",
        "-v",
        f"{work}:{work}",
        "-e",
        f"LINHA_SERVER_URL={url}",
        "-e",
        f"LINHA_CONTEXT_ID={c['id']}",
        "-e",
        f"LINHA_POD_UID={identity['id']}",
        "-e",
        "LINHA_SECURITY_ENABLED=false",
        "-e",
        "LINHA_CAPACITY=2",
        "-e",
        f'LINHA_SPARK_CONF={json.dumps({"spark.master":"local[4]","spark.app.name":"Linha release","spark.ui.enabled":"false","spark.driver.host":"127.0.0.1","spark.sql.shuffle.partitions":"4"})}',
        "linha/spark-example:platform-complete",
        "/opt/spark/bin/spark-submit",
        "--class",
        "linha.example.SparkMain",
        "--conf",
        "spark.jars.ivy=/tmp/ivy",
        "--conf",
        "spark.driver.extraJavaOptions=-Duser.timezone=Pacific/Honolulu -Duser.home=/tmp -Duser.name=linha",
        "/opt/linha/linha-examples.jar",
    ]
    subprocess.run(args, check=True, stdout=subprocess.DEVNULL)
    return name


def worker_ready(c, name):
    x = json.loads(subprocess.check_output(["docker", "inspect", name], text=True))[0]
    if not x["State"]["Running"]:
        raise RuntimeError(
            "worker exited: "
            + subprocess.check_output(
                ["docker", "logs", name], stderr=subprocess.STDOUT, text=True
            )[-8000:]
        )
    return call("GET", "/v1/contexts/" + c["id"])["state"] == "READY"


def submit(c, key, fail=False):
    return call(
        "POST",
        f"/v1/contexts/{c['id']}/jobs",
        {
            "handler": "export-rows",
            "version": 1,
            "payload": {"size": 100, "fail": fail},
            "idempotencyKey": key,
            "retry": {"maxAttempts": 1, "backoffSeconds": 1},
        },
    )


try:
    server = start()
    threading.Thread(target=renew, daemon=True).start()
    # Bucket creation uses a small Java tool compiled against the connector already in the image.
    src = work / "Bucket.java"
    src.write_text(
        """import com.amazonaws.services.s3.*;import com.amazonaws.auth.*;import com.amazonaws.client.builder.AwsClientBuilder;public class Bucket{public static void main(String[] a){AmazonS3 s=AmazonS3ClientBuilder.standard().withEndpointConfiguration(new AwsClientBuilder.EndpointConfiguration(a[0],"us-east-1")).withPathStyleAccessEnabled(true).withCredentials(new AWSStaticCredentialsProvider(new BasicAWSCredentials("linha-test","linha-test-only-secret"))).build();s.createBucket(a[1]);s.shutdown();}}"""
    )
    subprocess.run(
        [
            "docker",
            "run",
            "--rm",
            "--network",
            "host",
            "--user",
            "0",
            "-v",
            f"{work}:{work}",
            "--entrypoint",
            "/bin/bash",
            "linha/spark-example:platform-complete",
            "-c",
            f'javac -cp "/opt/spark/jars/*" {src} && java -cp "/opt/spark/jars/*:{work}" Bucket http://127.0.0.1:{state[1]["port"]} {bucket}',
        ],
        check=True,
    )
    retained = []
    for provider in ["local", "s3"]:
        policy = (
            {"type": "local", "root": str(results)}
            if provider == "local"
            else {
                "type": "s3",
                "destination": "test",
                "path": {
                    "version": 1,
                    "template": "dates/{submittedAt:yyyy/MM/dd}/{requestId}/{attemptId}/{file}",
                },
            }
        )
        c = call(
            "POST",
            "/v1/contexts:ensure",
            {
                "name": bucket + "-" + provider,
                "clientId": "acceptance",
                "spec": {
                    "image": "fixture",
                    "engine": {"type": "fake", "version": "1", "settings": {}},
                    "results": policy,
                },
            },
        )
        leases[c["id"]] = c["clientLease"]["id"]
        name = worker(c, provider)
        wait(lambda: worker_ready(c, name), "worker registration", 60)
        jobs = [submit(c, "good-a"), submit(c, "good-b"), submit(c, "failed", True)]
        for job in jobs:
            j = wait(
                lambda: (
                    x
                    if (x := call("GET", "/v1/jobs/" + job["id"]))["state"]
                    in ["SUCCEEDED", "FAILED"]
                    else None
                ),
                "job " + job["id"],
                240,
            )
            if job == jobs[-1]:
                assert j["state"] == "FAILED", j
            else:
                assert j["state"] == "SUCCEEDED", j
                metadata = call("GET", f"/v1/jobs/{j['id']}/result")
                assert metadata["dataset"]["format"] == "parquet"
                assert metadata["dataset"]["partCount"] >= 2
                parts = []
                cursor = ""
                while True:
                    page = call(
                        "GET",
                        f"/v1/jobs/{j['id']}/parts?limit=1&cursor="
                        + urllib.parse.quote(cursor, safe=""),
                    )
                    parts += page["items"]
                    cursor = page.get("nextCursor", "")
                    if not cursor:
                        break
                assert len(parts) == metadata["dataset"]["partCount"]
                for part in parts:
                    content = call(
                        "GET",
                        f"/v1/jobs/{j['id']}/part?path="
                        + urllib.parse.quote(part["path"], safe=""),
                    )
                    assert content[:4] == b"PAR1"
                retained.append((j["id"], metadata, parts[0]["path"]))
        subprocess.run(
            ["docker", "rm", "-f", name], check=True, stdout=subprocess.DEVNULL
        )
        containers.remove(name)
        print(
            "PASS",
            provider,
            "concurrent partitioned datasets, failure isolation, incremental retrieval",
            flush=True,
        )
    server.terminate()
    server.wait(15)
    server = start()
    for job, metadata, path in retained:
        assert call("GET", f"/v1/jobs/{job}/result") == metadata
        assert (
            call(
                "GET", f"/v1/jobs/{job}/part?path=" + urllib.parse.quote(path, safe="")
            )[:4]
            == b"PAR1"
        )
    print(
        "PASS dataset metadata/parts survive worker removal and server restart",
        flush=True,
    )
    print("EVIDENCE", work, flush=True)
finally:
    stop.set()
    for name in containers:
        p = subprocess.run(
            ["docker", "logs", name],
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
        )
        (work / (name + ".log")).write_text(p.stdout)
        subprocess.run(["docker", "rm", "-f", name], stdout=subprocess.DEVNULL)
    for p in processes:
        if p.poll() is None:
            p.terminate()
    for p in processes:
        try:
            p.wait(15)
        except subprocess.TimeoutExpired:
            p.kill()
    for log in logs:
        log.close()
    print("LOGS", work, flush=True)
