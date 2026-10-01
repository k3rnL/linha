#!/usr/bin/env python3
"""Format project JSON, Maven XML and non-templated YAML; --check never writes."""
import argparse
import io
import json
from pathlib import Path
from xml.dom import minidom

from ruamel.yaml import YAML
from ruamel.yaml.comments import CommentedMap, CommentedSeq

ROOT = Path(__file__).resolve().parents[1]


def xml_text(text):
    document = minidom.parseString(text)

    def strip_indentation(node):
        for child in list(node.childNodes):
            if child.nodeType == child.TEXT_NODE and not child.data.strip():
                node.removeChild(child)
            elif child.nodeType == child.ELEMENT_NODE:
                strip_indentation(child)

    strip_indentation(document)
    return document.toprettyxml(indent="  ", encoding="UTF-8").decode("UTF-8")


def yaml_text(text):
    yaml = YAML()
    yaml.preserve_quotes = True
    yaml.width = 100
    yaml.indent(mapping=2, sequence=4, offset=2)
    document = yaml.load(text)

    def block_collections(value):
        if isinstance(value, (CommentedMap, CommentedSeq)):
            value.fa.set_block_style()
            children = value.values() if isinstance(value, CommentedMap) else value
            for child in children:
                block_collections(child)

    block_collections(document)
    output = io.StringIO()
    yaml.dump(document, output)
    return output.getvalue()


def files():
    yield from ROOT.glob("pom.xml")
    yield from (ROOT / "sdk").glob("*/pom.xml")
    yield from (ROOT / "examples").glob("*/pom.xml")
    for directory in ["api", "deploy/helm"]:
        yield from (ROOT / directory).rglob("*.json")
    yield from (ROOT / "examples/spark").glob("*.json")
    yield from (ROOT / ".github/workflows").glob("*.yml")
    yield from (ROOT / "deploy/helm/linha").glob("*.yaml")
    yield from (ROOT / "openspec").rglob("*.yaml")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    changed = []
    for path in sorted(set(files())):
        before = path.read_text()
        if path.suffix == ".json":
            after = json.dumps(json.loads(before), indent=2, ensure_ascii=False) + "\n"
        elif path.suffix == ".xml":
            after = xml_text(before)
        else:
            after = yaml_text(before)
        if before == after:
            continue
        changed.append(str(path.relative_to(ROOT)))
        if not args.check:
            path.write_text(after)
    if changed:
        print(
            ("Needs formatting: " if args.check else "Formatted: ") + ", ".join(changed)
        )
    if args.check and changed:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
