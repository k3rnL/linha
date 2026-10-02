#!/usr/bin/env python3
"""Validate a release tag and materialize literal publication POM metadata."""
import argparse
import json
import os
from pathlib import Path
import re
import sys
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[1]
NS = "http://maven.apache.org/POM/4.0.0"
ET.register_namespace("", NS)
ET.register_namespace("xsi", "http://www.w3.org/2001/XMLSchema-instance")
SDK_MODULES = ["sdk/protocol-jvm", "sdk/client-jvm", "sdk/worker-jvm", "sdk/spark-jvm"]
POMS = ["pom.xml"] + [p + "/pom.xml" for p in SDK_MODULES + ["examples/jvm"]]


def tag_version(tag):
    if not re.fullmatch(
        r"v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?",
        tag,
    ):
        raise ValueError(
            "release tag must be vMAJOR.MINOR.PATCH, optionally with a prerelease suffix"
        )
    if "SNAPSHOT" in tag.upper() or len(tag) > 100:
        raise ValueError("snapshot or oversized release tags are not supported")
    return tag[1:]


def find(node, path):
    return node.find("/".join("{" + NS + "}" + part for part in path.split("/")))


def text(node, path):
    value = find(node, path)
    return value.text if value is not None else ""


def validate(root, tag, repository):
    version = tag_version(tag)
    data = json.loads((root / "release-metadata.json").read_text())
    for key in [
        "repository",
        "groupId",
        "license.name",
        "license.url",
        "license.spdx",
        "developer.id",
        "developer.name",
        "developer.email",
    ]:
        value = data
        for part in key.split("."):
            value = value.get(part, "") if isinstance(value, dict) else ""
        if (
            not isinstance(value, str)
            or not value.strip()
            or any(ord(c) < 32 for c in value)
        ):
            raise ValueError("configure " + key + " in release-metadata.json")
    if not re.fullmatch(
        r"[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9_.-]+", data["repository"]
    ):
        raise ValueError("repository must be GitHub owner/repository")
    if repository.lower() != data["repository"].lower():
        raise ValueError("the release repository does not match release-metadata.json")
    if not data["license"]["url"].startswith("https://"):
        raise ValueError("license.url must be an HTTPS URL")
    if not re.fullmatch(r"[^\s@]+@[^\s@]+\.[^\s@]+", data["developer"]["email"]):
        raise ValueError("developer.email must be a public contact email")
    if not (root / "LICENSE").is_file():
        raise ValueError("add the selected project LICENSE before publishing")
    parent = ET.parse(root / "pom.xml").getroot()
    if text(parent, "groupId") != data["groupId"]:
        raise ValueError(
            "groupId must match the parent and SDK POMs; confirm your verified Central namespace"
        )
    declared = text(parent, "version")
    if declared not in (
        version,
        version + "-SNAPSHOT",
        version.split("-")[0] + "-SNAPSHOT",
    ):
        raise ValueError("release tag does not match the Maven development version")
    for path in POMS[1:]:
        child = ET.parse(root / path).getroot()
        if (
            text(child, "parent/groupId") != data["groupId"]
            or text(child, "parent/version") != declared
        ):
            raise ValueError("inconsistent parent coordinates in " + path)
    return version, data


def child(parent, name, value=None):
    node = ET.SubElement(parent, "{" + NS + "}" + name)
    if value is not None:
        node.text = value
    return node


def prepare(root, version, data):
    url = "https://github.com/" + data["repository"]
    for path in POMS:
        tree = ET.parse(root / path)
        project = tree.getroot()
        node = (
            find(project, "version")
            if path == "pom.xml"
            else find(project, "parent/version")
        )
        node.text = version
        for field in ["name", "description", "url", "licenses", "developers", "scm"]:
            existing = find(project, field)
            if existing is not None:
                project.remove(existing)
        artifact = text(project, "artifactId")
        child(project, "name", artifact)
        child(project, "description", "Linha durable job execution: " + artifact)
        child(project, "url", url)
        license_node = child(child(project, "licenses"), "license")
        child(license_node, "name", data["license"]["name"])
        child(license_node, "url", data["license"]["url"])
        child(license_node, "distribution", "repo")
        developer = child(child(project, "developers"), "developer")
        for key in ["id", "name", "email"]:
            child(developer, key, data["developer"][key])
        scm = child(project, "scm")
        child(scm, "connection", "scm:git:" + url + ".git")
        child(
            scm,
            "developerConnection",
            "scm:git:ssh://git@github.com/" + data["repository"] + ".git",
        )
        child(scm, "url", url)
        child(scm, "tag", "v" + version)
        ET.indent(tree, space="  ")
        tree.write(root / path, encoding="UTF-8", xml_declaration=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--tag", required=True)
    parser.add_argument("--repository", required=True)
    parser.add_argument(
        "--check", action="store_true", help="validate without changing POMs"
    )
    parser.add_argument("--root", type=Path, default=ROOT)
    args = parser.parse_args()
    version, data = validate(args.root, args.tag, args.repository)
    if not args.check:
        prepare(args.root, version, data)
    outputs = {
        "version": version,
        "image_prefix": "ghcr.io/" + data["repository"].lower(),
        "license": data["license"]["spdx"],
    }
    if output := os.environ.get("GITHUB_OUTPUT"):
        with open(output, "a") as stream:
            stream.writelines(k + "=" + v + "\n" for k, v in outputs.items())
    print(("Validated" if args.check else "Prepared") + " Linha " + version)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError) as error:
        print("Release configuration: " + str(error), file=sys.stderr)
        sys.exit(1)
