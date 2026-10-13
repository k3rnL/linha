#!/usr/bin/env python3
"""Check public files for workspace paths and externally configured identifiers.

The optional LINHA_PUBLIC_CONTENT_DENYLIST is a JSON array supplied by the caller.
No organization-specific policy is embedded here. Diagnostics report locations
without echoing source text or configured identifiers.
"""

import argparse
import json
import os
import pathlib
import re
import subprocess

ROOT = pathlib.Path(__file__).resolve().parents[1]
DENYLIST_ENV = "LINHA_PUBLIC_CONTENT_DENYLIST"
IDENTIFIER = re.compile(r"[A-Za-z][A-Za-z0-9_.-]*\Z")
PERSONAL_PATH = re.compile(
    rb"/(?:home|Users)/[^/\s]+/(?:IdeaProjects|GoLandProjects|\.codex)(?:/|\b)"
)


def parse_denylist(raw, required=False):
    if not raw:
        if required:
            raise ValueError(
                DENYLIST_ENV + " must contain a nonempty JSON array of identifiers"
            )
        return frozenset()
    try:
        identifiers = json.loads(raw)
    except (TypeError, ValueError):
        raise ValueError(
            DENYLIST_ENV + " must be a JSON array of identifiers"
        ) from None
    if not isinstance(identifiers, list) or any(
        not isinstance(value, str) or not IDENTIFIER.fullmatch(value)
        for value in identifiers
    ):
        raise ValueError(DENYLIST_ENV + " must be a JSON array of nonempty identifiers")
    if required and not identifiers:
        raise ValueError(
            DENYLIST_ENV + " must contain a nonempty JSON array of identifiers"
        )
    return frozenset(value.lower().strip(".-").encode("ascii") for value in identifiers)


def configured_denylist(required=False):
    return parse_denylist(os.environ.get(DENYLIST_ENV), required)


def contains_private_identifier(line, denylist=None):
    if denylist is None:
        denylist = configured_denylist()
    if not denylist:
        return False
    for token in re.findall(rb"[a-zA-Z][a-zA-Z0-9_.-]*", line):
        token = token.lower().strip(b".-")
        candidates = {token}
        candidates.update(re.split(rb"[_.-]", token))
        parts = token.split(b".")
        candidates.update(parts)
        candidates.update(b".".join(parts[i:]) for i in range(len(parts)))
        # A corporate package prefix can be inside a longer dotted class name.
        candidates.update(b".".join(parts[i : i + 2]) for i in range(len(parts) - 1))
        if candidates & denylist:
            return True
    return False


def inspect(path, content, denylist=None):
    if denylist is None:
        denylist = configured_denylist()
    if b"\0" in content:
        return []
    return [
        f"{path}:{number}: restricted identifier or personal workspace path"
        for number, line in enumerate(content.splitlines(), 1)
        if contains_private_identifier(line, denylist) or PERSONAL_PATH.search(line)
    ]


def git(*args):
    return subprocess.check_output(["git", *args], cwd=ROOT)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--history", action="store_true", help="Also audit every reachable commit"
    )
    parser.add_argument(
        "--directory",
        type=pathlib.Path,
        help="Audit a generated directory instead of source files",
    )
    parser.add_argument(
        "--require-denylist",
        action="store_true",
        help="Fail when the external identifier denylist is missing or empty",
    )
    args = parser.parse_args()
    try:
        denylist = configured_denylist(args.require_denylist)
    except ValueError as error:
        parser.error(str(error))
    violations = []
    if args.directory:
        directory = args.directory.resolve(strict=True)
        if not directory.is_dir():
            parser.error("--directory must identify a directory")
        for path in directory.rglob("*"):
            if path.is_file() and ".git" not in path.relative_to(directory).parts:
                violations.extend(
                    inspect(
                        str(path.relative_to(directory)), path.read_bytes(), denylist
                    )
                )
    else:
        files = git(
            "ls-files", "-z", "--cached", "--others", "--exclude-standard"
        ).split(b"\0")
        for value in set(files):
            if not value:
                continue
            path = ROOT / value.decode()
            if path.is_file():
                violations.extend(
                    inspect(str(path.relative_to(ROOT)), path.read_bytes(), denylist)
                )
    commits = 0
    if args.history:
        seen = set()
        for commit in git("rev-list", "--all").decode().splitlines():
            commits += 1
            violations.extend(
                inspect(
                    f"commit {commit[:12]}",
                    git("show", "-s", "--format=%B", commit),
                    denylist,
                )
            )
            for entry in git("ls-tree", "-r", "-z", commit).split(b"\0"):
                if not entry:
                    continue
                metadata, path = entry.split(b"\t", 1)
                _, kind, oid = metadata.split()
                if kind != b"blob" or oid in seen:
                    continue
                seen.add(oid)
                violations.extend(
                    inspect(
                        f"{commit[:12]}:{path.decode()}",
                        git("cat-file", "blob", oid.decode()),
                        denylist,
                    )
                )
    if violations:
        print("\n".join(violations))
        raise SystemExit(1)
    suffix = f" and {commits} reachable commits" if args.history else ""
    print(f"Public content check passed{suffix}.")


if __name__ == "__main__":
    main()
