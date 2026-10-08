"""Regression checks for the Linux portable Qt build's host-test contract."""

from pathlib import Path
import re
import unittest


REPO_ROOT = Path(__file__).resolve().parents[1]
BUILD_SCRIPT = REPO_ROOT / "ci" / "build-portable-qt-linux.sh"


class PortableQtLinuxBuildTests(unittest.TestCase):
    def test_baseline_container_installs_font_config_and_a_font(self):
        script = BUILD_SCRIPT.read_text(encoding="utf-8")

        self.assertIn("fontconfig fonts-dejavu-core", script)

    def test_portable_ctest_selection_keeps_multimedia_gates(self):
        script = BUILD_SCRIPT.read_text(encoding="utf-8")
        match = re.search(
            r"portable_ctest_regex='\^\(([^']+)\)\$'",
            script,
        )
        self.assertIsNotNone(match)
        selected_tests = set(match.group(1).split("|"))

        self.assertIn("QtMediaClientTest", selected_tests)
        self.assertIn("F4GalleryVideoControlsPixelGridTest", selected_tests)
        self.assertIn("F4GalleryVideoSettingsPixelGridTest", selected_tests)

        self.assertNotIn("F4QuickViewSurfaceTest", selected_tests)
        self.assertNotIn("F4QtArchitectureCheck", selected_tests)


if __name__ == "__main__":
    unittest.main()
