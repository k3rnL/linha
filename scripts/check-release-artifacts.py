#!/usr/bin/env python3
"""Build and verify a signed Central bundle without credentials or registry writes."""
from contextlib import contextmanager
from email import policy
from email.parser import BytesParser
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import importlib.util
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import threading
import xml.etree.ElementTree as ET
import zipfile

MODULE = importlib.util.spec_from_file_location(
    "release_tests", Path(__file__).with_name("test-release.py")
)
fixtures = importlib.util.module_from_spec(MODULE)
MODULE.loader.exec_module(fixtures)
release = fixtures.release


def run(command, cwd, env, **kwargs):
    return subprocess.run(command, cwd=cwd, env=env, check=True, **kwargs)


def verify_bundle(bundle, root, env, version, data):
    prefix = data["groupId"].replace(".", "/")
    artifacts = ["linha-jvm"] + [
        release.text(ET.parse(root / module / "pom.xml").getroot(), "artifactId")
        for module in release.SDK_MODULES
    ]
    expected = set()
    for artifact in artifacts:
        base = f"{prefix}/{artifact}/{version}/{artifact}-{version}"
        expected.add(base + ".pom")
        if artifact != "linha-jvm":
            expected.update(
                base + suffix for suffix in [".jar", "-sources.jar", "-javadoc.jar"]
            )
    with zipfile.ZipFile(bundle) as archive:
        files = {p for p in archive.namelist() if not p.endswith("/")}
        actual = {p for p in files if p.endswith((".pom", ".jar"))}
        if actual != expected:
            raise AssertionError(f"Unexpected release artifacts: {actual ^ expected}")
        allowed = expected | {
            p + suffix
            for p in expected
            for suffix in [".asc", ".md5", ".sha1", ".sha256", ".sha512"]
        }
        if files - allowed:
            raise AssertionError(f"Unexpected bundle files: {files - allowed}")
        for name in sorted(expected):
            payload = archive.read(name)
            for algorithm in ["md5", "sha1", "sha256", "sha512"]:
                assert (
                    archive.read(name + "." + algorithm).decode().strip()
                    == hashlib.new(algorithm, payload).hexdigest()
                ), name
            artifact_file = root / "verify-artifact"
            signature = root / "verify-artifact.asc"
            artifact_file.write_bytes(payload)
            signature.write_bytes(archive.read(name + ".asc"))
            run(
                ["gpg", "--batch", "--verify", str(signature), str(artifact_file)],
                root,
                env,
                capture_output=True,
            )
            if name.endswith(".jar"):
                with zipfile.ZipFile(artifact_file) as jar:
                    suffix = (
                        ".scala"
                        if name.endswith("-sources.jar")
                        else ".html" if name.endswith("-javadoc.jar") else ".class"
                    )
                    assert any(p.endswith(suffix) for p in jar.namelist()), name
            else:
                project = ET.fromstring(payload)
                assert (
                    release.text(
                        project, "version" if "linha-jvm/" in name else "parent/version"
                    )
                    == version
                ), name
                for field in [
                    "name",
                    "description",
                    "url",
                    "licenses/license/name",
                    "licenses/license/url",
                    "developers/developer/name",
                    "developers/developer/email",
                    "scm/url",
                    "scm/connection",
                ]:
                    value = release.text(project, field)
                    assert value and "${" not in value, (name, field)
    print(
        f"Verified {len(expected)} release artifacts, signatures, checksums, sources and API docs; examples excluded.",
        flush=True,
    )


@contextmanager
def local_portal(root):
    """Receive the actual publisher upload on loopback and emulate its completion."""

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def do_POST(self):
            size = int(self.headers.get("Content-Length", "0"))
            body = self.rfile.read(size)
            if self.path.startswith("/api/v1/publisher/upload"):
                message = BytesParser(policy=policy.default).parsebytes(
                    (
                        "Content-Type: "
                        + self.headers["Content-Type"]
                        + "\r\nMIME-Version: 1.0\r\n\r\n"
                    ).encode()
                    + body
                )
                bundles = [
                    part.get_payload(decode=True)
                    for part in message.iter_parts()
                    if part.get_filename()
                ]
                if len(bundles) != 1 or (root / "uploaded-bundle.zip").exists():
                    self.send_error(400, "Expected one release bundle")
                    return
                (root / "uploaded-bundle.zip").write_bytes(bundles[0])
                response = b"00000000-0000-0000-0000-000000000001"
                content_type = "text/plain"
            elif self.path.startswith("/api/v1/publisher/status"):
                response = json.dumps(
                    {
                        "deploymentId": "00000000-0000-0000-0000-000000000001",
                        "deploymentName": "fixture",
                        "deploymentState": "PUBLISHED",
                        "purls": [],
                    }
                ).encode()
                content_type = "application/json"
            else:
                self.send_error(404)
                return
            self.send_response(200)
            self.send_header("Content-Type", content_type)
            self.send_header("Content-Length", str(len(response)))
            self.end_headers()
            self.wfile.write(response)

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        yield f"http://127.0.0.1:{server.server_port}"
    finally:
        server.shutdown()
        server.server_close()
        thread.join()


def main():
    with tempfile.TemporaryDirectory(
        prefix="linha-release-bundle-"
    ) as temporary, local_portal(Path(temporary)) as portal:
        root = Path(temporary)
        for path in release.POMS:
            target = root / path
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(release.ROOT / path, target)
        for module in release.SDK_MODULES:
            shutil.copytree(release.ROOT / module / "src", root / module / "src")
        shutil.copytree(release.ROOT / "api", root / "api")
        shutil.copy2(release.ROOT / ".scalafmt.conf", root / ".scalafmt.conf")
        fixtures.fixture_metadata(root)
        version = release.text(
            ET.parse(root / "pom.xml").getroot(), "version"
        ).removesuffix("-SNAPSHOT")
        version, data = release.validate(root, "v" + version, "fixture/linha")
        release.prepare(root, version, data)
        # Only this disposable POM points publishing at our loopback fixture.
        # Real credentials and Maven user settings are never used.
        tree = ET.parse(root / "pom.xml")
        for plugin in tree.getroot().findall(".//{" + release.NS + "}plugin"):
            if release.text(plugin, "artifactId") == "central-publishing-maven-plugin":
                config = release.find(plugin, "configuration")
                release.child(config, "centralBaseUrl", portal)
        tree.write(root / "pom.xml", encoding="UTF-8", xml_declaration=True)
        settings = root / "settings.xml"
        settings.write_text(
            "<settings><servers><server><id>central</id><username>fixture</username><password>fixture</password></server></servers></settings>"
        )
        keyring = root / "keyring"
        keyring.mkdir(mode=0o700)
        env = {
            key: value
            for key, value in os.environ.items()
            if not key.startswith(("MAVEN_GPG_", "MAVEN_CENTRAL_", "GPG_"))
        }
        env["GNUPGHOME"] = str(keyring)
        passphrase = "disposable-release-test-only"
        gpg = ["gpg", "--batch", "--pinentry-mode", "loopback", "--passphrase-fd", "0"]
        try:
            run(
                gpg
                + [
                    "--quick-generate-key",
                    "Linha Release Test <fixture@example.com>",
                    "rsa2048",
                    "sign",
                    "1d",
                ],
                root,
                env,
                input=passphrase,
                text=True,
                capture_output=True,
            )
            key = run(
                gpg + ["--armor", "--export-secret-keys", "fixture@example.com"],
                root,
                env,
                input=passphrase,
                text=True,
                capture_output=True,
            ).stdout
            assert key.startswith("-----BEGIN PGP PRIVATE KEY BLOCK-----")
            env.update(MAVEN_GPG_KEY=key, MAVEN_GPG_PASSPHRASE=passphrase)
            print(
                "Building a disposable signed release bundle for the loopback test receiver...",
                flush=True,
            )
            with (root / "build.log").open("w+") as log:
                try:
                    run(
                        [
                            "mvn",
                            "-B",
                            "--no-transfer-progress",
                            "-s",
                            str(settings),
                            "-gs",
                            str(settings),
                            "-Prelease",
                            "-pl",
                            ",".join(release.SDK_MODULES),
                            "-am",
                            "-Dmaven.install.skip=true",
                            "-Dcentral.autoPublish=true",
                            "-Dcentral.waitUntil=published",
                            "deploy",
                        ],
                        root,
                        env,
                        stdout=log,
                        stderr=subprocess.STDOUT,
                        timeout=900,
                    )
                except subprocess.CalledProcessError:
                    log.seek(0)
                    print("\n".join(log.read().splitlines()[-100:]))
                    raise
            bundle = root / "uploaded-bundle.zip"
            assert bundle.is_file(), "The local Portal fixture did not receive a bundle"
            verify_bundle(bundle, root, env, version, data)
        finally:
            subprocess.run(
                ["gpgconf", "--kill", "gpg-agent"], env=env, capture_output=True
            )


if __name__ == "__main__":
    main()
