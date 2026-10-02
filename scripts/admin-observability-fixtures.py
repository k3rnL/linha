#!/usr/bin/env python3
"""Create/clean an owned Kind fixture for admin/Spark/Prometheus/Grafana acceptance."""
import argparse
import json
import pathlib
import shutil
import secrets
import subprocess
import tempfile
import time
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parents[1]


def run(*args, **kwargs):
    return subprocess.run(args, check=True, **kwargs)


def up(args):
    if args.state.exists():
        raise SystemExit("State already exists; clean that owned fixture first")
    directory = pathlib.Path(tempfile.mkdtemp(prefix="linha-ui-cluster-"))
    name = "linha-ui-" + str(int(time.time())) + "-" + secrets.token_hex(3)
    state = {
        "cluster": name,
        "context": "kind-" + name,
        "kubeconfig": str(directory / "kubeconfig"),
        "directory": str(directory),
        "postgres": name + "-pg",
        "values": str(directory / "values.json"),
    }
    args.state.write_text(json.dumps(state))
    (directory / ".linha-owned-fixture").write_text(name)
    run(
        "kind",
        "create",
        "cluster",
        "--name",
        name,
        "--image",
        "kindest/node:v1.33.1",
        "--kubeconfig",
        state["kubeconfig"],
    )
    node = name + "-control-plane"
    worker_tag = "linha/spark-fixture:" + name
    run("docker", "tag", args.worker_image, worker_tag)
    state["workerTag"] = worker_tag
    args.state.write_text(json.dumps(state))
    for image in (args.server_image, worker_tag):
        writer = subprocess.Popen(["docker", "save", image], stdout=subprocess.PIPE)
        run(
            "docker",
            "exec",
            "-i",
            node,
            "ctr",
            "-n",
            "k8s.io",
            "images",
            "import",
            "--all-platforms",
            "--digests",
            "-",
            stdin=writer.stdout,
            stdout=subprocess.DEVNULL,
        )
        writer.stdout.close()
        assert writer.wait() == 0
    images = subprocess.check_output(
        ["docker", "exec", node, "ctr", "-n", "k8s.io", "images", "ls"], text=True
    )
    line = next(
        v for v in images.splitlines() if v.startswith("docker.io/" + worker_tag + " ")
    )
    state["workerImage"] = "docker.io/linha/spark-fixture@" + line.split()[2]
    run(
        "docker",
        "exec",
        node,
        "ctr",
        "-n",
        "k8s.io",
        "images",
        "tag",
        "--force",
        "docker.io/" + worker_tag,
        state["workerImage"],
    )
    run(
        "docker",
        "exec",
        node,
        "ctr",
        "-n",
        "k8s.io",
        "images",
        "tag",
        "--force",
        state["workerImage"],
        state["workerImage"].replace("docker.io/", "index.docker.io/"),
    )
    base = [
        "kubectl",
        "--kubeconfig",
        state["kubeconfig"],
        "--context",
        state["context"],
    ]
    run(*base, "create", "namespace", "linha-ui-test")
    ns = base + ["-n", "linha-ui-test"]

    def apply(objects):
        run(
            *ns,
            "apply",
            "-f",
            "-",
            input=json.dumps({"apiVersion": "v1", "kind": "List", "items": objects}),
            text=True,
            stdout=subprocess.DEVNULL
        )

    run(
        "docker",
        "run",
        "-d",
        "--name",
        state["postgres"],
        "--network",
        "kind",
        "--label",
        "linha.validation=ui-observability",
        "-e",
        "POSTGRES_USER=linha",
        "-e",
        "POSTGRES_PASSWORD=linha-ui-test-only",
        "-e",
        "POSTGRES_DB=linha",
        "postgres:17-alpine",
        stdout=subprocess.DEVNULL,
    )
    run(
        "docker",
        "exec",
        node,
        "sh",
        "-c",
        "mkdir -p /var/lib/linha-ui-results && chmod 2777 /var/lib/linha-ui-results",
    )
    apply(
        [
            {
                "apiVersion": "v1",
                "kind": "PersistentVolume",
                "metadata": {"name": name + "-results"},
                "spec": {
                    "capacity": {"storage": "2Gi"},
                    "accessModes": ["ReadWriteMany"],
                    "storageClassName": "",
                    "hostPath": {"path": "/var/lib/linha-ui-results"},
                },
            },
            {
                "apiVersion": "v1",
                "kind": "PersistentVolumeClaim",
                "metadata": {"name": "results"},
                "spec": {
                    "accessModes": ["ReadWriteMany"],
                    "storageClassName": "",
                    "volumeName": name + "-results",
                    "resources": {"requests": {"storage": "2Gi"}},
                },
            },
            {
                "apiVersion": "v1",
                "kind": "Secret",
                "metadata": {"name": "linha-postgres"},
                "stringData": {"username": "linha", "password": "linha-ui-test-only"},
            },
        ]
    )
    for project, tag, destination in (
        ("kubeflow/spark-operator", "v2.4.0", "spark-operator"),
        ("grafana/grafana-operator", "v5.25.0", "grafana-operator"),
    ):
        run(
            "git",
            "clone",
            "--depth",
            "1",
            "--branch",
            tag,
            "https://github.com/" + project + ".git",
            str(directory / destination),
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
    state["sparkChart"] = str(directory / "spark-operator/charts/spark-operator-chart")
    helm = ["--kubeconfig", state["kubeconfig"], "--kube-context", state["context"]]
    ingress_values = directory / "spark-ingress.json"
    ingress_values.write_text(
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
    run(
        "helm",
        "upgrade",
        "--install",
        "spark-operator",
        state["sparkChart"],
        *helm,
        "-n",
        "spark-operator",
        "--create-namespace",
        "--set",
        "spark.jobNamespaces={linha-ui-test}",
        "-f",
        str(ingress_values),
        "--wait",
        "--timeout",
        "180s"
    )
    bundle = directory / "prometheus-operator.yaml"
    with urllib.request.urlopen(
        "https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/v0.94.1/bundle.yaml",
        timeout=30,
    ) as response:
        bundle.write_bytes(response.read())
    run(*base, "apply", "--server-side", "-f", str(bundle), stdout=subprocess.DEVNULL)
    run(
        *base,
        "-n",
        "default",
        "rollout",
        "status",
        "deployment/prometheus-operator",
        "--timeout=180s"
    )
    run(
        "helm",
        "upgrade",
        "--install",
        "grafana-operator",
        str(directory / "grafana-operator/deploy/helm/grafana-operator"),
        *helm,
        "-n",
        "linha-ui-test",
        "--set",
        "namespaceScope=true",
        "--set",
        "rbac.createClusterRole=false",
        "--wait",
        "--timeout",
        "180s"
    )
    apply(
        [
            {
                "apiVersion": "v1",
                "kind": "ServiceAccount",
                "metadata": {"name": "prometheus"},
            },
            {
                "apiVersion": "rbac.authorization.k8s.io/v1",
                "kind": "Role",
                "metadata": {"name": "prometheus"},
                "rules": [
                    {
                        "apiGroups": [""],
                        "resources": ["pods", "services", "endpoints"],
                        "verbs": ["get", "list", "watch"],
                    },
                    {
                        "apiGroups": ["discovery.k8s.io"],
                        "resources": ["endpointslices"],
                        "verbs": ["get", "list", "watch"],
                    },
                ],
            },
            {
                "apiVersion": "rbac.authorization.k8s.io/v1",
                "kind": "RoleBinding",
                "metadata": {"name": "prometheus"},
                "subjects": [
                    {
                        "kind": "ServiceAccount",
                        "name": "prometheus",
                        "namespace": "linha-ui-test",
                    }
                ],
                "roleRef": {
                    "apiGroup": "rbac.authorization.k8s.io",
                    "kind": "Role",
                    "name": "prometheus",
                },
            },
            {
                "apiVersion": "monitoring.coreos.com/v1",
                "kind": "Prometheus",
                "metadata": {"name": "linha"},
                "spec": {
                    "serviceAccountName": "prometheus",
                    "replicas": 1,
                    "version": "v3.7.1",
                    "serviceMonitorSelector": {"matchLabels": {"fixture": "linha-ui"}},
                    "serviceMonitorNamespaceSelector": {
                        "matchLabels": {"kubernetes.io/metadata.name": "linha-ui-test"}
                    },
                    "resources": {
                        "requests": {"memory": "256Mi"},
                        "limits": {"memory": "512Mi"},
                    },
                },
            },
            {
                "apiVersion": "grafana.integreatly.org/v1beta1",
                "kind": "Grafana",
                "metadata": {"name": "linha", "labels": {"dashboards": "linha-ui"}},
                "spec": {
                    "config": {
                        "log": {"mode": "console"},
                        "security": {
                            "admin_user": "admin",
                            "admin_password": "linha-fixture-only",
                        },
                    },
                    "deployment": {
                        "spec": {
                            "template": {
                                "spec": {
                                    "containers": [
                                        {
                                            "name": "grafana",
                                            "image": "grafana/grafana:13.2.3",
                                            "resources": {
                                                "requests": {"memory": "128Mi"},
                                                "limits": {"memory": "512Mi"},
                                            },
                                        }
                                    ]
                                }
                            }
                        }
                    },
                },
            },
        ]
    )
    run(
        "helm",
        "upgrade",
        "--install",
        "ingress-fixture",
        "traefik",
        "--repo",
        "https://traefik.github.io/charts",
        "--version",
        "41.6.1",
        *helm,
        "-n",
        "linha-ui-test",
        "--set",
        "providers.kubernetesCRD.enabled=false",
        "--set",
        "providers.kubernetesIngress.enabled=true",
        "--set",
        "service.type=ClusterIP"
    )
    repository, tag = args.server_image.rsplit(":", 1)
    values = {
        "security": {"enabled": False},
        "allowedImages": ["index.docker.io/linha"],
        "image": {"repository": repository, "tag": tag, "pullPolicy": "Never"},
        "database": {"postgres": {"host": state["postgres"], "sslMode": "disable"}},
        "local": {
            "enabled": True,
            "existingClaim": "results",
            "root": "/var/lib/linha/results",
        },
        "workerVolumes": [
            {"name": "results", "persistentVolumeClaim": {"claimName": "results"}}
        ],
        "workerMounts": [{"name": "results", "mountPath": "/var/lib/linha/results"}],
        "ui": {"enabled": True, "publicURL": "http://127.0.0.1:8080/ui/"},
        "metrics": {
            "serviceMonitor": {
                "enabled": True,
                "interval": "15s",
                "scrapeTimeout": "5s",
                "labels": {"fixture": "linha-ui"},
            },
            "grafanaDashboard": {
                "enabled": True,
                "instanceSelector": {"matchLabels": {"dashboards": "linha-ui"}},
                "datasourceUID": "prometheus",
            },
        },
    }
    pathlib.Path(state["values"]).write_text(json.dumps(values))
    args.state.write_text(json.dumps(state))
    run(
        "helm",
        "upgrade",
        "--install",
        "ui",
        str(ROOT / "deploy/helm/linha"),
        *helm,
        "-n",
        "linha-ui-test",
        "-f",
        state["values"],
        "--wait",
        "--timeout",
        "180s"
    )
    for deployment in ("linha-deployment", "ingress-fixture-traefik"):
        run(*ns, "rollout", "status", "deployment/" + deployment, "--timeout=180s")
    run(
        *ns, "wait", "--for=condition=Ready", "pod/prometheus-linha-0", "--timeout=180s"
    )
    print("READY", args.state)


def clean(args):
    state = json.loads(args.state.read_text())
    name = state["cluster"]
    directory = pathlib.Path(state["directory"])
    assert name.startswith("linha-ui-") and state["context"] == "kind-" + name
    assert (
        directory.name.startswith("linha-ui-cluster-")
        and (directory / ".linha-owned-fixture").read_text() == name
    )
    inspection = subprocess.run(
        [
            "docker",
            "inspect",
            "-f",
            '{{index .Config.Labels "linha.validation"}}',
            state["postgres"],
        ],
        capture_output=True,
        text=True,
    )
    if inspection.returncode == 0:
        assert inspection.stdout.strip() == "ui-observability"
        run("docker", "rm", "-f", state["postgres"], stdout=subprocess.DEVNULL)
    run(
        "kind", "delete", "cluster", "--name", name, "--kubeconfig", state["kubeconfig"]
    )
    if state.get("workerTag"):
        assert state["workerTag"].startswith("linha/spark-fixture:linha-ui-")
        subprocess.run(
            ["docker", "image", "rm", state["workerTag"]], stdout=subprocess.DEVNULL
        )
    shutil.rmtree(directory)
    args.state.unlink()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("up", "clean"))
    parser.add_argument("--state", required=True, type=pathlib.Path)
    parser.add_argument("--server-image", default="linha/server:observability-ui")
    parser.add_argument(
        "--worker-image", default="linha/spark-example:platform-complete"
    )
    args = parser.parse_args()
    (up if args.action == "up" else clean)(args)


if __name__ == "__main__":
    main()
