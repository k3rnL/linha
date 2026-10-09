#!/usr/bin/env python3
"""Render configuration combinations; requires Helm and PyYAML (no cluster)."""
import itertools
import json
import os
import tempfile
import pathlib
import subprocess
import unittest
import yaml

ROOT = pathlib.Path(__file__).resolve().parents[1]
CHART = ROOT / "deploy/helm/linha"


def render(*settings, succeeds=True, api_versions=()):
    command = ["helm", "template", "test", str(CHART), "--namespace", "isolated"]
    for version in api_versions:
        command += ["--api-versions", version]
    for setting in settings:
        command += ["--set", setting]
    with tempfile.TemporaryDirectory(prefix="linha-helm-plugins-") as plugins:
        result = subprocess.run(
            command,
            text=True,
            capture_output=True,
            env=dict(os.environ, HELM_PLUGINS=plugins),
        )
    if not succeeds:
        assert result.returncode != 0, "invalid chart configuration accepted"
        return result.stderr
    assert result.returncode == 0, result.stderr
    return list(yaml.safe_load_all(result.stdout))


def environment(documents):
    deployment = next(d for d in documents if d["kind"] == "Deployment")
    env = deployment["spec"]["template"]["spec"]["containers"][0]["env"]
    return {e["name"]: e for e in env}


class ChartConfiguration(unittest.TestCase):
    def test_monitoring_combinations_and_namespaced_rbac(self):
        versions = (
            "monitoring.coreos.com/v1/ServiceMonitor",
            "grafana.integreatly.org/v1beta1/GrafanaDashboard",
        )
        for monitor, dashboard in itertools.product((False, True), repeat=2):
            settings = [
                "security.enabled=false",
                f"metrics.serviceMonitor.enabled={str(monitor).lower()}",
                f"metrics.grafanaDashboard.enabled={str(dashboard).lower()}",
                "metrics.grafanaDashboard.instanceSelector.matchLabels.team=linha",
                "metrics.cluster=test",
            ]
            docs = render(*settings, api_versions=versions)
            self.assertEqual(
                sum(d["kind"] == "ServiceMonitor" for d in docs), int(monitor)
            )
            self.assertEqual(
                sum(d["kind"] == "GrafanaDashboard" for d in docs), int(dashboard)
            )
            self.assertFalse(any(d["kind"].startswith("Cluster") for d in docs))
            if monitor:
                m = next(d for d in docs if d["kind"] == "ServiceMonitor")
                self.assertEqual(
                    m["spec"]["namespaceSelector"]["matchNames"], ["isolated"]
                )
                self.assertEqual(m["spec"]["endpoints"][0]["port"], "http")
                self.assertNotIn("filterRunning", m["spec"]["endpoints"][0])
            if dashboard:
                d = next(d for d in docs if d["kind"] == "GrafanaDashboard")
                data = json.loads(d["spec"]["json"])
                self.assertEqual(data["uid"], "linha-operations")
                self.assertFalse(data.get("links"))
        self.assertIn(
            "ServiceMonitor",
            render(
                "security.enabled=false",
                "metrics.serviceMonitor.enabled=true",
                succeeds=False,
            ),
        )
        self.assertIn(
            "GrafanaDashboard",
            render(
                "security.enabled=false",
                "metrics.grafanaDashboard.enabled=true",
                "metrics.grafanaDashboard.instanceSelector.matchLabels.team=linha",
                succeeds=False,
            ),
        )
        render(
            "security.enabled=false",
            "metrics.serviceMonitor.enabled=true",
            "metrics.serviceMonitor.interval=5s",
            "metrics.serviceMonitor.scrapeTimeout=10s",
            api_versions=versions,
            succeeds=False,
        )
        render(
            "security.enabled=false",
            "metrics.serviceMonitor.labels.app\\.kubernetes\\.io/name=spoof",
            api_versions=versions,
            succeeds=False,
        )

    def test_dashboard_overview_links_and_collapsed_diagnostics(self):
        docs = render(
            "security.enabled=false",
            "ui.enabled=true",
            "ui.publicURL=https://linha.example/ui/",
            "metrics.grafanaDashboard.enabled=true",
            "metrics.grafanaDashboard.datasourceUID=production",
            "metrics.grafanaDashboard.instanceSelector.matchLabels.team=linha",
            api_versions=("grafana.integreatly.org/v1beta1/GrafanaDashboard",),
        )
        data = json.loads(
            next(d for d in docs if d["kind"] == "GrafanaDashboard")["spec"]["json"]
        )
        self.assertEqual(
            [link["url"] for link in data["links"]],
            [
                "https://linha.example/ui/",
                "https://linha.example/ui/contexts",
                "https://linha.example/ui/requests",
            ],
        )
        self.assertEqual(data["version"], 2)
        self.assertTrue(data["panels"][-1]["collapsed"])
        self.assertEqual(len(data["panels"][-1]["panels"]), 4)
        self.assertEqual(
            next(v for v in data["templating"]["list"] if v["name"] == "datasource")[
                "current"
            ]["value"],
            "production",
        )
        # Aggregated panels have no context label: do not emit misleading per-series URLs.
        for panel in data["panels"]:
            if "fieldConfig" in panel:
                self.assertFalse(panel["fieldConfig"]["defaults"]["links"])

    def test_ui_security_matrix_and_browser_secret(self):
        render(
            "ui.enabled=true",
            "ui.publicURL=https://linha.example/ui/",
            "security.oidc.enabled=false",
            succeeds=False,
        )
        render("security.enabled=false", "ui.enabled=true", succeeds=False)
        docs = render(
            "security.enabled=false",
            "ui.enabled=true",
            "ui.publicURL=http://linha.example/ui/",
        )
        self.assertEqual(environment(docs)["LINHA_UI_ENABLED"]["value"], "true")
        docs = render(
            "ui.enabled=true",
            "ui.publicURL=https://linha.example/ui/",
            "security.oidc.issuer=https://issuer.example",
            "security.oidc.audience=sdk",
            "ui.oidc.clientId=browser",
            "ui.oidc.secretRef=browser-secret",
            "ui.oidc.clientSecretKey=secret",
        )
        self.assertEqual(
            environment(docs)["LINHA_UI_OIDC_CLIENT_SECRET"]["valueFrom"][
                "secretKeyRef"
            ],
            {"name": "browser-secret", "key": "secret"},
        )
        self.assertEqual(
            environment(docs)["LINHA_UI_OIDC_CLIENT_ID"]["value"], "browser"
        )

    def test_ui_ingress_disabled_preserves_external_routing(self):
        for settings in (
            (),
            ("ui.enabled=true", "ui.publicURL=http://127.0.0.1:8080/ui/"),
        ):
            docs = render("security.enabled=false", *settings)
            self.assertFalse(any(d["kind"] == "Ingress" for d in docs))

    def test_ui_ingress_routes_canonical_origin_and_ha_service(self):
        for public_url in (
            "http://linha.example/ui/",
            "https://linha.example/ui/",
            "https://linha.example:8443/ui/",
        ):
            with self.subTest(public_url=public_url):
                docs = render(
                    "security.enabled=false",
                    "ui.enabled=true",
                    "ui.ingress.enabled=true",
                    "ui.publicURL=" + public_url,
                )
                ingress = next(d for d in docs if d["kind"] == "Ingress")
                self.assertEqual(ingress["apiVersion"], "networking.k8s.io/v1")
                self.assertEqual(ingress["metadata"]["namespace"], "isolated")
                self.assertEqual(ingress["metadata"]["name"], "test-linha-ui")
                self.assertNotIn("annotations", ingress["metadata"])
                self.assertNotIn("ingressClassName", ingress["spec"])
                self.assertNotIn("tls", ingress["spec"])
                self.assertNotIn("defaultBackend", ingress["spec"])
                self.assertEqual(len(ingress["spec"]["rules"]), 1)
                rule = ingress["spec"]["rules"][0]
                self.assertEqual(rule["host"], "linha.example")
                service = next(d for d in docs if d["kind"] == "Service")
                paths = rule["http"]["paths"]
                self.assertEqual([p["path"] for p in paths], ["/ui", "/v1/admin"])
                for path in paths:
                    self.assertEqual(path["pathType"], "Prefix")
                    self.assertEqual(
                        path["backend"]["service"],
                        {"name": service["metadata"]["name"], "port": {"name": "http"}},
                    )
                self.assertIn(
                    {"name": "http", "port": 8080, "targetPort": "http"},
                    service["spec"]["ports"],
                )
                deployment = next(d for d in docs if d["kind"] == "Deployment")
                self.assertEqual(deployment["spec"]["replicas"], 2)
                self.assertEqual(service["spec"]["selector"], {"app": "test-linha"})
                self.assertNotIn("sessionAffinity", service["spec"])
                self.assertEqual(
                    environment(docs)["LINHA_UI_PUBLIC_URL"]["value"], public_url
                )
                self.assertFalse(any(d["kind"].startswith("Cluster") for d in docs))

    def test_ui_ingress_tls_class_annotations_and_oidc(self):
        docs = render(
            "ui.enabled=true",
            "ui.publicURL=https://linha.example/ui/",
            "security.oidc.issuer=https://issuer.example",
            "security.oidc.audience=sdk",
            "ui.oidc.clientId=browser",
            "ui.ingress.enabled=true",
            "ui.ingress.className=nginx",
            "ui.ingress.annotations.cert-manager\\.io/cluster-issuer=letsencrypt",
            "ui.ingress.tls.enabled=true",
            "ui.ingress.tls.secretName=linha-ui-tls",
        )
        ingress = next(d for d in docs if d["kind"] == "Ingress")
        self.assertEqual(ingress["spec"]["ingressClassName"], "nginx")
        self.assertEqual(
            ingress["metadata"]["annotations"],
            {"cert-manager.io/cluster-issuer": "letsencrypt"},
        )
        self.assertEqual(
            ingress["spec"]["tls"],
            [{"hosts": ["linha.example"], "secretName": "linha-ui-tls"}],
        )
        self.assertEqual(environment(docs)["LINHA_SECURITY_ENABLED"]["value"], "true")
        self.assertEqual(
            environment(docs)["LINHA_UI_OIDC_CLIENT_ID"]["value"], "browser"
        )
        self.assertFalse(any(d["kind"] in ("Secret", "ClusterRole") for d in docs))

    def test_ui_ingress_rejects_invalid_configuration(self):
        self.assertIn(
            "ui.enabled",
            render("security.enabled=false", "ui.ingress.enabled=true", succeeds=False),
        )
        settings = (
            "security.enabled=false",
            "ui.enabled=true",
            "ui.ingress.enabled=true",
        )
        for host in (
            "127.0.0.1",
            "[::1]",
            "*.linha.example",
            "user@linha.example",
            "LINHA.example",
            "linha..example",
            "linha.example.",
            "linha_example",
            "-linha.example",
            "linha-.example",
            "a" * 64 + ".example",
            ".".join(["a" * 63] * 3 + ["a" * 62]),
        ):
            with self.subTest(host=host):
                self.assertIn(
                    "ui.publicURL",
                    render(
                        *settings,
                        "ui.publicURL=https://" + host + "/ui/",
                        succeeds=False,
                    ),
                )
        self.assertIn(
            "https",
            render(
                *settings,
                "ui.publicURL=http://linha.example/ui/",
                "ui.ingress.tls.enabled=true",
                "ui.ingress.tls.secretName=linha-ui-tls",
                succeeds=False,
            ),
        )
        self.assertIn(
            "secretName",
            render(
                *settings,
                "ui.publicURL=https://linha.example/ui/",
                "ui.ingress.tls.enabled=true",
                succeeds=False,
            ),
        )
        for invalid in (
            "ui.ingress.enabled=typo",
            "ui.ingress.className=invalid_class",
            "ui.ingress.annotations.flag=true",
            "ui.ingress.tls.enabled=typo",
            "ui.ingress.tls.secretName=invalid_secret",
        ):
            with self.subTest(invalid=invalid):
                render(
                    *settings,
                    "ui.publicURL=https://linha.example/ui/",
                    invalid,
                    succeeds=False,
                )

    def test_staging_limit_is_a_decimal_integer(self):
        env = environment(render("security.enabled=false"))
        self.assertEqual(env["LINHA_MAX_STAGING_BYTES"]["value"], "5368709120")
        self.assertEqual(env["LINHA_CLEANUP_ENABLED"]["value"], "false")
        env = environment(
            render(
                "security.enabled=false",
                "results.cleanup.enabled=true",
                "results.maxStagingBytes=1048576",
            )
        )
        self.assertEqual(env["LINHA_MAX_STAGING_BYTES"]["value"], "1048576")
        self.assertEqual(env["LINHA_CLEANUP_ENABLED"]["value"], "true")

    def test_security_and_rbac_combinations(self):
        for enabled, oidc, namespace in itertools.product((False, True), repeat=3):
            with self.subTest(enabled=enabled, oidc=oidc, namespace=namespace):
                docs = render(
                    f"security.enabled={str(enabled).lower()}",
                    f"security.oidc.enabled={str(oidc).lower()}",
                    f"rbac.namespaceOnly={str(namespace).lower()}",
                    "security.oidc.issuer=https://issuer.example",
                    "security.oidc.audience=linha",
                )
                cluster = [d["kind"] for d in docs if d["kind"].startswith("Cluster")]
                self.assertEqual(
                    sorted(cluster),
                    (
                        ["ClusterRole", "ClusterRoleBinding"]
                        if enabled and not namespace
                        else []
                    ),
                )
                env = environment(docs)
                self.assertEqual(
                    env["LINHA_SECURITY_ENABLED"]["value"], str(enabled).lower()
                )
                self.assertEqual(
                    env["LINHA_WORKER_AUTH"]["value"],
                    "projected-token" if namespace else "token-review",
                )
                self.assertEqual(env["LINHA_NAMESPACE"]["value"], "isolated")
                self.assertNotIn("LINHA_MODE", env)
                for d in docs:
                    if d["kind"].endswith("Binding"):
                        self.assertTrue(
                            all(s["namespace"] == "isolated" for s in d["subjects"])
                        )
                role = next(
                    d
                    for d in docs
                    if d["kind"] == "Role" and d["metadata"]["name"] == "test-linha"
                )
                application_rules = [
                    r for r in role["rules"] if "sparkapplications" in r["resources"]
                ]
                self.assertEqual(
                    application_rules,
                    [
                        {
                            "apiGroups": ["sparkoperator.k8s.io"],
                            "resources": ["sparkapplications"],
                            "verbs": ["get", "list", "watch", "create", "delete"],
                        }
                    ],
                )
                sa_rules = [
                    r for r in role["rules"] if "serviceaccounts" in r["resources"]
                ]
                self.assertEqual(len(sa_rules), int(enabled and namespace))
                if sa_rules:
                    self.assertEqual(sa_rules[0]["verbs"], ["get"])

    def test_worker_collection_cleanup_permissions(self):
        for enabled, namespace in itertools.product((False, True), repeat=2):
            with self.subTest(enabled=enabled, namespace=namespace):
                docs = render(
                    f"security.enabled={str(enabled).lower()}",
                    f"rbac.namespaceOnly={str(namespace).lower()}",
                    "security.oidc.issuer=https://issuer.example",
                    "security.oidc.audience=linha",
                )
                worker = next(
                    d
                    for d in docs
                    if d["kind"] == "Role"
                    and d["metadata"]["name"] == "test-linha-worker"
                )
                cleanup = [
                    r
                    for r in worker["rules"]
                    if "deletecollection" in r["verbs"] or "*" in r["verbs"]
                ]
                self.assertEqual(
                    len(cleanup), 1, "missing worker collection cleanup rule"
                )
                self.assertEqual(cleanup[0]["apiGroups"], [""])
                self.assertEqual(set(cleanup[0]["resources"]), {"pods", "configmaps"})
                self.assertEqual(cleanup[0]["verbs"], ["deletecollection"])
                self.assertNotIn("resourceNames", cleanup[0])
                # Preserve the existing management verbs without giving collection
                # deletion to Services/PVCs, the server, or cluster-scoped roles.
                management = next(
                    r for r in worker["rules"] if "services" in r["resources"]
                )
                self.assertEqual(
                    set(management["verbs"]),
                    {"get", "list", "watch", "create", "delete", "patch"},
                )
                for role in [
                    d
                    for d in docs
                    if d["kind"] in ("Role", "ClusterRole") and d is not worker
                ]:
                    self.assertFalse(
                        any(
                            "deletecollection" in r["verbs"] or "*" in r["verbs"]
                            for r in role["rules"]
                        )
                    )
                binding = next(
                    d
                    for d in docs
                    if d["kind"] == "RoleBinding"
                    and d["metadata"]["name"] == "test-linha-worker"
                )
                self.assertEqual(binding["roleRef"]["kind"], "Role")
                self.assertEqual(
                    binding["subjects"],
                    [
                        {
                            "kind": "ServiceAccount",
                            "name": "test-linha-worker",
                            "namespace": "isolated",
                        }
                    ],
                )

    def test_secret_keys_and_disabled_oidc(self):
        docs = render(
            "security.oidc.enabled=false",
            "database.postgres.secretRef=my-db",
            "database.postgres.usernameKey=db-user",
            "database.postgres.passwordKey=db-password",
            "database.postgres.host=postgres.example",
            "database.postgres.port=5544",
            "database.postgres.database=jobs",
            "s3.enabled=true",
            "s3.secretRef=results-config",
            "s3.destinationsKey=locations",
        )
        env = environment(docs)
        self.assertEqual(
            env["LINHA_DATABASE_USERNAME"]["valueFrom"]["secretKeyRef"],
            {"name": "my-db", "key": "db-user"},
        )
        self.assertEqual(
            env["LINHA_DATABASE_PASSWORD"]["valueFrom"]["secretKeyRef"],
            {"name": "my-db", "key": "db-password"},
        )
        self.assertEqual(
            env["LINHA_S3_DESTINATIONS"]["valueFrom"]["secretKeyRef"],
            {"name": "results-config", "key": "locations"},
        )
        self.assertEqual(env["LINHA_DATABASE_PORT"]["value"], "5544")
        self.assertNotIn("LINHA_DATABASE_URL", env)
        self.assertFalse(any(d["kind"] == "Secret" for d in docs))
        # The master switch needs neither issuer nor audience despite oidc.enabled's default.
        render("security.enabled=false")

    def test_pod_defaults_and_registry_permissions(self):
        docs = render(
            "security.enabled=false",
            "imagePullSecrets[0].name=shared-pull",
            "registrySecrets.allowedNames[0]=app-pull",
            "registrySecrets.allowedNames[1]=shared-pull",
            "workerPodTemplates.driverPodTemplate.spec.nodeSelector.pool=compute",
        )
        env = environment(docs)
        self.assertEqual(
            json.loads(env["LINHA_WORKER_IMAGE_PULL_SECRETS"]["value"]), ["shared-pull"]
        )
        self.assertEqual(
            json.loads(env["LINHA_WORKER_POD_TEMPLATES"]["value"])["driverPodTemplate"][
                "spec"
            ]["nodeSelector"],
            {"pool": "compute"},
        )
        role = next(
            d
            for d in docs
            if d["kind"] == "Role" and d["metadata"]["name"] == "test-linha"
        )
        secret = next(r for r in role["rules"] if "secrets" in r["resources"])
        self.assertEqual(sorted(secret["resourceNames"]), ["app-pull", "shared-pull"])
        self.assertEqual(secret["verbs"], ["get"])
        configmaps = next(r for r in role["rules"] if "configmaps" in r["resources"])
        self.assertEqual(sorted(configmaps["verbs"]), ["create", "get"])
        self.assertFalse(any(d["kind"].startswith("Cluster") for d in docs))
        defaults = render("security.enabled=false")
        role = next(
            d
            for d in defaults
            if d["kind"] == "Role" and d["metadata"]["name"] == "test-linha"
        )
        self.assertFalse(any("secrets" in r["resources"] for r in role["rules"]))
        render(
            "security.enabled=false",
            "workerPodTemplates.driverPodTemplate.metadata.namespace=other",
            succeeds=False,
        )
        render(
            "security.enabled=false",
            "registrySecrets.allowedNames[0]=../other",
            succeeds=False,
        )

    def test_oidc_tls_option_and_types(self):
        key = "LINHA_OIDC_TLS_INSECURE_SKIP_VERIFY"
        defaults = environment(render("security.enabled=false"))
        self.assertEqual(defaults[key]["value"], "false")
        for security, oidc, skip in itertools.product((False, True), repeat=3):
            with self.subTest(security=security, oidc=oidc, skip=skip):
                docs = render(
                    f"security.enabled={str(security).lower()}",
                    f"security.oidc.enabled={str(oidc).lower()}",
                    f"security.oidc.tls.insecureSkipVerify={str(skip).lower()}",
                    "security.oidc.issuer=https://issuer.example",
                    "security.oidc.audience=linha",
                )
                env = environment(docs)
                self.assertEqual(env[key]["value"], str(skip).lower())
                self.assertEqual(env["LINHA_WORKER_AUTH"]["value"], "projected-token")
        for setting in (
            "security.oidc.tls.insecureSkipVerify=typo",
            "security.oidc.tls.insecureSkipVerify=1",
            "security.oidc.tls.insecureSkipVerify[0]=true",
            "security.oidc.tls.skipTLS=true",
        ):
            with self.subTest(setting=setting):
                self.assertIn(
                    "security.oidc.tls",
                    render("security.enabled=false", setting, succeeds=False),
                )

    def test_admin_mapping_issuer_inheritance(self):
        for role, kind in itertools.product(
            ("viewer", "operator"), ("subjects", "claims")
        ):
            with self.subTest(role=role, kind=kind):
                prefix = f"security.admin.{role}.{kind}[0]"
                settings = (
                    [f"{prefix}.subject=admin"]
                    if kind == "subjects"
                    else [f"{prefix}.path[0]=roles", f"{prefix}.values[0]=admin"]
                )
                for legacy in (False, True):
                    docs = render(
                        "security.oidc.issuer=https://issuer.example",
                        "security.oidc.audience=sdk",
                        "ui.enabled=true",
                        "ui.publicURL=https://linha.example/ui/",
                        "ui.oidc.clientId=browser",
                        *settings,
                        *(
                            [f"{prefix}.issuer=https://issuer.example"]
                            if legacy
                            else []
                        ),
                    )
                    mappings = json.loads(
                        environment(docs)["LINHA_ADMIN_MAPPINGS"]["value"]
                    )
                    rule = mappings[role][kind][0]
                    if legacy:
                        self.assertEqual(rule["issuer"], "https://issuer.example")
                    else:
                        self.assertNotIn("issuer", rule)

    def test_invalid_configuration(self):
        self.assertIn("security.oidc.issuer", render(succeeds=False))
        self.assertIn(
            "security.oidc.audience",
            render("security.oidc.issuer=https://issuer.example", succeeds=False),
        )
        for setting in (
            "existingSecret=old",
            "mode=development",
            "deploymentMode=production",
            "oidc.issuer=https://old",
        ):
            self.assertIn(
                "Legacy", render("security.enabled=false", setting, succeeds=False)
            )
        for setting in (
            "database.postgres.passwordKey=",
            "database.postgres.sslMode=typo",
            "database.postgres.port=70000",
            "security.enabled=typo",
        ):
            render("security.enabled=false", setting, succeeds=False)


if __name__ == "__main__":
    unittest.main()
