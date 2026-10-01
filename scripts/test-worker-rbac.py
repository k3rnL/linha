#!/usr/bin/env python3
"""Verify Spark cleanup RBAC in an owned disposable Kind cluster, then remove it.
Requires Docker, Kind, kubectl, Helm and PyYAML. Never uses the current context.
"""
import copy
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time
from urllib.parse import quote
import uuid

import yaml

ROOT = Path(__file__).resolve().parents[1]
cluster = "linha-rbac-" + uuid.uuid4().hex[:10]
namespace = "cleanup-test"
release = "rbac"
worker = "rbac-linha-worker"
server = "rbac-linha"
selector = quote("spark-app-selector=cleanup-fixture,spark-role=executor", safe="")

with tempfile.TemporaryDirectory(prefix="linha-rbac-") as temp:
    config = str(Path(temp) / "kubeconfig")
    plugins = str(Path(temp) / "plugins")
    Path(plugins).mkdir()
    base = [
        "kubectl",
        "--kubeconfig",
        config,
        "--context",
        "kind-" + cluster,
        "--request-timeout=15s",
        "-n",
        namespace,
    ]

    def kube(*args, data=None, identity=None, check=True):
        command = base.copy()
        if identity:
            command += [
                "--as=system:serviceaccount:" + namespace + ":" + identity,
                "--as-group=system:authenticated",
                "--as-group=system:serviceaccounts",
                "--as-group=system:serviceaccounts:" + namespace,
            ]
        result = subprocess.run(
            command + list(args), input=data, text=True, capture_output=True, timeout=30
        )
        if check and result.returncode:
            raise AssertionError(result.stderr)
        return result

    def apply(objects):
        kube(
            "apply",
            "-f",
            "-",
            data=json.dumps({"apiVersion": "v1", "kind": "List", "items": objects}),
        )

    def collection(resource, identity=worker, target_namespace=namespace):
        return kube(
            "delete",
            "--raw",
            "/api/v1/namespaces/"
            + target_namespace
            + "/"
            + resource
            + "?labelSelector="
            + selector,
            identity=identity,
            check=False,
        )

    def denied(resource, identity=worker, target_namespace=namespace):
        result = collection(resource, identity, target_namespace)
        assert (
            result.returncode != 0
            and "Forbidden" in result.stderr
            and "deletecollection" in result.stderr
        ), result.stderr

    def fixture(kind, name, selected):
        result = {
            "apiVersion": "v1",
            "kind": kind,
            "metadata": {
                "name": name,
                "labels": {
                    "spark-app-selector": (
                        "cleanup-fixture" if selected else "unrelated"
                    ),
                    "spark-role": "executor",
                },
            },
        }
        if kind == "Pod":
            # Deliberately unschedulable: this RBAC test needs no image pull or running workload.
            result["spec"] = {
                "nodeSelector": {"linha-test.invalid/node": "absent"},
                "containers": [
                    {"name": "fixture", "image": "registry.k8s.io/pause:3.10"}
                ],
            }
        else:
            result["data"] = {"example": "disposable fixture"}
        return result

    try:
        subprocess.run(
            [
                "kind",
                "create",
                "cluster",
                "--name",
                cluster,
                "--kubeconfig",
                config,
                "--image",
                "kindest/node:v1.33.1",
                "--wait",
                "60s",
            ],
            check=True,
            timeout=180,
        )
        kube("create", "namespace", namespace)
        rendered = subprocess.check_output(
            [
                "helm",
                "template",
                release,
                str(ROOT / "deploy/helm/linha"),
                "--namespace",
                namespace,
                "--show-only",
                "templates/rbac.yaml",
                "--set",
                "security.oidc.enabled=false",
            ],
            text=True,
            env=dict(os.environ, HELM_PLUGINS=plugins),
        )
        corrected = [item for item in yaml.safe_load_all(rendered) if item]
        assert not any(item["kind"].startswith("Cluster") for item in corrected)
        previous = copy.deepcopy(corrected)
        for item in previous:
            if item["kind"] == "Role" and item["metadata"]["name"] == worker:
                item["rules"] = [
                    r for r in item["rules"] if "deletecollection" not in r["verbs"]
                ]
        apply(previous)
        uid = json.loads(kube("get", "serviceaccount", worker, "-o", "json").stdout)[
            "metadata"
        ]["uid"]
        for resource in ("pods", "configmaps"):
            denied(resource)
        print(
            "PASS old worker Role denies Pod and ConfigMap collection DELETE",
            flush=True,
        )

        apply(
            [
                fixture(kind, name, selected)
                for kind in ("Pod", "ConfigMap")
                for name, selected in (
                    ("selected-executor", True),
                    ("unrelated-executor", False),
                )
            ]
        )
        apply(corrected)
        assert (
            json.loads(kube("get", "serviceaccount", worker, "-o", "json").stdout)[
                "metadata"
            ]["uid"]
            == uid
        )
        for resource in ("pods", "configmaps"):
            deadline = time.monotonic() + 20
            while True:
                result = collection(resource)
                if result.returncode == 0:
                    break
                if time.monotonic() >= deadline:
                    raise AssertionError(result.stderr)
                time.sleep(0.2)
            deadline = time.monotonic() + 20
            while True:
                selected = json.loads(
                    kube(
                        "get",
                        resource,
                        "-l",
                        "spark-app-selector=cleanup-fixture",
                        "-o",
                        "json",
                    ).stdout
                )["items"]
                if not selected:
                    break
                assert time.monotonic() < deadline, (
                    "selected " + resource + " were not removed"
                )
                time.sleep(0.2)
            kube("get", resource, "unrelated-executor")
        print(
            "PASS updated Role permits selector cleanup, preserves unrelated resources and reuses the service account",
            flush=True,
        )

        for resource in ("pods", "configmaps"):
            denied(resource, target_namespace="default")
            denied(resource, identity=server)
        for resource in ("services", "persistentvolumeclaims", "secrets"):
            denied(resource)
        print(
            "PASS other namespaces, server identity, Services, PVCs and Secrets still deny collection DELETE",
            flush=True,
        )
    finally:
        subprocess.run(
            ["kind", "delete", "cluster", "--name", cluster, "--kubeconfig", config],
            check=True,
            timeout=90,
        )
        print("Removed owned disposable cluster " + cluster, flush=True)
