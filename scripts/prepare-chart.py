#!/usr/bin/env python3
"""Prepare a matching source chart or verify an existing release archive for OCI."""
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import subprocess
import tarfile

import yaml

MODULE = importlib.util.spec_from_file_location(
    "prepare_release", Path(__file__).with_name("prepare-release.py")
)
release = importlib.util.module_from_spec(MODULE)
MODULE.loader.exec_module(release)
ROOT = Path(__file__).resolve().parents[1]


def validate_archive(archive, version, checksum=None):
    archive = Path(archive)
    if checksum is not None:
        entries = [line.split() for line in Path(checksum).read_text().splitlines()]
        matches = [
            parts[0]
            for parts in entries
            if len(parts) == 2 and parts[1].lstrip("*") == archive.name
        ]
        if len(matches) != 1 or not re.fullmatch(r"[a-fA-F0-9]{64}", matches[0]):
            raise ValueError(
                "SHA256SUMS must contain exactly one checksum for " + archive.name
            )
        if hashlib.sha256(archive.read_bytes()).hexdigest() != matches[0].lower():
            raise ValueError("Release chart SHA-256 checksum mismatch")
    with tarfile.open(archive, "r:gz") as chart:
        documents = {}
        for filename in ["linha/Chart.yaml", "linha/values.yaml"]:
            entries = [
                member for member in chart.getmembers() if member.name == filename
            ]
            if (
                len(entries) != 1
                or not entries[0].isfile()
                or entries[0].size > 1024 * 1024
            ):
                raise ValueError("Invalid chart metadata: " + filename)
            documents[filename] = yaml.safe_load(chart.extractfile(entries[0]))
    metadata = documents["linha/Chart.yaml"]
    if not isinstance(metadata, dict) or any(
        metadata.get(key) != value
        for key, value in {
            "name": "linha",
            "version": version,
            "appVersion": version,
        }.items()
    ):
        raise ValueError("Chart name, version and appVersion must match the release")
    values = documents["linha/values.yaml"]
    if (
        not isinstance(values, dict)
        or not isinstance(values.get("image"), dict)
        or values["image"].get("tag") != version
    ):
        raise ValueError("Default server image tag must match the chart release")


def validate_published_release(data, tag):
    if data.get("isDraft") is not False or data.get("tagName") != tag:
        raise ValueError(
            "Chart recovery requires an existing published release for " + tag
        )


def prepare(tag, repository, output, published=False):
    version = release.tag_version(tag)
    metadata = json.loads((ROOT / "release-metadata.json").read_text())
    if repository.lower() != metadata.get("repository", "").lower():
        raise ValueError("Repository does not match release-metadata.json")
    output = Path(output).resolve()
    output.mkdir(parents=True, exist_ok=True)
    archive = output / f"linha-{version}.tgz"
    if published:
        info = json.loads(
            subprocess.check_output(
                [
                    "gh",
                    "release",
                    "view",
                    tag,
                    "--repo",
                    repository,
                    "--json",
                    "isDraft,tagName",
                ],
                text=True,
            )
        )
        validate_published_release(info, tag)
        subprocess.run(
            [
                "gh",
                "release",
                "download",
                tag,
                "--repo",
                repository,
                "--pattern",
                archive.name,
                "--pattern",
                "SHA256SUMS",
                "--dir",
                str(output),
            ],
            check=True,
        )
        validate_archive(archive, version, output / "SHA256SUMS")
    else:
        release.validate(ROOT, tag, repository)
        subprocess.run(["helm", "lint", str(ROOT / "deploy/helm/linha")], check=True)
        subprocess.run(
            [
                "helm",
                "package",
                str(ROOT / "deploy/helm/linha"),
                "--destination",
                str(output),
            ],
            check=True,
        )
        validate_archive(archive, version)
    owner = repository.split("/")[0].lower()
    return {
        "version": version,
        "archive": str(archive),
        "chart_repository": f"oci://ghcr.io/{owner}/charts",
        "chart_ref": f"oci://ghcr.io/{owner}/charts/linha",
        "package_settings": f"https://github.com/users/{owner}/packages/container/charts%2Flinha/settings",
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--tag", required=True)
    parser.add_argument("--repository", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument(
        "--published",
        action="store_true",
        help="Use the original published release archive",
    )
    args = parser.parse_args()
    result = prepare(args.tag, args.repository, args.output, args.published)
    if os.environ.get("GITHUB_OUTPUT"):
        with open(os.environ["GITHUB_OUTPUT"], "a") as target:
            for name, value in result.items():
                target.write(f"{name}={value}\n")
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    try:
        main()
    except (ValueError, tarfile.TarError) as error:
        raise SystemExit(str(error))
