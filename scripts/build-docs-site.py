#!/usr/bin/env python3
"""Build one frozen documentation version and export the complete Pages artifact."""

import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parents[1]


def run(*args, **kwargs):
    return subprocess.run(args, cwd=ROOT, check=True, text=True, **kwargs)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True)
    parser.add_argument("--branch", default="docs-pages")
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--source-ref", default="")
    args = parser.parse_args()
    if args.version != "dev" and not re.fullmatch(
        r"[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z][0-9A-Za-z.-]*)?", args.version
    ):
        parser.error("version must be dev or a release version")
    run("git", "check-ref-format", "refs/heads/" + args.branch)
    if args.output.exists():
        parser.error("output directory must not already exist")
    source = (
        args.source_ref
        or run("git", "rev-parse", "HEAD", capture_output=True).stdout.strip()
    )
    exists = (
        subprocess.run(
            ["git", "show-ref", "--verify", "--quiet", "refs/heads/" + args.branch],
            cwd=ROOT,
        ).returncode
        == 0
    )
    versions = (
        json.loads(
            run(
                "mike", "list", "--branch", args.branch, "--json", capture_output=True
            ).stdout
        )
        if exists
        else []
    )
    previous = next((v for v in versions if v["version"] == args.version), None)
    if previous and args.version != "dev":
        if previous.get("properties", {}).get("source_commit") != source:
            parser.error(
                "release documentation already exists for a different source commit"
            )
        print("Reusing frozen documentation version " + args.version)
    else:
        alias = []
        if "-" in args.version:
            alias = ["prerelease"]
        elif args.version != "dev":
            stable_versions = [
                tuple(map(int, value["version"].split(".")))
                for value in versions
                if re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", value["version"])
            ]
            if tuple(map(int, args.version.split("."))) >= max(
                stable_versions, default=(0, 0, 0)
            ):
                alias = ["latest"]
        run(
            "mike",
            "deploy",
            "--branch",
            args.branch,
            "--update-aliases",
            "--prop-set-string",
            "source_commit=" + source,
            args.version,
            *alias,
            env=dict(os.environ, DOCS_VERSION=args.version),
        )
    versions = json.loads(
        run(
            "mike", "list", "--branch", args.branch, "--json", capture_output=True
        ).stdout
    )
    has_latest = any("latest" in v.get("aliases", []) for v in versions)
    has_dev = any(v["version"] == "dev" for v in versions)
    default = "latest" if has_latest else ("dev" if has_dev else args.version)
    run("mike", "set-default", "--branch", args.branch, default)
    args.output.mkdir(parents=True)
    with tempfile.TemporaryDirectory(prefix="linha-docs-export-") as temporary:
        archive = Path(temporary) / "site.tar"
        run("git", "archive", "--format=tar", "--output=" + str(archive), args.branch)
        with tarfile.open(archive) as bundle:
            bundle.extractall(args.output, filter="data")
    (args.output / ".nojekyll").touch()
    print("Pages artifact: " + str(args.output))


if __name__ == "__main__":
    main()
