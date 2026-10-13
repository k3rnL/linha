#!/usr/bin/env python3
"""Reject known private identifiers and personal workspace paths in public files.

Identifiers are represented by one-way digests so this check does not itself
publish them. It reports locations without echoing source text or secrets.
"""

import argparse
import hashlib
import pathlib
import re
import subprocess

ROOT = pathlib.Path(__file__).resolve().parents[1]
PRIVATE_DIGESTS = set()
PERSONAL_PATH = re.compile(
    rb"/(?:home|Users)/[^/\s]+/(?:IdeaProjects|GoLandProjects|\.codex)(?:/|\b)"
)


def contains_private_identifier(line):
    for token in re.findall(rb"[a-zA-Z][a-zA-Z0-9_.-]*", line):
        token = token.lower().strip(b".-")
        candidates = {token}
        candidates.update(re.split(rb"[_.-]", token))
        parts = token.split(b".")
        candidates.update(b".".join(parts[i:]) for i in range(len(parts)))
        if any(
            hashlib.sha256(value).hexdigest() in PRIVATE_DIGESTS for value in candidates
        ):
            return True
    return False


def inspect(path, content):
    if b"\0" in content:
        return []
    return [
        f"{path}:{number}: private identifier or personal workspace path"
        for number, line in enumerate(content.splitlines(), 1)
        if contains_private_identifier(line) or PERSONAL_PATH.search(line)
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
    args = parser.parse_args()
    violations = []
    if args.directory:
        directory = args.directory.resolve(strict=True)
        if not directory.is_dir():
            parser.error("--directory must identify a directory")
        for path in directory.rglob("*"):
            if path.is_file() and ".git" not in path.relative_to(directory).parts:
                violations.extend(
                    inspect(str(path.relative_to(directory)), path.read_bytes())
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
                    inspect(str(path.relative_to(ROOT)), path.read_bytes())
                )
    commits = 0
    if args.history:
        seen = set()
        for commit in git("rev-list", "--all").decode().splitlines():
            commits += 1
            violations.extend(
                inspect(
                    f"commit {commit[:12]}", git("show", "-s", "--format=%B", commit)
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
                    )
                )
    if violations:
        print("\n".join(violations))
        raise SystemExit(1)
    suffix = f" and {commits} reachable commits" if args.history else ""
    print(f"Public content check passed{suffix}.")


if __name__ == "__main__":
    main()
