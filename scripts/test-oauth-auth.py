#!/usr/bin/env python3
"""Verify OAuth client credentials against a disposable Keycloak realm."""
import json
import os
import pathlib
import secrets
import socket
import subprocess
import tempfile
import time
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parents[1]
IMAGE = "quay.io/keycloak/keycloak:26.5.7"


def main():
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        port = listener.getsockname()[1]
    address = f"http://127.0.0.1:{port}"
    issuer = address + "/realms/linha-test"
    client_secret = secrets.token_urlsafe(32)
    audience_mapper = {
        "name": "linha-api-audience",
        "protocol": "openid-connect",
        "protocolMapper": "oidc-audience-mapper",
        "consentRequired": False,
        "config": {
            "included.client.audience": "linha",
            "access.token.claim": "true",
            "id.token.claim": "false",
        },
    }
    realm = {
        "realm": "linha-test",
        "enabled": True,
        "accessTokenLifespan": 30,
        "clients": [
            {"clientId": "linha", "protocol": "openid-connect", "bearerOnly": True},
            *[
                {
                    "clientId": name,
                    "protocol": "openid-connect",
                    "publicClient": False,
                    "secret": client_secret,
                    "serviceAccountsEnabled": True,
                    "standardFlowEnabled": False,
                    "directAccessGrantsEnabled": False,
                    "protocolMappers": [audience_mapper],
                }
                for name in ("report-api", "import-api")
            ],
        ],
    }
    with tempfile.TemporaryDirectory(prefix="linha-oauth-") as temporary:
        directory = pathlib.Path(temporary)
        import_directory = directory / "import"
        import_directory.mkdir(mode=0o755)
        realm_file = import_directory / "linha-test-realm.json"
        realm_file.write_text(json.dumps(realm))
        realm_file.chmod(0o644)
        secret_file = directory / "client-secret"
        secret_file.write_text(client_secret)
        secret_file.chmod(0o600)
        container = subprocess.check_output(
            [
                "docker",
                "run",
                "--rm",
                "-d",
                "--memory=1g",
                "-p",
                f"127.0.0.1:{port}:8080",
                "-v",
                f"{import_directory}:/opt/keycloak/data/import:ro",
                IMAGE,
                "start-dev",
                "--import-realm",
                f"--hostname={address}",
            ],
            text=True,
        ).strip()
        try:
            ready = False
            for _ in range(180):
                try:
                    with urllib.request.urlopen(
                        issuer + "/.well-known/openid-configuration", timeout=2
                    ) as response:
                        discovery = json.load(response)
                    if discovery.get("issuer") == issuer:
                        ready = True
                        break
                except (OSError, ValueError):
                    time.sleep(0.5)
            if not ready:
                raise RuntimeError("Disposable OAuth fixture did not become ready")
            env = dict(
                os.environ,
                LINHA_TEST_OAUTH_ISSUER=issuer,
                LINHA_TEST_OAUTH_SECRET_FILE=str(secret_file),
            )
            subprocess.run(
                [
                    "mvn",
                    "-B",
                    "-pl",
                    "sdk/client-jvm",
                    "-am",
                    "test",
                    "-Dtest=OAuthIntegrationTest",
                    "-Dsurefire.failIfNoSpecifiedTests=false",
                ],
                cwd=ROOT,
                env=env,
                check=True,
            )
            print(
                "Real OAuth client credentials: issuance, renewal, stable identity and signature verified"
            )
        finally:
            subprocess.run(
                ["docker", "stop", container], stdout=subprocess.DEVNULL, check=True
            )


if __name__ == "__main__":
    main()
