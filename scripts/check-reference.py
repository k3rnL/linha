#!/usr/bin/env python3
"""Check reference coverage of public models and configured option surfaces.

This is a structural check, not a proof that descriptions match runtime semantics.
Scala signatures/defaults still require the source review described in the docs.
"""

import argparse
import json
from pathlib import Path
import re

import yaml

ROOT = Path(__file__).resolve().parents[1]
INVENTORY = "scripts/fixtures/reference-coverage.json"


def slug(value):
    return re.sub(r"[^a-z0-9]+", "-", value.lower()).strip("-")


def schema_fields(node, prefix=""):
    """Enumerate declared properties; named references are covered independently."""
    for name, child in node.get("properties", {}).items():
        path = prefix + "." + name if prefix else name
        yield path
        if child.get("type") == "array":
            yield from schema_fields(child.get("items", {}), path + "[]")
        else:
            yield from schema_fields(child, path)
    for operation in ("allOf", "oneOf", "anyOf"):
        for child in node.get(operation, []):
            yield from schema_fields(child, prefix)


def value_fields(node, prefix=""):
    if isinstance(node, dict):
        for name, child in node.items():
            path = prefix + "." + name if prefix else name
            yield path
            yield from value_fields(child, path)
    elif isinstance(node, list):
        for child in node:
            yield from value_fields(child, prefix + "[]")


def struct_fields(name, sources, visiting=()):
    """Read json tags and embedded selected structs; no Go runtime dependency."""
    if name in visiting:
        raise ValueError("recursive embedded reference struct: " + name)
    matches = [
        match
        for source in sources
        for match in re.finditer(
            r"\btype\s+" + re.escape(name) + r"\s+struct\s*\{(.*?)^\}",
            source,
            re.MULTILINE | re.DOTALL,
        )
    ]
    if len(matches) != 1:
        raise ValueError("expected one selected Go struct: " + name)
    body = matches[0].group(1)
    for tag in re.findall(r'json:"([^"]+)"', body):
        field = tag.split(",", 1)[0]
        if field and field != "-":
            yield field
    for embedded in re.findall(r"^\s*([A-Z]\w*)\s*$", body, re.MULTILINE):
        yield from struct_fields(embedded, sources, (*visiting, name))


def environment_names(root, directories):
    names = set()
    for directory in directories:
        for path in (root / directory).glob("*.go"):
            if not path.name.endswith("_test.go"):
                names.update(re.findall(r"\bLINHA_[A-Z0-9_]+\b", path.read_text()))
    # DatabaseURL builds the remaining variable names from this public suffix list.
    config = root / "server/internal/config/config.go"
    if "LINHA_DATABASE_" in names and config.exists():
        match = re.search(r"keys := \[\]string\{([^}]+)\}", config.read_text())
        if not match:
            raise ValueError("cannot read separate database environment suffixes")
        names.remove("LINHA_DATABASE_")
        names.update(
            "LINHA_DATABASE_" + key for key in re.findall(r'"([A-Z_]+)"', match[1])
        )
    return names


def check(root):
    root = Path(root).resolve()
    docs = root / "docs"
    inventory = json.loads((root / INVENTORY).read_text())
    sources = inventory["sources"]
    errors = []
    pages = {}

    def target_text(target):
        file, separator, anchor = target.partition("#")
        path = (docs / file).resolve()
        if not path.is_relative_to(docs) or not path.is_file():
            errors.append("missing/invalid reference page: " + target)
            return ""
        if file not in pages:
            pages[file] = path.read_text()
        text = pages[file]
        if (
            not separator
            or not anchor
            or not re.search(
                r"(?:id=[\"\']"
                + re.escape(anchor)
                + r"[\"\']|\{#"
                + re.escape(anchor)
                + r"\})",
                text,
            )
        ):
            errors.append("missing explicit reference anchor: " + target)
        return text

    def field_target(page, anchor, field):
        text = target_text(page + "#" + anchor)
        row = next(
            (
                line
                for line in text.splitlines()
                if line.startswith("|") and 'id="' + anchor + '"' in line
            ),
            "",
        )
        cells = row.strip("|").split("|")
        labels = re.findall(r"`([^`]+)`", cells[0]) if cells else []
        # A Go DriverSettings field `memory` is displayed as `drivers.memory`
        # in the containing JSON model, while its anchor still scopes the field.
        matches = any(label == field or label.endswith("." + field) for label in labels)
        if len(cells) < 4 or not matches or not all(cell.strip() for cell in cells):
            errors.append("missing field definition/table row: " + page + "#" + anchor)

    def models(mapping, expected, fields):
        for missing in sorted(set(expected) - set(mapping)):
            errors.append("unindexed API model: " + missing)
        for extra in sorted(set(mapping) - set(expected)):
            errors.append("stale API model inventory: " + extra)
        for name in sorted(set(mapping) & set(expected)):
            target = mapping[name]
            target_text(target)
            page, _, prefix = target.partition("#")
            for field in sorted(set(fields(expected[name]))):
                field_target(page, slug(prefix + "-" + field), field)

    api = json.loads((root / sources["api"]).read_text())["components"]["schemas"]
    models(inventory["api"], api, schema_fields)

    go_sources = [
        (root / source).read_text()
        for source in sorted(
            {entry["source"] for entry in inventory.get("go", {}).values()}
        )
    ]
    for name, entry in inventory.get("go", {}).items():
        target_text(entry["target"])
        page, _, anchor = entry["target"].partition("#")
        for field in set(struct_fields(name, go_sources)):
            field_target(
                page, slug(entry.get("field_prefix", anchor) + "-" + field), field
            )

    if inventory.get("helm"):
        page = inventory["helm"]
        schema = json.loads((root / sources["helm_schema"]).read_text())
        values = yaml.safe_load((root / sources["helm_values"]).read_text())
        for field in sorted(set(schema_fields(schema)) | set(value_fields(values))):
            field_target(page, "helm-" + slug(field), field)

    actual_environment = environment_names(root, sources.get("environment", []))
    indexed_environment = inventory.get("environment", {})
    for missing in sorted(actual_environment - set(indexed_environment)):
        errors.append("unindexed server environment variable: " + missing)
    for extra in sorted(set(indexed_environment) - actual_environment):
        errors.append("stale server environment inventory: " + extra)
    for name, target in indexed_environment.items():
        text = target_text(target)
        if "`" + name + "`" not in text:
            errors.append("missing environment definition: " + name)

    for name, entry in inventory.get("sdk", {}).items():
        text = target_text(entry["target"])
        source = (root / entry["source"]).read_text()
        short_name = name.rsplit(".", 1)[-1]
        if not re.search(r"\b" + re.escape(short_name) + r"\b", source):
            errors.append("missing SDK evidence source for: " + name)
        for member in [short_name, *entry.get("members", [])]:
            if not re.search(r"\b" + re.escape(member) + r"\b", text):
                errors.append(
                    "missing SDK type/member definition: " + name + "." + member
                )

    for target in inventory.get("legacy", []):
        target_text(target)

    if not inventory.get("boundaries") or any(
        not reason.strip() for reason in inventory["boundaries"].values()
    ):
        errors.append("reference extension/exclusion boundaries must be explained")
    return sorted(set(errors))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=ROOT)
    args = parser.parse_args()
    try:
        errors = check(args.root)
    except (OSError, ValueError, KeyError, yaml.YAMLError) as error:
        raise SystemExit("reference coverage input error: " + str(error)) from error
    if errors:
        raise SystemExit("\n".join(errors))
    print(
        "PASS: reference models, nested fields, Go supplements, chart values, environment and SDK index"
    )


if __name__ == "__main__":
    main()
