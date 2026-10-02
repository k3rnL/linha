#!/usr/bin/env python3
"""Validate PromQL, dashboard layout/catalogue and actual admin API fixtures."""
import argparse
import json
import pathlib
import re
import subprocess
import tempfile
import urllib.parse
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parents[1]


def all_panels(panels):
    for panel in panels:
        yield panel
        yield from all_panels(panel.get("panels", []))


def expressions(dashboard):
    for panel in all_panels(dashboard["panels"]):
        for target in panel.get("targets", []):
            expression = target["expr"]
            for variable in (
                "cluster",
                "namespace",
                "linha_deployment",
                "engine",
                "context",
                "pod",
            ):
                expression = expression.replace("$" + variable, ".*")
            yield expression.replace("$__rate_interval", "1m").replace("$__range", "5m")


def dashboard_cases(dashboard):
    by_title = {p["title"]: p for p in all_panels(dashboard["panels"])}

    def expression(title, index=0):
        panel = by_title[title].copy()
        panel["targets"] = [panel["targets"][index]]
        return next(expressions({"panels": [panel]}))

    labels = 'cluster="test",namespace="test",linha_deployment="test",engine="spark"'

    def series(metric, context, pod, extra, values):
        return {
            "series": f'{metric}{{{labels},context="{context}",pod="{pod}"{extra}}}',
            "values": values,
        }

    data = [
        series("linha_job_records", "a", "one", ',state="QUEUED"', "5 5 5 5 5 5"),
        series(
            "linha_job_records",
            "a",
            "two",
            ',state="QUEUED"',
            "5 5 5 stale stale stale",
        ),
        series("linha_job_records", "b", "one", ',state="RETRYING"', "3 3 3 3 3 3"),
        series("linha_queue_oldest_age_seconds", "a", "one", "", "10 10 10 10 10 10"),
        series("linha_queue_oldest_age_seconds", "b", "one", "", "20 20 20 20 20 20"),
        series(
            "linha_job_completions_total",
            "a",
            "one",
            ',outcome="FAILED"',
            "0 10 20 30 40 50",
        ),
        series(
            "linha_job_completions_total",
            "a",
            "two",
            ',outcome="FAILED"',
            "0 10 20 stale stale stale",
        ),
        series(
            "linha_spark_pod_cpu_cores",
            "a",
            "one",
            ',role="driver",allocation="request"',
            "2 2 2 2 2 2",
        ),
        series(
            "linha_engine_observations", "a", "one", ',state="fresh"', "1 1 1 1 1 1"
        ),
        series(
            "linha_engine_observations", "b", "one", ',state="missing"', "1 1 1 1 1 1"
        ),
    ]
    checks = []
    for title, value in [
        ("Queued now", 8),
        ("Oldest queued request", 20),
        ("Failed · selected period", 50),
        ("Spark CPU requests and limits", None),
    ]:
        checks.append(
            {
                "expr": "round((" + expression(title) + "),0.000001)",
                "eval_time": "5m",
                "exp_samples": (
                    [] if value is None else [{"labels": "{}", "value": value}]
                ),
            }
        )
    return {
        "name": "actual curated dashboard queries with HA and incomplete totals",
        "interval": "1m",
        "input_series": data,
        "promql_expr_test": checks,
    }


def schemas(value):
    if isinstance(value, list):
        return [schemas(v) for v in value]
    if not isinstance(value, dict):
        return value
    result = {k: schemas(v) for k, v in value.items() if k != "nullable"}
    if value.get("nullable"):
        return {"anyOf": [result, {"type": "null"}]}
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--fixtures", type=pathlib.Path)
    parser.add_argument("--prometheus-url")
    args = parser.parse_args()
    dashboard = json.loads(
        (ROOT / "deploy/helm/linha/dashboards/linha-operations.json").read_text()
    )
    rows = [p for p in dashboard["panels"] if p["type"] == "row"]
    flat = list(all_panels(dashboard["panels"]))
    panels = [p for p in flat if p["type"] != "row"]
    assert len(rows) == 4 and len(panels) == 21
    assert rows[-1]["collapsed"] and len(rows[-1]["panels"]) == 4
    assert len({p["id"] for p in flat}) == len(flat)
    catalogue = set(
        re.findall(r"`(linha_[a-z_]+)`", (ROOT / "docs/metrics.md").read_text())
    )
    for expression in expressions(dashboard):
        for metric in re.findall(r"linha_[a-z_]+(?=\{)", expression):
            assert any(
                metric == base
                or metric in (base + "_bucket", base + "_sum", base + "_count")
                for base in catalogue
            ), metric
    for i, a in enumerate(flat):
        for b in flat[i + 1 :]:
            x, y = a["gridPos"], b["gridPos"]
            assert not (
                x["x"] < y["x"] + y["w"]
                and y["x"] < x["x"] + x["w"]
                and x["y"] < y["y"] + y["h"]
                and y["y"] < x["y"] + x["h"]
            ), (a["title"], b["title"])
    queries = list(expressions(dashboard))
    if args.prometheus_url:
        for expression in queries:
            url = (
                args.prometheus_url.rstrip("/")
                + "/api/v1/query?"
                + urllib.parse.urlencode({"query": expression})
            )
            with urllib.request.urlopen(url, timeout=15) as response:
                assert json.load(response)["status"] == "success"
    else:
        with tempfile.TemporaryDirectory(prefix="linha-promql-") as directory:
            path = pathlib.Path(directory)
            path.chmod(0o755)
            (path / "rules.json").write_text(
                json.dumps(
                    {
                        "groups": [
                            {
                                "name": "dashboard",
                                "rules": [
                                    {"record": "linha_fixture_" + str(i), "expr": q}
                                    for i, q in enumerate(queries)
                                ],
                            }
                        ]
                    }
                )
            )
            subprocess.run(
                [
                    "docker",
                    "run",
                    "--rm",
                    "--entrypoint",
                    "/bin/promtool",
                    "-v",
                    str(path) + ":/fixtures:ro",
                    "prom/prometheus:v3.7.1",
                    "check",
                    "rules",
                    "/fixtures/rules.json",
                ],
                check=True,
            )
            fixtures = json.loads(
                (ROOT / "scripts/fixtures/observability-promql.json").read_text()
            )
            fixtures["tests"].append(dashboard_cases(dashboard))
            (path / "cases.json").write_text(json.dumps(fixtures))
            subprocess.run(
                [
                    "docker",
                    "run",
                    "--rm",
                    "--entrypoint",
                    "/bin/promtool",
                    "-v",
                    str(path) + ":/fixtures:ro",
                    "prom/prometheus:v3.7.1",
                    "test",
                    "rules",
                    "/fixtures/cases.json",
                ],
                check=True,
            )
    print(
        f"PASS {len(queries)} PromQL expressions, 17 main panels and four collapsed diagnostics"
    )
    if args.fixtures:
        import jsonschema

        contract = schemas(json.loads((ROOT / "api/openapi.json").read_text()))
        fixtures = json.loads(args.fixtures.read_text())
        for fixture in fixtures:
            schema = contract["paths"][fixture["path"]][fixture["method"].lower()][
                "responses"
            ][str(fixture["status"])]["content"]["application/json"]["schema"]
            validator = jsonschema.Draft4Validator(
                {"components": contract["components"], "allOf": [schema]},
                format_checker=jsonschema.FormatChecker(),
            )
            errors = list(validator.iter_errors(fixture["body"]))
            assert not errors, (
                fixture["path"],
                [(list(e.path), e.message) for e in errors],
            )
        print(f"PASS {len(fixtures)} real admin HTTP responses conform to OpenAPI")


if __name__ == "__main__":
    main()
