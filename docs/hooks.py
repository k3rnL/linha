"""Render guide versions for the selected release without changing source Markdown."""

import os
from pathlib import Path
import re
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[1]
NAMESPACE = {"m": "http://maven.apache.org/POM/4.0.0"}
SNAPSHOT = (
    ET.parse(ROOT / "pom.xml").getroot().findtext("m:version", namespaces=NAMESPACE)
)
DEVELOPMENT = re.compile(
    r"<!-- development-only:start -->\s*.*?\s*<!-- development-only:end -->", re.DOTALL
)


def on_page_markdown(markdown, page, **_):
    if page.file.src_uri.startswith("releases/"):
        return markdown
    version = os.environ.get("DOCS_VERSION", "dev")
    if version == "dev":
        return re.sub(r"\b[0-9]+\.[0-9]+\.[0-9]+-SNAPSHOT\b", SNAPSHOT, markdown)
    markdown = DEVELOPMENT.sub("", markdown)
    markdown = re.sub(r"\b[0-9]+\.[0-9]+\.[0-9]+-SNAPSHOT\b", version, markdown)
    if page.file.src_uri == "quickstart.md":
        markdown = markdown.replace(
            "git clone https://github.com/k3rnL/linha.git",
            "git clone --branch v" + version + " https://github.com/k3rnL/linha.git",
        )
        markdown = markdown.replace(
            "## 2. Build the server and SDKs",
            "## 2. Build the server and SDKs\n\n"
            "This source tutorial builds the development coordinates recorded in the tag's POMs. "
            "Applications using published SDKs select version `"
            + version
            + "` instead.",
        )
    if page.file.src_uri == "operations.md":
        markdown = markdown.replace(
            "repository: registry.example/linha/server",
            "repository: ghcr.io/k3rnl/linha/server",
        ).replace("tag: development", "tag: " + version)
    return markdown
