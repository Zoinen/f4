"""Portable Go launcher version and build metadata regressions."""

import importlib.util
from pathlib import Path
import re
import shlex
import unittest


ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("go_build_metadata", ROOT / "ci/go-build-metadata.py")
METADATA = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(METADATA)


class GoBuildMetadataTests(unittest.TestCase):
    def flags(self, **env):
        return shlex.split(METADATA.linker_flags({
            "GITHUB_SHA": "123456789abcdef",
            "F4_BUILD_TIME": "2026-10-10T00:00:00Z",
            **env,
        }))

    def test_tag_wins_over_other_version_sources(self):
        flags = self.flags(GITHUB_REF="refs/tags/v2026.10.10", GITHUB_REF_NAME="v2026.10.10",
                           F4_BUILD_VERSION="old", F4_RELEASE_TAG="other")
        self.assertIn(METADATA.APP_PACKAGE + ".buildVersion=v2026.10.10", flags)

    def test_feature_release_gets_its_explicit_tag(self):
        self.assertIn(METADATA.APP_PACKAGE + ".buildVersion=v2026.10.10",
                      self.flags(F4_RELEASE_TAG="v2026.10.10"))

    def test_nightly_keeps_revision_without_claiming_a_stable_tag(self):
        flags = self.flags(GITHUB_REF="refs/heads/main")
        self.assertFalse(any(".buildVersion=" in flag for flag in flags))
        self.assertIn(METADATA.APP_PACKAGE + ".buildRevision=123456789", flags)

    def test_explicit_fork_metadata_is_preserved(self):
        flags = self.flags(F4_BUILD_REVISION="abcdefghi", F4_BUILD_MODIFIED="true")
        self.assertIn(METADATA.APP_PACKAGE + ".buildRevision=abcdefghi", flags)
        self.assertIn(METADATA.APP_PACKAGE + ".buildModified=true", flags)

    def test_unchecked_version_symbol_fails(self):
        with self.assertRaisesRegex(SystemExit, "unexpected version symbol"):
            self.flags(F4_RELEASE_TAG="v2026.10.10", VERSION_SYMBOL="main.buildVersion")

    def test_linker_fields_are_existing_app_string_variables(self):
        declarations = (ROOT / "internal/app/title.go").read_text()
        for name in ("buildVersion", "buildRevision", "buildModified", "buildTime"):
            self.assertRegex(declarations, rf"\b{name}\s+string\b")

    def test_workflow_literal_version_guard_stays_strict(self):
        workflow = (ROOT / ".github/workflows/build.yml").read_text()
        self.assertEqual(re.findall(r"(?m)^\s*VERSION_SYMBOL:\s*(\S+)\s*$", workflow),
                         [METADATA.APP_PACKAGE + ".buildVersion"])
        for symbol in re.findall(r'-X\s+"?([^\s="]+)=', workflow):
            self.assertEqual(symbol, "$VERSION_SYMBOL")


if __name__ == "__main__":
    unittest.main()
