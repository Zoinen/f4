"""Regression checks for the Linux portable Qt build's host-test contract."""

from pathlib import Path
import re
import unittest


REPO_ROOT = Path(__file__).resolve().parents[1]
BUILD_SCRIPT = REPO_ROOT / "ci" / "build-portable-qt-linux.sh"
WORKFLOW = REPO_ROOT / ".github" / "workflows" / "build.yml"


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

    def test_linux_release_repairs_arm64_baseline_without_building_qt_or_ffmpeg(self):
        script = BUILD_SCRIPT.read_text(encoding="utf-8")
        workflow = WORKFLOW.read_text(encoding="utf-8")
        release_job = workflow.split("  portable-qt-linux:", 1)[1].split(
            "  build-batch:", 1
        )[0]
        build_step = release_job.split(
            "    - name: Build the glibc 2.27 portable executable", 1
        )[1].split("      run:", 1)[0]

        self.assertIn("inputs.publish_qt_release", release_job)
        self.assertIn("F4_CONAN_FORBID_QT_FFMPEG_BUILD", build_step)
        self.assertIn("-e F4_CONAN_FORBID_QT_FFMPEG_BUILD", release_job)
        self.assertRegex(
            build_step,
            r"F4_CONAN_TRUST_REMOTE_BASELINE:.*inputs\.publish_qt_release",
        )
        self.assertRegex(
            build_step,
            r"F4_CONAN_BOOTSTRAP_MULTIMEDIA:.*inputs\.publish_qt_release",
        )
        self.assertIn("F4_CONAN_FORBID_QT_FFMPEG_BUILD", script)
        self.assertRegex(
            script,
            r"conan_build_args=\(--build='missing:~qt/\*' "
            r"--build='missing:~ffmpeg/\*'\)",
        )


if __name__ == "__main__":
    unittest.main()
