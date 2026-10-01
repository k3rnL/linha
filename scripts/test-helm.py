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


def render(*settings, succeeds=True):
    command = ["helm", "template", "test", str(CHART), "--namespace", "isolated"]
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
