#!/usr/bin/env python3
import json, pathlib, subprocess, socket, time, urllib.request, urllib.parse, urllib.error, base64, concurrent.futures
import argparse

parser = argparse.ArgumentParser(
    description="Exercise admin HA, monitoring and owned-cluster failure recovery"
)
parser.add_argument("--state", required=True, type=pathlib.Path)
parser.add_argument("--outages-only", action="store_true")
args = parser.parse_args()
s = json.loads(args.state.read_text())
assert s["cluster"].startswith("linha-ui-") and s["context"] == "kind-" + s["cluster"]
b = [
    "kubectl",
    "--kubeconfig",
    s["kubeconfig"],
    "--context",
    s["context"],
    "-n",
    "linha-ui-test",
]
procs = []
frozen = []
role = None
paused = False
ev = {}
work = pathlib.Path(s["directory"])
nonce = str(int(time.time()))
lease_id = None
lease_renewed = 0.0
c = s["contextId"]


def run(*a, **kw):
    return subprocess.run(a, check=True, **kw)


def kube(*a):
    return run(*b, *a, stdout=subprocess.DEVNULL)


def objects(kind):
    return json.loads(subprocess.check_output(b + ["get", kind, "-o", "json"]))["items"]


def forward(resource, port):
    sk = socket.socket()
    sk.bind(("127.0.0.1", 0))
    n = sk.getsockname()[1]
    sk.close()
    p = subprocess.Popen(
        b + ["port-forward", resource, f"{n}:{port}"],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    procs.append(p)
    time.sleep(0.8)
    return "http://127.0.0.1:" + str(n)


def call(method, path, body=None, url=None, headers={}):
    r = urllib.request.Request(
        (url or api) + path,
        method=method,
        data=None if body is None else json.dumps(body).encode(),
        headers={"Content-Type": "application/json", **headers},
    )
    try:
        with urllib.request.urlopen(r, timeout=12) as out:
            data = out.read()
            return (
                json.loads(data)
                if "json" in out.headers.get("Content-Type", "")
                else data
            )
    except urllib.error.HTTPError as e:
        raise AssertionError(f"{path}: {e.code} {e.read().decode()}")


def wait(fn, label, timeout=360):
    global lease_renewed
    end = time.monotonic() + timeout
    error = None
    while time.monotonic() < end:
        try:
            if lease_id and time.monotonic() - lease_renewed > 20:
                call("POST", f"/v1/contexts/{c}/clients/{lease_id}/renew", {})
                lease_renewed = time.monotonic()
            v = fn()
            if v:
                return v
        except Exception as e:
            error = e
        time.sleep(1)
    raise AssertionError(f"{label}: {error}")


def sql(query):
    return subprocess.check_output(
        [
            "docker",
            "exec",
            s["postgres"],
            "psql",
            "-U",
            "linha",
            "-d",
            "linha",
            "-Atc",
            query,
        ],
        text=True,
    ).strip()


def totals():
    return sql(
        "SELECT name||'|'||kind||'|'||bucket||'|'||labels::text||'|'||sum(value)::text FROM linha_metric_values GROUP BY name,kind,bucket,labels ORDER BY name,kind,bucket,labels"
    )


def job(key, size=50, delay=0):
    return {
        "handler": "count-rows",
        "version": 1,
        "payload": {"size": size, "delayMillis": delay},
        "idempotencyKey": key,
        "retry": {"maxAttempts": 3, "backoffSeconds": 1},
        "activateIfStopped": True,
    }


def state(j):
    return call("GET", "/v1/admin/jobs/" + j["id"])["state"]


try:
    api = forward("service/ui-linha", 8080)
    prom = forward("pod/prometheus-linha-0", 9090)
    grafana = forward("deployment/linha-deployment", 3000)

    def healthy_targets():
        targets = call("GET", "/api/v1/targets", url=prom)["data"]["activeTargets"]
        selected = [x for x in targets if x["labels"].get("service") == "ui-linha"]
        return (
            selected
            if len(selected) == 2 and all(x["health"] == "up" for x in selected)
            else None
        )

    ts = wait(healthy_targets, "two Prometheus targets")
    ev["targets"] = 2
    print("PASS ServiceMonitor discovers two distinct responding pods", flush=True)
    gh = {
        "Authorization": "Basic "
        + base64.b64encode(b"admin:linha-fixture-only").decode()
    }
    dashboard = call(
        "GET", "/api/dashboards/uid/linha-operations", url=grafana, headers=gh
    )["dashboard"]
    assert len([p for p in dashboard["panels"] if p["type"] != "row"]) == 17
    ev["dashboardPanels"] = 21
    ds = call("GET", "/api/datasources", url=grafana, headers=gh)
    if not any(x["uid"] == "prometheus" for x in ds):
        call(
            "POST",
            "/api/datasources",
            {
                "name": "Linha Prometheus",
                "type": "prometheus",
                "uid": "prometheus",
                "access": "proxy",
                "url": "http://prometheus-operated:9090",
            },
            grafana,
            gh,
        )
    run("python3", "scripts/check-observability.py", "--prometheus-url", prom)
    print(
        "PASS live Grafana Operator import and dashboard PromQL evaluations", flush=True
    )
    if not args.outages_only:
        jobs = [
            call(
                "POST",
                f"/v1/admin/contexts/{c}/jobs",
                job("ha-" + nonce + "-" + str(n), 100 + n, 10000),
            )
            for n in range(6)
        ]
        wait(
            lambda: sum(state(j) == "RUNNING" for j in jobs) >= 4,
            "four active and queued requests",
        )
        drivers = [
            p
            for p in objects("pods")
            if p["metadata"].get("labels", {}).get("linha.io/context") == c
            and p["metadata"]["labels"].get("spark-role") == "driver"
        ]
        assert len(drivers) == 2
        # Access the operator-created UI service without altering any ingress metadata.
        service = call("GET", f"/v1/admin/contexts/{c}/instances")["items"][0][
            "detail"
        ]["uiService"]
        ui = forward("service/" + service, 4040)
        assert b"Spark" in call("GET", "/", url=ui)
        ev["sparkUIReachable"] = True
        ingress = forward("service/ingress-fixture-traefik", 80)
        link = call("GET", f"/v1/admin/contexts/{c}/instances")["items"][0]["detail"][
            "links"
        ][0]["url"]
        host = urllib.parse.urlparse(link).netloc
        assert b"Spark" in wait(
            lambda: call("GET", "/jobs/", url=ingress, headers={"Host": host}),
            "operator Spark ingress",
        )
        ev["sparkIngressReachable"] = True
        print("PASS operator-owned Spark ingress reaches the UI", flush=True)
        for pod in drivers:
            name = pod["metadata"]["name"]
            ps = subprocess.check_output(
                b + ["exec", name, "--", "ps", "-eo", "pid,comm"], text=True
            )
            pid = next(
                line.split()[0]
                for line in ps.splitlines()
                if line.split()[-1] == "java"
            )
            kube("exec", name, "--", "sh", "-c", "kill -STOP " + pid)
            frozen.append((name, pid))
        running = next(j for j in jobs if state(j) == "RUNNING")
        cancel = call("POST", "/v1/admin/jobs/" + running["id"] + "/cancel", {})
        assert cancel["state"] == "CANCELLING"
        before = totals()
        ev["restartStates"] = sorted(set(state(j) for j in jobs))
        assert ev["restartStates"] == ["CANCELLING", "QUEUED", "RUNNING"], ev
        kube("scale", "deployment/ui-linha", "--replicas=0")
        kube(
            "wait",
            "--for=delete",
            "pods",
            "-l",
            "app.kubernetes.io/component=server",
            "--timeout=60s",
        )
        kube("scale", "deployment/ui-linha", "--replicas=2")
        kube("rollout", "status", "deployment/ui-linha", "--timeout=120s")
        api = forward("service/ui-linha", 8080)
        assert (
            totals() == before
        ), "durable accounting changed without an execution transition"
        assert sorted(set(state(j) for j in jobs)) == ev["restartStates"]
        print(
            "PASS full restart preserves queued/running/cancelling requests and every durable accounting row",
            flush=True,
        )
        for name, pid in frozen:
            kube("exec", name, "--", "sh", "-c", "kill -CONT " + pid)
        frozen = []
        wait(
            lambda: all(state(j) in ("SUCCEEDED", "CANCELLED") for j in jobs),
            "accepted work after restart",
        )
        assert state(running) == "CANCELLED"
        assert sum(state(j) == "SUCCEEDED" for j in jobs) == 5
        # Lose the accepting replica after committed headers but before reading the ID.
        pods = [
            p["metadata"]["name"]
            for p in objects("pods")
            if p["metadata"].get("labels", {}).get("app.kubernetes.io/component")
            == "server"
        ]
        a = forward("pod/" + pods[0], 8080)
        other = forward("pod/" + pods[1], 8080)
        body = job("lost-response-" + nonce, 222)
        request = urllib.request.Request(
            a + f"/v1/admin/contexts/{c}/jobs",
            data=json.dumps(body).encode(),
            headers={"Content-Type": "application/json"},
        )
        response = urllib.request.urlopen(request)
        assert response.status == 202
        response.close()
        committed = sql(
            "SELECT id FROM linha_jobs WHERE idempotency_key='"
            + body["idempotencyKey"]
            + "'"
        )
        kube("delete", "pod", pods[0], "--wait=false")
        retry = call("POST", f"/v1/admin/contexts/{c}/jobs", body, url=other)
        assert retry["id"] == committed
        assert (
            sql(
                "SELECT count(*) FROM linha_admin_audit WHERE job_id='"
                + committed
                + "'"
            )
            == "1"
        )
        wait(
            lambda: call("GET", "/v1/admin/jobs/" + committed, url=other)["state"]
            == "SUCCEEDED",
            "lost-response request",
        )
        api = forward("service/ui-linha", 8080)
        print(
            "PASS accepting-replica loss plus same-key retry returns one ID and accepted audit",
            flush=True,
        )
        kube("rollout", "status", "deployment/ui-linha", "--timeout=120s")
        time.sleep(12)
    # Retained payload failure must be attributed to storage without destroying metadata.
    source = json.loads((work / "spark-ui-evidence.json").read_text())["sourceJob"]
    result = call("GET", "/v1/admin/jobs/" + source + "/result")
    location = result["files"][0]["location"]
    relative = location.removeprefix("/var/lib/linha/results/")
    host = "/var/lib/linha-ui-results/" + relative
    run("docker", "exec", s["cluster"] + "-control-plane", "mv", host, host + ".fault")
    try:
        try:
            call("GET", "/v1/admin/jobs/" + source + "/preview")
            raise AssertionError("missing result looked available")
        except AssertionError as e:
            assert "503" in str(e), e
        assert (
            call("GET", "/v1/admin/jobs/" + source + "/result")["files"][0]["location"]
            == location
        )
    finally:
        run(
            "docker",
            "exec",
            s["cluster"] + "-control-plane",
            "mv",
            host + ".fault",
            host,
        )
    assert call("GET", "/v1/admin/jobs/" + source + "/preview")["validJSON"]
    print("PASS result-store failure/recovery preserves published metadata", flush=True)
    # Discover unready pods through the ServiceMonitor and retain local metrics.
    pod = next(
        p["metadata"]["name"]
        for p in objects("pods")
        if p["metadata"].get("labels", {}).get("app.kubernetes.io/component")
        == "server"
    )
    direct = forward("pod/" + pod, 8080)
    run("docker", "pause", s["postgres"], stdout=subprocess.DEVNULL)
    paused = True
    wait(
        lambda: 'linha_collector_success{collector="database"} 0'
        in call("GET", "/metrics", url=direct).decode(),
        "database collector failure",
        30,
    )
    body = call("GET", "/metrics", url=direct).decode()
    assert "linha_job_records{" not in body and "go_goroutines" in body
    time.sleep(12)

    def healthy_targets():
        targets = call("GET", "/api/v1/targets", url=prom)["data"]["activeTargets"]
        selected = [x for x in targets if x["labels"].get("service") == "ui-linha"]
        return (
            selected
            if len(selected) == 2 and all(x["health"] == "up" for x in selected)
            else None
        )

    ts = wait(healthy_targets, "two Prometheus targets")
    assert all(
        not any(
            x["type"] == "Ready" and x["status"] == "True"
            for x in p["status"].get("conditions", [])
        )
        for p in objects("pods")
        if p["metadata"].get("labels", {}).get("app.kubernetes.io/component")
        == "server"
    )
    ev["unreadyTargets"] = 2
    print(
        "PASS unready replicas remain scrape targets; DB outage omits shared metrics with HTTP 200",
        flush=True,
    )
    run("docker", "unpause", s["postgres"], stdout=subprocess.DEVNULL)
    paused = False
    wait(
        lambda: 'linha_collector_success{collector="database"} 1'
        in call("GET", "/metrics", url=direct).decode(),
        "database recovery",
        30,
    )
    assert call("GET", "/v1/admin/jobs/" + source + "/preview")["validJSON"]
    assert len(call("GET", "/v1/admin/jobs/" + source + "/audit")["items"]) == 1
    # Kubernetes authorization failure leaves timestamped stale observations.
    attach = call(
        "POST", f"/v1/contexts/{c}/clients", {"clientId": "observation-fault"}
    )
    lease_id = attach["clientLease"]["id"]
    lease_renewed = time.monotonic()
    wait(
        lambda: call("GET", f"/v1/admin/contexts/{c}")["readyWorkers"] >= 2
        and len(
            [
                i
                for i in call("GET", f"/v1/admin/contexts/{c}/instances")["items"]
                if i["state"] == "READY" and i["available"] and not i["stale"]
            ]
        )
        >= 2,
        "fault-check workers and fresh instances",
    )
    role = json.loads(
        subprocess.check_output(b + ["get", "role", "ui-linha", "-o", "json"])
    )
    bad = json.loads(json.dumps(role))
    bad["rules"] = [
        r
        for r in bad["rules"]
        if "pods" not in r.get("resources", [])
        and "sparkapplications" not in r.get("resources", [])
    ]
    run(
        *b,
        "replace",
        "-f",
        "-",
        input=json.dumps(bad),
        text=True,
        stdout=subprocess.DEVNULL,
    )
    wait(
        lambda: any(
            i["stale"]
            for i in call("GET", f"/v1/admin/contexts/{c}/instances")["items"]
            if i["state"] == "READY"
        ),
        "stale observation after Kubernetes denial",
        45,
    )
    # Kubernetes calls are local metrics; only the current controller may make them.
    replica_urls = [
        forward("pod/" + p["metadata"]["name"], 8080)
        for p in objects("pods")
        if p["metadata"].get("labels", {}).get("app.kubernetes.io/component")
        == "server"
    ]
    wait(
        lambda: any(
            'outcome="forbidden"' in call("GET", "/metrics", url=url).decode()
            for url in replica_urls
        ),
        "controller-replica Kubernetes error metrics",
        30,
    )
    print(
        "PASS Kubernetes denial produces bounded error metrics and stale engine observations",
        flush=True,
    )
    role.pop("status", None)
    role["metadata"].pop("resourceVersion", None)
    run(
        *b,
        "apply",
        "-f",
        "-",
        input=json.dumps(role),
        text=True,
        stdout=subprocess.DEVNULL,
    )
    role = None
    wait(
        lambda: all(
            not i["stale"]
            for i in call("GET", f"/v1/admin/contexts/{c}/instances")["items"]
            if i["state"] == "READY"
        ),
        "fresh Kubernetes recovery",
    )
    call("DELETE", f"/v1/contexts/{c}/clients/{lease_id}")
    lease_id = None
    wait(lambda: not objects("sparkapplications"), "fault check retirement", 90)
    query = 'sum(max by (namespace,linha_deployment,engine,context,state) (linha_job_records{service="ui-linha"}))'
    result = call(
        "GET", "/api/v1/query?" + urllib.parse.urlencode({"query": query}), url=prom
    )
    count = int(sql("SELECT count(*) FROM linha_jobs"))
    wait(
        lambda: int(
            float(
                call(
                    "GET",
                    "/api/v1/query?" + urllib.parse.urlencode({"query": query}),
                    url=prom,
                )["data"]["result"][0]["value"][1]
            )
        )
        == count,
        "deduplicated Prometheus total",
        30,
    )
    ev["retainedJobs"] = count
    storage = call(
        "GET",
        "/api/v1/query?"
        + urllib.parse.urlencode(
            {
                "query": 'sum(linha_storage_operations_total{service="ui-linha",outcome="error"})'
            }
        ),
        url=prom,
    )
    assert float(storage["data"]["result"][0]["value"][1]) > 0
    ev["storageErrorsAttributed"] = True
    metrics = call("GET", "/metrics", url=direct)
    run(
        "docker",
        "run",
        "--rm",
        "-i",
        "--entrypoint",
        "/bin/promtool",
        "prom/prometheus:v3.7.1",
        "check",
        "metrics",
        input=metrics,
    )
    ev["promtoolExposition"] = True
    (work / "ha-monitoring-evidence.json").write_text(json.dumps(ev, indent=2))
    print(
        "PASS HA shared Prometheus job total matches PostgreSQL; audit/results survive all restarts",
        flush=True,
    )
    print("EVIDENCE", work / "ha-monitoring-evidence.json", flush=True)
finally:
    if lease_id:
        try:
            call("DELETE", f"/v1/contexts/{c}/clients/{lease_id}")
        except Exception:
            pass
    if paused:
        run("docker", "unpause", s["postgres"], stdout=subprocess.DEVNULL)
    if role:
        role["metadata"].pop("resourceVersion", None)
        run(
            *b,
            "apply",
            "-f",
            "-",
            input=json.dumps(role),
            text=True,
            stdout=subprocess.DEVNULL,
        )
    for name, pid in frozen:
        subprocess.run(
            b + ["exec", name, "--", "sh", "-c", "kill -CONT " + pid],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
    subprocess.run(
        b + ["scale", "deployment/ui-linha", "--replicas=2"], stdout=subprocess.DEVNULL
    )
    for p in procs:
        p.terminate()
