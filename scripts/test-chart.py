#!/usr/bin/env python3
"""Check that OCI publication rejects altered or mismatched release archives."""
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import tarfile
import tempfile
import unittest

MODULE = importlib.util.spec_from_file_location(
    "prepare_chart", Path(__file__).with_name("prepare-chart.py")
)
chart = importlib.util.module_from_spec(MODULE)
MODULE.loader.exec_module(chart)


class ChartPublication(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="linha-chart-test-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.archive = self.root / "linha-1.2.3.tgz"
        self.checksum = self.root / "SHA256SUMS"

    def fixture(
        self, *, name="linha", version="1.2.3", app_version="1.2.3", image_tag="1.2.3"
    ):
        files = {
            "linha/Chart.yaml": {
                "name": name,
                "version": version,
                "appVersion": app_version,
            },
            "linha/values.yaml": {"image": {"tag": image_tag}},
        }
        with tarfile.open(self.archive, "w:gz") as output:
            for filename, document in files.items():
                content = json.dumps(document).encode()
                member = tarfile.TarInfo(filename)
                member.size = len(content)
                output.addfile(member, io.BytesIO(content))
        digest = hashlib.sha256(self.archive.read_bytes()).hexdigest()
        self.checksum.write_text(f"{digest}  {self.archive.name}\n")

    def test_valid_release_archive_is_unchanged(self):
        self.fixture()
        before = self.archive.read_bytes()
        chart.validate_archive(self.archive, "1.2.3", self.checksum)
        self.assertEqual(before, self.archive.read_bytes())

    def test_rejects_corrupted_checksum(self):
        self.fixture()
        self.checksum.write_text("0" * 64 + "  " + self.archive.name + "\n")
        with self.assertRaisesRegex(ValueError, "checksum mismatch"):
            chart.validate_archive(self.archive, "1.2.3", self.checksum)

    def test_rejects_ambiguous_or_missing_checksum(self):
        self.fixture()
        line = self.checksum.read_text()
        for content in [
            "",
            line.replace(self.archive.name, "different.tgz"),
            line + line,
            "invalid  " + self.archive.name,
        ]:
            with self.subTest(content=content):
                self.checksum.write_text(content)
                with self.assertRaisesRegex(ValueError, "exactly one checksum"):
                    chart.validate_archive(self.archive, "1.2.3", self.checksum)

    def test_rejects_wrong_chart_or_release_version(self):
        for settings in [
            {"name": "another"},
            {"version": "1.2.4"},
            {"app_version": "1.2.4"},
            {"image_tag": "latest"},
        ]:
            with self.subTest(settings=settings):
                self.fixture(**settings)
                with self.assertRaises(ValueError):
                    chart.validate_archive(self.archive, "1.2.3", self.checksum)

    def test_rejects_archive_with_missing_metadata(self):
        with tarfile.open(self.archive, "w:gz"):
            pass
        with self.assertRaisesRegex(ValueError, "Invalid chart metadata"):
            chart.validate_archive(self.archive, "1.2.3")

    def test_source_validation_checks_the_default_image_before_packaging(self):
        target = self.root / "deploy/helm/linha"
        target.mkdir(parents=True)
        (target / "Chart.yaml").write_text(
            json.dumps({"name": "linha", "version": "1.2.3", "appVersion": "1.2.3"})
        )
        (target / "values.yaml").write_text(json.dumps({"image": {"tag": "latest"}}))
        with self.assertRaisesRegex(ValueError, "image tag"):
            chart.validate_source(self.root, "1.2.3")

    def test_recovery_requires_published_matching_release(self):
        chart.validate_published_release(
            {"isDraft": False, "tagName": "v1.2.3"}, "v1.2.3"
        )
        for info in [
            {},
            {"isDraft": True, "tagName": "v1.2.3"},
            {"isDraft": False, "tagName": "v1.2.4"},
        ]:
            with self.subTest(info=info), self.assertRaisesRegex(
                ValueError, "published release"
            ):
                chart.validate_published_release(info, "v1.2.3")


if __name__ == "__main__":
    unittest.main()
