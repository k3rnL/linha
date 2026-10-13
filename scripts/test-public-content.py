#!/usr/bin/env python3
"""Public-content guard contracts using only invented identifiers and fixtures."""

import contextlib
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

MODULE = importlib.util.spec_from_file_location(
    "public_content", Path(__file__).with_name("check-public-content.py")
)
guard = importlib.util.module_from_spec(MODULE)
MODULE.loader.exec_module(guard)


class PublicContent(unittest.TestCase):
    def test_optional_and_required_external_configuration(self):
        for raw in (None, "", "[]"):
            self.assertEqual(guard.parse_denylist(raw), frozenset())
            with self.assertRaisesRegex(ValueError, "nonempty JSON array"):
                guard.parse_denylist(raw, required=True)
        self.assertEqual(
            guard.parse_denylist('["INTERNAL-WORKLOAD", "io.example"]', required=True),
            frozenset((b"internal-workload", b"io.example")),
        )

    def test_malformed_configuration_does_not_echo_values(self):
        for raw in (
            "restricted-value",
            '{"unexpected":"restricted-value"}',
            '["restricted-value",null]',
            '["restricted-value",""]',
            '["restricted-value",123]',
            '["restricted-value","bad value"]',
            '["restricted-value","example\\nidentifier"]',
        ):
            with self.subTest(raw=raw), self.assertRaises(ValueError) as failure:
                guard.parse_denylist(raw)
            self.assertNotIn("restricted-value", str(failure.exception))

    def test_case_components_domain_suffix_and_dotted_package_prefix(self):
        denied = guard.parse_denylist(
            '["internal-workload", "corp.example", "io.example"]'
        )
        for line in (
            b"name: INTERNAL-WORKLOAD",
            b"repository/internal-workload/worker",
            b"https://INTERNAL-WORKLOAD.cluster.example/",
            b"https://login.corp.example/realms/demo",
            b"class com.io.example.analytics.Job",
            b"package io.example.jobs",
        ):
            self.assertTrue(guard.contains_private_identifier(line, denied), line)
        for line in (
            b"public-workload",
            b"https://identity.example",
            b"io.examples.Job",
        ):
            self.assertFalse(guard.contains_private_identifier(line, denied), line)
        component = guard.parse_denylist('["internal"]')
        self.assertTrue(
            guard.contains_private_identifier(b"worker_internal_task", component)
        )
        self.assertFalse(guard.contains_private_identifier(b"internalized", component))

    def test_location_only_diagnostics_and_generic_workspace_paths(self):
        denied = guard.parse_denylist('["internal-workload"]')
        self.assertEqual(
            guard.inspect(
                "example.md", b"safe\ninternal-workload secret-value\n", denied
            ),
            ["example.md:2: restricted identifier or personal workspace path"],
        )
        personal = b"/home/" + b"reader/IdeaProjects/demo"
        self.assertEqual(len(guard.inspect("example.md", personal, frozenset())), 1)
        self.assertEqual(guard.inspect("binary", b"\0internal-workload", denied), [])

    def test_generated_directory_cli_fails_closed_only_when_required(self):
        with tempfile.TemporaryDirectory(prefix="linha-content-test-") as temporary:
            root = Path(temporary)
            (root / "output.html").write_text("ordinary output")
            env = dict(os.environ)
            env.pop(guard.DENYLIST_ENV, None)
            command = [
                "python3",
                str(Path(__file__).with_name("check-public-content.py")),
                "--directory",
                str(root),
            ]
            optional = subprocess.run(command, env=env, text=True, capture_output=True)
            self.assertEqual(optional.returncode, 0, optional.stderr)
            required = subprocess.run(
                command + ["--require-denylist"],
                env=env,
                text=True,
                capture_output=True,
            )
            self.assertNotEqual(required.returncode, 0)
            env[guard.DENYLIST_ENV] = '["internal-workload"]'
            (root / "output.html").write_text("internal-workload secret-value")
            restricted = subprocess.run(
                command + ["--require-denylist"],
                env=env,
                text=True,
                capture_output=True,
            )
            self.assertEqual(restricted.returncode, 1)
            self.assertIn("output.html:1:", restricted.stdout)
            self.assertNotIn("internal-workload", restricted.stdout + restricted.stderr)
            self.assertNotIn("secret-value", restricted.stdout + restricted.stderr)

    def test_history_includes_removed_files_and_commit_messages(self):
        with tempfile.TemporaryDirectory(prefix="linha-content-history-") as temporary:
            root = Path(temporary)

            def git(*args):
                return subprocess.run(
                    [
                        "git",
                        "-c",
                        "user.name=Fixture",
                        "-c",
                        "user.email=fixture@example.invalid",
                        "-c",
                        "commit.gpgsign=false",
                        *args,
                    ],
                    cwd=root,
                    check=True,
                    text=True,
                    capture_output=True,
                )

            git("init", "--initial-branch=main")
            (root / "old.md").write_text("internal-workload")
            git("add", "old.md")
            git("commit", "-m", "Initial fixture")
            git("rm", "old.md")
            (root / "new.md").write_text("public")
            git("add", "new.md")
            git("commit", "-m", "Remove internal-workload fixture")
            output = io.StringIO()
            with patch.object(guard, "ROOT", root), patch.dict(
                os.environ, {guard.DENYLIST_ENV: '["internal-workload"]'}
            ), patch(
                "sys.argv",
                ["check-public-content.py", "--history", "--require-denylist"],
            ), contextlib.redirect_stdout(
                output
            ), self.assertRaises(
                SystemExit
            ) as failure:
                guard.main()
            self.assertEqual(failure.exception.code, 1)
            self.assertIn("old.md:1:", output.getvalue())
            self.assertIn("commit ", output.getvalue())
            self.assertNotIn("internal-workload", output.getvalue())


if __name__ == "__main__":
    unittest.main()
