#!/usr/bin/env python3
"""Release preparation contracts; all mutations are confined to temporary fixtures."""
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
import xml.etree.ElementTree as ET

MODULE = importlib.util.spec_from_file_location(
    "prepare_release", Path(__file__).with_name("prepare-release.py")
)
release = importlib.util.module_from_spec(MODULE)
MODULE.loader.exec_module(release)


def fixture_metadata(root):
    data = {
        "repository": "fixture/linha",
        "groupId": release.text(ET.parse(root / "pom.xml").getroot(), "groupId"),
        "license": {
            "name": "Fixture license",
            "url": "https://example.com/license",
            "spdx": "LicenseRef-Fixture",
        },
        "developer": {
            "id": "fixture",
            "name": "Release Fixture",
            "email": "fixture@example.com",
        },
    }
    (root / "release-metadata.json").write_text(json.dumps(data))
    (root / "LICENSE").write_text("Release test fixture only.\n")
    return data


class ReleasePreparation(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="linha-release-test-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        for path in release.POMS:
            target = self.root / path
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes((release.ROOT / path).read_bytes())
        self.metadata = fixture_metadata(self.root)
        self.version = release.text(
            ET.parse(self.root / "pom.xml").getroot(), "version"
        ).removesuffix("-SNAPSHOT")
        self.tag = "v" + self.version

    def validate(self, tag=None, repository="fixture/linha"):
        return release.validate(self.root, tag or self.tag, repository)

    def test_release_and_prerelease_versions(self):
        for version in [self.version, self.version + "-rc.1"]:
            with self.subTest(version=version):
                self.assertEqual(self.validate("v" + version)[0], version)

    def test_rejects_unsafe_or_nonrelease_tags(self):
        for tag in [
            "main",
            "1.0.0",
            "v01.0.0",
            "v1.0",
            "v1.0.0-SNAPSHOT",
            "v1.0.0+build",
            "v1.0.0\nforged=value",
            "v1.0.0-$(id)",
            "v" + "1" * 120 + ".0.0",
        ]:
            with self.subTest(tag=tag), self.assertRaises(ValueError):
                release.tag_version(tag)

    def test_rejects_mismatched_version_and_repository(self):
        with self.assertRaisesRegex(ValueError, "development version"):
            self.validate("v9999.0.0")
        with self.assertRaisesRegex(ValueError, "repository"):
            self.validate(repository="another/linha")

    def test_missing_metadata_does_not_change_poms(self):
        before = {p: (self.root / p).read_bytes() for p in release.POMS}
        self.metadata["license"]["name"] = ""
        (self.root / "release-metadata.json").write_text(json.dumps(self.metadata))
        with self.assertRaisesRegex(ValueError, "license.name"):
            self.validate()
        self.assertEqual(
            before, {p: (self.root / p).read_bytes() for p in release.POMS}
        )

    def test_requires_license_file(self):
        (self.root / "LICENSE").unlink()
        with self.assertRaisesRegex(ValueError, "LICENSE"):
            self.validate()

    def test_rejects_multiline_output_injection(self):
        self.metadata["license"]["spdx"] = "MIT\nimage_prefix=elsewhere"
        (self.root / "release-metadata.json").write_text(json.dumps(self.metadata))
        with self.assertRaisesRegex(ValueError, "license.spdx"):
            self.validate()

    def test_rejects_mismatched_namespace_or_module(self):
        self.metadata["groupId"] = "com.another"
        (self.root / "release-metadata.json").write_text(json.dumps(self.metadata))
        with self.assertRaisesRegex(ValueError, "groupId"):
            self.validate()
        fixture_metadata(self.root)
        child = self.root / release.POMS[1]
        child.write_text(
            child.read_text().replace("<version>" + self.version, "<version>9.9.9")
        )
        with self.assertRaisesRegex(ValueError, "parent coordinates"):
            self.validate()

    def test_materializes_resolvable_poms_and_literal_metadata(self):
        version, data = self.validate()
        release.prepare(self.root, version, data)
        self.validate()
        for path in release.POMS:
            with self.subTest(path=path):
                project = ET.parse(self.root / path).getroot()
                self.assertEqual(
                    release.text(
                        project, "version" if path == "pom.xml" else "parent/version"
                    ),
                    version,
                )
                self.assertEqual(
                    release.text(project, "url"), "https://github.com/fixture/linha"
                )
                self.assertEqual(release.text(project, "scm/tag"), self.tag)
                self.assertEqual(
                    release.text(project, "licenses/license/name"), "Fixture license"
                )
                self.assertEqual(
                    release.text(project, "developers/developer/email"),
                    "fixture@example.com",
                )
                for dep in project.findall(".//{" + release.NS + "}dependency"):
                    if release.text(dep, "artifactId").startswith("linha-"):
                        self.assertEqual(release.text(dep, "groupId"), data["groupId"])
                        self.assertEqual(
                            release.text(dep, "version"), "${project.version}"
                        )
        before = {p: (self.root / p).read_bytes() for p in release.POMS}
        release.prepare(self.root, version, data)
        self.assertEqual(
            before, {p: (self.root / p).read_bytes() for p in release.POMS}
        )


if __name__ == "__main__":
    unittest.main()
