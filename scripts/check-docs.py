#!/usr/bin/env python3
"""Verify real docs builds and frozen versions in an isolated temporary repository."""

import hashlib
import html
import importlib.util
import json
import os
import re
from pathlib import Path
import shutil
import subprocess
import tempfile
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[1]


def main():
    for command in ["mkdocs", "mike", "git"]:
        if not shutil.which(command):
            raise SystemExit(command + " is required; install docs/requirements.txt")
    spec = importlib.util.spec_from_file_location(
        "public_content", ROOT / "scripts/check-public-content.py"
    )
    guard = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(guard)
    snapshot = (
        ET.parse(ROOT / "pom.xml")
        .getroot()
        .findtext("{http://maven.apache.org/POM/4.0.0}version")
    )
    with tempfile.TemporaryDirectory(prefix="linha-docs-check-") as temporary:
        checkout = Path(temporary) / "checkout"
        checkout.mkdir()
        # Include local run artifacts to prove MkDocs excludes them.
        shutil.copytree(ROOT / "docs", checkout / "docs")
        shutil.copy2(ROOT / "mkdocs.yml", checkout)
        shutil.copy2(ROOT / "pom.xml", checkout)
        (checkout / "scripts").mkdir()
        shutil.copy2(
            ROOT / "scripts/build-docs-site.py", checkout / "scripts/build-docs-site.py"
        )
        (checkout / "docs/validation/local.log").write_text(
            "private local log fixture\n"
        )
        (checkout / "docs/validation/local.json").write_text('{"local":true}\n')
        environment = dict(os.environ)
        environment.update(
            GIT_AUTHOR_NAME="Documentation fixture",
            GIT_AUTHOR_EMAIL="fixture@example.invalid",
            GIT_COMMITTER_NAME="Documentation fixture",
            GIT_COMMITTER_EMAIL="fixture@example.invalid",
        )

        def run(*args, check=True):
            result = subprocess.run(
                args,
                cwd=checkout,
                env=environment,
                check=False,
                text=True,
                capture_output=True,
            )
            if check and result.returncode:
                raise RuntimeError(result.stdout + result.stderr)
            return result

        run("git", "init", "--initial-branch=main")
        run("git", "add", "docs", "mkdocs.yml", "pom.xml", "scripts")
        run("git", "commit", "-m", "Initial documentation fixture")
        first = run("git", "rev-parse", "HEAD").stdout.strip()

        def build(version, label, source):
            output = Path(temporary) / label
            result = run(
                "python3",
                "scripts/build-docs-site.py",
                "--version",
                version,
                "--source-ref",
                source,
                "--output",
                str(output),
            )
            assert output.is_dir(), result.stdout
            assert not list(output.rglob("*.log"))
            assert not list((output / version / "validation").glob("*.json"))
            violations = []
            for path in output.rglob("*"):
                if path.is_file():
                    violations.extend(
                        guard.inspect(str(path.relative_to(output)), path.read_bytes())
                    )
            assert not violations, "\n".join(violations)
            assert (output / "index.html").is_file()
            assert (output / version / "index.html").is_file()
            assert any(
                "versions.json" in path.read_text()
                for path in (output / version / "assets/javascripts").glob(
                    "bundle.*.min.js"
                )
            )
            return output

        build("dev", "first-dev", first)
        dev = Path(temporary) / "first-dev"
        assert "Development version" in (dev / "dev/index.html").read_text()
        assert snapshot in (dev / "dev/sdk-reference/index.html").read_text()
        release = build("0.0.0", "first-release", first)
        assert "Development version" not in (release / "0.0.0/index.html").read_text()
        release_sdk = (release / "0.0.0/sdk-reference/index.html").read_text()
        assert snapshot not in release_sdk and "0.0.0" in release_sdk
        quickstart_text = html.unescape(
            re.sub(
                r"<[^>]+>", "", (release / "0.0.0/quickstart/index.html").read_text()
            )
        )
        assert "--branch v0.0.0" in quickstart_text
        release_deploy = (release / "0.0.0/operations/index.html").read_text()
        assert "ghcr.io/k3rnl/linha/server" in release_deploy
        assert (
            "current OAuth/API-browser configuration is unreleased"
            not in release_deploy
        )
        frozen = hashlib.sha256((release / "0.0.0/index.html").read_bytes()).hexdigest()
        assert "latest/" in (release / "index.html").read_text()
        (checkout / "docs/index.md").write_text(
            (checkout / "docs/index.md").read_text()
            + "\nNew development fixture content.\n"
        )
        run("git", "add", "docs/index.md")
        run("git", "commit", "-m", "Change development documentation")
        second = run("git", "rev-parse", "HEAD").stdout.strip()
        updated = build("dev", "updated-dev", second)
        assert (
            "New development fixture content."
            in (updated / "dev/index.html").read_text()
        )
        assert (
            hashlib.sha256((updated / "0.0.0/index.html").read_bytes()).hexdigest()
            == frozen
        )
        versions = json.loads((updated / "versions.json").read_text())
        assert {v["version"] for v in versions} == {"dev", "0.0.0"}
        same = build("0.0.0", "repeated-release", first)
        assert (
            hashlib.sha256((same / "0.0.0/index.html").read_bytes()).hexdigest()
            == frozen
        )
        newer = build("0.0.2", "newer-release", second)
        late = build("0.0.1", "late-release", second)
        versions = json.loads((late / "versions.json").read_text())
        assert (
            next(value["version"] for value in versions if "latest" in value["aliases"])
            == "0.0.2"
        )
        assert (
            hashlib.sha256((late / "0.0.0/index.html").read_bytes()).hexdigest()
            == frozen
        )
        assert (late / "0.0.2/index.html").read_bytes() == (
            newer / "0.0.2/index.html"
        ).read_bytes()
        rejected = run(
            "python3",
            "scripts/build-docs-site.py",
            "--version",
            "0.0.0",
            "--source-ref",
            second,
            "--output",
            str(Path(temporary) / "rejected"),
            check=False,
        )
        assert rejected.returncode != 0
        assert "different source commit" in rejected.stderr
        print(
            "PASS: strict docs, private artifact exclusion, version selector, frozen releases, dev updates, release ordering and immutable source identity"
        )


if __name__ == "__main__":
    main()
