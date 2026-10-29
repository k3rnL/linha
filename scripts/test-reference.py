#!/usr/bin/env python3
"""Exercise omission/broken-target failures of the reference coverage check."""

import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

MODULE = importlib.util.spec_from_file_location(
    "reference", Path(__file__).with_name("check-reference.py")
)
reference = importlib.util.module_from_spec(MODULE)
MODULE.loader.exec_module(reference)


class ReferenceCoverageTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="linha-reference-test-")
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.schema = {
            "components": {
                "schemas": {
                    "Model": {
                        "properties": {
                            "nested": {"properties": {"value": {"type": "string"}}},
                            "items": {
                                "type": "array",
                                "items": {"properties": {"size": {"type": "integer"}}},
                            },
                        }
                    }
                }
            }
        }
        self.inventory = {
            "sources": {"api": "api.json"},
            "api": {"Model": "models.md#model"},
            "boundaries": {
                "dynamic": "Arbitrary application keys are outside fixed model coverage."
            },
        }
        self.page = "## Model {#model}\n\n" + self.row("model-nested", "nested")
        self.page += self.row("model-nested-value", "nested.value")
        self.page += self.row("model-items", "items")
        self.page += self.row("model-items-size", "items[].size")

    def row(self, anchor, field):
        return (
            f'| <span id="{anchor}"></span>`{field}` | string | optional | Meaning |\n'
        )

    def write(self, path, value):
        target = self.root / path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(value if isinstance(value, str) else json.dumps(value))

    def check(self):
        self.write(reference.INVENTORY, self.inventory)
        self.write("api.json", self.schema)
        self.write("docs/models.md", self.page)
        return reference.check(self.root)

    def test_complete_nested_and_array_fields(self):
        self.assertEqual(self.check(), [])

    def test_new_model_requires_inventory(self):
        self.schema["components"]["schemas"]["NewModel"] = {"type": "object"}
        self.assertIn("unindexed API model: NewModel", self.check())

    def test_new_nested_field_requires_definition(self):
        self.schema["components"]["schemas"]["Model"]["properties"]["nested"][
            "properties"
        ]["newField"] = {}
        self.assertTrue(any("model-nested-newfield" in error for error in self.check()))

    def test_new_array_item_field_requires_definition(self):
        self.schema["components"]["schemas"]["Model"]["properties"]["items"]["items"][
            "properties"
        ]["newField"] = {}
        self.assertTrue(any("model-items-newfield" in error for error in self.check()))

    def test_missing_page(self):
        self.inventory["api"]["Model"] = "missing.md#model"
        self.assertTrue(
            any("missing/invalid reference page" in error for error in self.check())
        )

    def test_missing_anchor(self):
        self.inventory["api"]["Model"] = "models.md#missing"
        self.assertTrue(
            any("missing explicit reference anchor" in error for error in self.check())
        )

    def test_anchor_without_field_table_is_insufficient(self):
        self.page = self.page.replace(
            self.row("model-nested-value", "nested.value"),
            '<span id="model-nested-value"></span>\n',
        )
        self.assertTrue(
            any("missing field definition/table row" in error for error in self.check())
        )

    def test_stale_model_inventory(self):
        self.inventory["api"]["Removed"] = "models.md#model"
        self.assertIn("stale API model inventory: Removed", self.check())

    def test_legacy_anchor_must_survive_reorganization(self):
        self.inventory["legacy"] = ["models.md#missing"]
        self.assertTrue(
            any("missing explicit reference anchor" in error for error in self.check())
        )

    def test_helm_schema_and_defaults_both_require_coverage(self):
        self.inventory["helm"] = "models.md"
        self.inventory["sources"].update(
            helm_values="values.yaml", helm_schema="values.schema.json"
        )
        self.write("values.yaml", {"onlyInValues": True})
        self.write(
            "values.schema.json", {"properties": {"onlyInSchema": {"type": "string"}}}
        )
        errors = self.check()
        self.assertTrue(any("helm-onlyinvalues" in error for error in errors))
        self.assertTrue(any("helm-onlyinschema" in error for error in errors))

    def test_embedded_go_struct_and_new_tag(self):
        self.inventory["go"] = {
            "Input": {"source": "model.go", "target": "models.md#input"}
        }
        self.write(
            "model.go",
            'type Common struct {\n ID string `json:"id"`\n}\ntype Input struct {\n Common\n Hidden string `json:"-"`\n}\n',
        )
        self.page += "\n## Input {#input}\n" + self.row("input-id", "id")
        self.assertEqual(self.check(), [])
        self.write("model.go", 'type Input struct {\n New string `json:"new"`\n}\n')
        self.assertTrue(any("input-new" in error for error in self.check()))

    def test_new_environment_option_requires_index(self):
        self.inventory["sources"]["environment"] = ["server/cmd/linha-server"]
        self.write(
            "server/cmd/linha-server/main.go", 'os.Getenv("LINHA_S3_NEW_OPTION")'
        )
        self.assertIn(
            "unindexed server environment variable: LINHA_S3_NEW_OPTION", self.check()
        )

    def test_sdk_member_and_source_evidence(self):
        self.inventory["sdk"] = {
            "Client": {
                "source": "Client.scala",
                "target": "models.md#model",
                "members": ["submit"],
            }
        }
        self.write("Client.scala", "class Client { def submit(): Unit = () }")
        self.assertTrue(
            any("missing SDK type/member definition" in error for error in self.check())
        )

    def test_boundaries_require_explanations(self):
        self.inventory["boundaries"]["dynamic"] = ""
        self.assertTrue(
            any("boundaries must be explained" in error for error in self.check())
        )


if __name__ == "__main__":
    unittest.main()
