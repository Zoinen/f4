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

    def test_required_embedded_qml_gate_runs_without_installed_import_paths(self):
        script = BUILD_SCRIPT.read_text(encoding="utf-8")
        self.assertRegex(script, r'env -u QML_IMPORT_PATH -u QML2_IMPORT_PATH\s*\\\s*'
                         r'"\$\{build_dir\}/F4QuickViewSurfaceTests" qmlImportsWithoutInstalledQt')

    def test_both_linux_container_jobs_receive_tag_metadata(self):
        workflow = WORKFLOW.read_text(encoding="utf-8")
        self.assertEqual(workflow.count("-e GITHUB_REF -e GITHUB_REF_NAME -e GITHUB_SHA -e VERSION_SYMBOL"), 2)
        self.assertEqual(workflow.count("-e F4_RELEASE_TAG"), 2)
        script = BUILD_SCRIPT.read_text(encoding="utf-8")
        self.assertIn('launcher_ldflags="$(python ci/go-build-metadata.py)"', script)
        self.assertIn('bash scripts/check_release_version.sh "${launcher_output}" --version', script)

    def test_desktop_native_ctest_failures_block_release(self):
        workflow = WORKFLOW.read_text(encoding="utf-8")
        self.assertNotIn("continuing to artifact smoke tests", workflow)
        self.assertIn('throw "Qt CTest gate failed: $qtTestStatus"', workflow)
        self.assertIn('exit "$qt_test_status"', workflow)
        self.assertGreaterEqual(workflow.count("^(F4|QtMediaClient|QtShellController|WindowGeometryPersistence)"), 3)


if __name__ == "__main__":
    unittest.main()
