"""Generate the packaged dashboard from the bounded metric catalogue."""

import json
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
CATALOG = {}
for line in (ROOT / "docs/metrics.md").read_text().splitlines():
    parts = [p.strip() for p in line.split("|")]
    if len(parts) < 6 or not re.fullmatch(r"`linha_[a-z_]+`", parts[1]):
        continue
    labels = ["cluster", "namespace", "linha_deployment"]
    if re.search(r"\bC\b", parts[3]):
        labels += ["engine", "context"]
    if re.search(r"\bD\b", parts[3]):
        labels += ["provider", "destination"]
    for value in re.findall(r"`([^`]+)`", parts[3]):
        labels += value.split(",")
    CATALOG[parts[1][1:-1]] = (parts[2].split()[1], labels)
DEPLOYMENT = (
    'cluster=~"$cluster",namespace=~"$namespace",linha_deployment=~"$linha_deployment"'
)
BUSINESS = DEPLOYMENT + ',engine=~"$engine",context=~"$context"'
LOCAL = DEPLOYMENT + ',pod=~"$pod"'


def shared(name, extra="", window=None, group="", aggregate="sum"):
    base = name
    for suffix in ("_bucket", "_sum", "_count"):
        if base.removesuffix(suffix) in CATALOG:
            base = base.removesuffix(suffix)
            break
    scope, labels = CATALOG[base]
    if name.endswith("_bucket"):
        labels = labels + ["le"]
    selector = BUSINESS if "context" in labels else DEPLOYMENT
    metric = name + "{" + selector + ("," + extra if extra else "") + "}"
    dedup = f"max by ({','.join(labels)}) ({metric})"
    if window:
        fn, interval = window
        dedup = f"{fn}(({dedup})[{interval}:15s])"
    result = (
        f"{aggregate} by ({group}) ({dedup})" if group else f"{aggregate} ({dedup})"
    )
    if scope == "O":
        # Mask the entire selected total when any selected required observation is
        # incomplete. Dropping only the stale context would silently undercount.
        incomplete = (
            "sum(max by (cluster,namespace,linha_deployment,engine,context,state) (linha_engine_observations{"
            + BUSINESS
            + ',state=~"stale|missing"})) > 0'
        )
        result = f"({result}) unless on () ({incomplete})"
    return result


RATE = ("rate", "$__rate_interval")
RANGE = ("increase", "$__range")


def local(name, extra="", group="", rate=True):
    assert name in CATALOG, f"Unknown metric: {name}"
    metric = name + "{" + LOCAL + ("," + extra if extra else "") + "}"
    if rate:
        metric = f"rate({metric}[$__rate_interval])"
    return f"sum by ({group}) ({metric})" if group else f"sum ({metric})"


def quantile(name, q=0.95):
    return (
        f"histogram_quantile({q}, {shared(name + '_bucket', window=RATE, group='le')})"
    )


def mean(name):
    return (
        shared(name + "_sum", window=RATE)
        + " / "
        + shared(name + "_count", window=RATE)
    )


panels = []
current_panels = panels
x, y, height, counter = 0, 0, 0, 0


def row(title, collapsed=False):
    global x, y, height, counter, current_panels
    if x or height:
        y += height
    x, height = 0, 0
    counter += 1
    value = {
        "id": counter,
        "type": "row",
        "title": title,
        "collapsed": collapsed,
        "gridPos": {"x": 0, "y": y, "w": 24, "h": 1},
        "panels": [],
    }
    panels.append(value)
    current_panels = value["panels"] if collapsed else panels
    y += 1


def panel(
    title,
    expressions,
    unit="short",
    kind="timeseries",
    description="",
    width=12,
    decimals=None,
):
    global x, y, height, counter
    panel_height = 4 if kind == "stat" else 7
    if x + width > 24:
        x, y, height = 0, y + height, 0
    if isinstance(expressions, str):
        expressions = [(title, expressions)]
    counter += 1
    defaults = {
        "unit": unit,
        "noValue": "Not available",
        "links": [],
        "color": {"mode": "palette-classic"},
        "min": 0,
    }
    if decimals is not None:
        defaults["decimals"] = decimals
    options = {
        "legend": {"displayMode": "list", "placement": "bottom"},
        "tooltip": {"mode": "multi"},
    }
    if kind == "stat":
        options = {
            "reduceOptions": {"calcs": ["lastNotNull"], "fields": "", "values": False},
            "orientation": "auto",
            "textMode": "auto",
            "colorMode": "value",
            "graphMode": "none",
        }
    else:
        defaults["custom"] = {
            "drawStyle": "line",
            "lineWidth": 2,
            "fillOpacity": 8,
            "spanNulls": False,
        }
    current_panels.append(
        {
            "id": counter,
            "type": kind,
            "title": title,
            "description": description,
            "datasource": {"type": "prometheus", "uid": "${datasource}"},
            "gridPos": {"x": x, "y": y, "w": width, "h": panel_height},
            "targets": [
                {
                    "refId": chr(65 + i),
                    "expr": expr,
                    "legendFormat": legend,
                    "interval": "15s",
                    "instant": kind == "stat",
                }
                for i, (legend, expr) in enumerate(expressions)
            ],
            "fieldConfig": {"defaults": defaults, "overrides": []},
            "options": options,
        }
    )
    x, height = x + width, max(height, panel_height)


row("Queries")
panel(
    "Running now",
    shared("linha_job_records", 'state="RUNNING"'),
    kind="stat",
    width=4,
    decimals=0,
    description="Requests currently executing. Cancelling requests are shown separately in the activity chart.",
)
panel(
    "Queued now",
    shared("linha_job_records", 'state=~"QUEUED|RETRYING"'),
    kind="stat",
    width=4,
    decimals=0,
    description="Accepted requests waiting to execute, including those waiting before a retry.",
)
panel(
    "Succeeded · selected period",
    shared("linha_job_completions_total", 'outcome="SUCCEEDED"', RANGE),
    kind="stat",
    width=4,
    decimals=0,
    description="Successful requests during the dashboard time range. Estimated from Prometheus samples; retries are not extra requests.",
)
panel(
    "Failed · selected period",
    shared("linha_job_completions_total", 'outcome="FAILED"', RANGE),
    kind="stat",
    width=4,
    decimals=0,
    description="Final request failures during the dashboard time range, after any allowed retries. Open Requests in the console for exceptions.",
)
panel(
    "Success rate · selected period",
    shared("linha_job_completions_total", 'outcome="SUCCEEDED"', RANGE)
    + " / "
    + shared("linha_job_completions_total", 'outcome=~"SUCCEEDED|FAILED"', RANGE),
    "percentunit",
    "stat",
    width=4,
    decimals=1,
    description="Succeeded / (succeeded + failed) during the selected period. Cancelled requests are excluded. No completed requests means no percentage.",
)
panel(
    "Ready servers",
    shared("linha_servers", 'state="ready"'),
    kind="stat",
    width=4,
    decimals=0,
    description="Servers with a recent ready heartbeat. Deployment-wide: engine/context filters do not apply.",
)
panel(
    "Query activity",
    [
        (label, shared("linha_job_records", f'state=~"{states}"'))
        for label, states in [
            ("Running", "RUNNING"),
            ("Queued", "QUEUED|RETRYING"),
            ("Cancelling", "CANCELLING"),
        ]
    ],
    description="How many requests are running, waiting, or being cancelled at each moment.",
)
panel(
    "Completions per minute",
    [
        (
            label,
            "60 * " + shared("linha_job_completions_total", f'outcome="{state}"', RATE),
        )
        for label, state in [
            ("Succeeded", "SUCCEEDED"),
            ("Failed", "FAILED"),
            ("Cancelled", "CANCELLED"),
        ]
    ],
    description="Request outcomes per minute. Counts each request once, even when execution needed retries.",
)

row("Time spent waiting and processing")
panel(
    "Processing time",
    [
        ("Average", mean("linha_attempt_duration_seconds")),
        ("95% finish within", quantile("linha_attempt_duration_seconds")),
    ],
    "s",
    description="Time from assignment to a finished execution. Queue wait is excluded; retries count as separate executions. The 95% line highlights slower work.",
)
panel(
    "Queue waiting time",
    [
        ("Typical (median)", quantile("linha_job_queue_wait_seconds", 0.5)),
        ("95% assigned within", quantile("linha_job_queue_wait_seconds")),
    ],
    "s",
    description="Submission to first assignment for requests that started. A growing queue wait usually means demand exceeds available execution capacity.",
)
panel(
    "End-to-end request time",
    [
        ("Average", mean("linha_job_duration_seconds")),
        ("95% finish within", quantile("linha_job_duration_seconds")),
    ],
    "s",
    description="Submission to a final outcome, including queue wait, retries and processing. Includes succeeded, failed and cancelled requests.",
)
panel(
    "Oldest queued request",
    shared("linha_queue_oldest_age_seconds", aggregate="max"),
    "s",
    description="Age of the oldest waiting request across selected contexts, including retry backoff. Zero means the selected queues are empty.",
)

row("Capacity and resources")
panel(
    "Clients and contexts now",
    [
        ("Client connections", shared("linha_client_leases", 'state="active"')),
        (
            "Active context versions",
            shared("linha_context_versions", 'state!="STOPPED"'),
        ),
    ],
    kind="stat",
    width=8,
    decimals=0,
    description="Live client connections to context versions, plus versions starting, running or draining. One client can connect to several versions.",
)
panel(
    "Worker capacity now",
    [
        ("Ready workers", shared("linha_workers", 'state="ready"')),
        ("Execution slots", shared("linha_worker_capacity_slots")),
        ("Slots in use", shared("linha_worker_occupied_slots")),
    ],
    kind="stat",
    width=8,
    decimals=0,
    description="Ready worker processes and the concurrent executions they can accept. Compare occupied slots with total slots when queues grow.",
)
panel(
    "Spark pods running now",
    [
        ("Drivers", shared("linha_spark_driver_pods", 'state="Running"')),
        ("Executors", shared("linha_spark_executor_pods", 'state="Running"')),
    ],
    kind="stat",
    width=8,
    decimals=0,
    description="Currently running Spark pods. Not available when any required selected observation is stale or incomplete; retired instances are excluded.",
)
for metric, title, unit in [
    ("linha_spark_pod_cpu_cores", "Spark CPU requests and limits", "cores"),
    ("linha_spark_pod_memory_bytes", "Spark memory requests and limits", "bytes"),
]:
    panel(
        title,
        [
            (
                f"{role.title()} {allocation}s",
                shared(metric, f'role="{role}",allocation="{allocation}"'),
            )
            for role in ("driver", "executor")
            for allocation in ("request", "limit")
        ],
        unit,
        description="Configured resources for current Spark pods, not measured consumption. Requests reserve capacity; limits cap it. Any stale or incomplete selected observation makes the total unavailable.",
    )

row("Diagnostics · expand when investigating a problem", collapsed=True)
panel(
    "Server readiness",
    [("{{pod}}", "linha_server_ready{" + LOCAL + "}")],
    description="1 means the server is ready, 0 means a dependency is unavailable. Deployment/server filters apply; context filters do not.",
)
panel(
    "Monitoring data age",
    [
        (
            "{{pod}} · {{collector}}",
            "time()-linha_collector_last_success_timestamp_seconds{" + LOCAL + "}",
        )
    ],
    "s",
    description="Seconds since each collector last succeeded. Growing values explain unavailable or stale panels. Zero timestamps mean collection has never succeeded.",
)
panel(
    "API failures per minute",
    [
        (
            "{{status_code}}",
            "60 * "
            + local(
                "linha_api_requests_total", 'status_code=~"5.."', group="status_code"
            ),
        )
    ],
    description="Server-side HTTP failures per minute. Inspect server logs for details; query business failures are shown in the main Queries section.",
)
panel(
    "Dependency errors per minute",
    [
        (
            "Kubernetes",
            "60 * "
            + local("linha_kubernetes_api_requests_total", 'outcome!="success"'),
        ),
        (
            "Result storage",
            "60 * " + local("linha_storage_operations_total", 'outcome="error"'),
        ),
        (
            "Database connections",
            "60 * " + local("linha_database_pool_acquire_failures_total"),
        ),
    ],
    description="Server-observed dependency failures per minute. Storage excludes direct worker/Spark traffic. These are process events, not failed query totals.",
)


def variable(name, query, label=None, hide=0):
    return {
        "name": name,
        "label": label or name.replace("_", " ").title(),
        "type": "query",
        "hide": hide,
        "datasource": {"type": "prometheus", "uid": "${datasource}"},
        "query": query,
        "definition": query,
        "refresh": 2,
        "multi": True,
        "includeAll": True,
        "allValue": ".*",
        "current": {"text": "All", "value": "$__all"},
    }


variables = [
    {"name": "datasource", "type": "datasource", "query": "prometheus", "current": {}},
    variable("cluster", "label_values(linha_server_build_info,cluster)"),
    variable(
        "namespace",
        'label_values(linha_server_build_info{cluster=~"$cluster"},namespace)',
    ),
    variable(
        "linha_deployment",
        'label_values(linha_server_build_info{cluster=~"$cluster",namespace=~"$namespace"},linha_deployment)',
        "Deployment",
    ),
    variable(
        "engine", "label_values(linha_context_versions{" + DEPLOYMENT + "},engine)"
    ),
    variable(
        "context",
        "label_values(linha_context_versions{"
        + DEPLOYMENT
        + ',engine=~"$engine"},context)',
    ),
    variable(
        "pod",
        "label_values(linha_server_build_info{" + DEPLOYMENT + "},pod)",
        "Server pod (diagnostics)",
        2,
    ),
    {"name": "uiBaseUrl", "type": "constant", "query": "", "hide": 2},
]
dashboard = {
    "uid": "linha-operations",
    "title": "Linha · Queries and capacity",
    "description": "Query activity, outcomes, processing time and the capacity serving your requests. Expand Diagnostics for internal health checks.",
    "tags": ["linha", "operations"],
    "schemaVersion": 39,
    "version": 2,
    "timezone": "utc",
    "refresh": "30s",
    "time": {"from": "now-6h", "to": "now"},
    "editable": True,
    "templating": {"list": variables},
    "panels": panels,
    "links": [],
}
out = ROOT / "deploy/helm/linha/dashboards/linha-operations.json"
out.parent.mkdir(parents=True, exist_ok=True)
out.write_text(json.dumps(dashboard, indent=2, ensure_ascii=False) + "\n")
main = sum(p["type"] != "row" for p in panels)
extra = sum(len(p.get("panels", [])) for p in panels if p["type"] == "row")
print(f"Generated {main} primary panels and {extra} collapsed diagnostic panels")
