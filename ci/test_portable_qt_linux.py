"""Regression checks for the Linux portable Qt build's host-test contract."""

from pathlib import Path
import re
import unittest


REPO_ROOT = Path(__file__).resolve().parents[1]
BUILD_SCRIPT = REPO_ROOT / "ci" / "build-portable-qt-linux.sh"
QT_HOST_RECIPE = REPO_ROOT / "qt" / "host" / "conanfile.py"
WORKFLOW = REPO_ROOT / ".github" / "workflows" / "build.yml"


class PortableQtLinuxBuildTests(unittest.TestCase):
    def test_baseline_container_installs_font_config_and_a_font(self):
        script = BUILD_SCRIPT.read_text(encoding="utf-8")

        self.assertIn("fontconfig fonts-dejavu-core", script)

    def test_bionic_container_uses_https_for_the_x86_mirror(self):
        script = BUILD_SCRIPT.read_text(encoding="utf-8")

        self.assertIn("https://us.archive.ubuntu.com/ubuntu", script)
        self.assertIn("archive|security", script)

    def test_x86_bootstrap_installs_ca_before_verified_apt(self):
        script = BUILD_SCRIPT.read_text(encoding="utf-8")

        self.assertIn("Acquire::https::Verify-Peer=false", script)
        self.assertIn(
            "apt_bootstrap_with_timeout install -y --no-install-recommends ca-certificates",
            script,
        )

    def test_qt_host_resolves_brotli_for_pinned_freetype(self):
        recipe = QT_HOST_RECIPE.read_text(encoding="utf-8")

        self.assertIn('self.requires("freetype/2.13.2", override=True)', recipe)
        self.assertIn('self.requires("brotli/1.1.1", override=True, force=True)', recipe)
        workflow = WORKFLOW.read_text(encoding="utf-8")
        self.assertIn('"qt/*:with_brotli=False"', workflow)
        self.assertIn('"qt/*:with_freetype=False"', workflow)
        self.assertIn('"qt/*:with_harfbuzz=False"', workflow)
        self.assertIn('"qt/*:with_fontconfig=False"', workflow)

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
        self.assertIn("F4_CONAN_FORCE_ARM64_LIBRAW_REBUILD", build_step)
        self.assertIn("-e F4_CONAN_FORCE_ARM64_LIBRAW_REBUILD", release_job)
        self.assertIn("F4_CONAN_ALLOW_MISSING_MPG123_REPAIR", build_step)
        self.assertIn("-e F4_CONAN_ALLOW_MISSING_MPG123_REPAIR", release_job)
        self.assertRegex(
            build_step,
            r"F4_CONAN_TRUST_REMOTE_BASELINE:.*inputs\.publish_qt_release",
        )
        self.assertRegex(
            build_step,
            r"F4_CONAN_BOOTSTRAP_MULTIMEDIA:.*inputs\.publish_qt_release",
        )
        self.assertIn("F4_CONAN_FORBID_QT_FFMPEG_BUILD", script)
        self.assertIn("F4_CONAN_FORCE_ARM64_LIBRAW_REBUILD", script)
        self.assertIn("Trusted baseline mode permits only targeted ARM64 libraw/libffi repair", script)
        self.assertIn("F4_CONAN_ALLOW_MISSING_MPG123_REPAIR", script)
        self.assertIn("Trusted baseline mode permits only missing mpg123 repair", script)
        self.assertIn("conan_build_args=(--build='missing:mpg123/*')", script)
        self.assertIn("targeted_baseline_repair", script)
        self.assertIn("Forcing only m4 to rebuild", script)
        self.assertRegex(
            script,
            r"conan_build_args=\(--build='missing:~qt/\*' "
            r"--build='missing:~ffmpeg/\*'\)",
        )

    def test_bootstrap_repairs_glibc_sensitive_parser_tools(self):
        script = BUILD_SCRIPT.read_text(encoding="utf-8")

        for tool in ("bison", "flex"):
            self.assertIn(f"path '*/p/bin/{tool}'", script)
            self.assertIn(f"cached_{tool}_found", script)
            self.assertIn(f"--build='{tool}/*'", script)

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
        self.assertIn(
            "^(F4IconProviderTest|F4IconProviderFractionalDprFramebufferTest|"
            "QtMediaClientTest|F4GalleryVideoControlsPixelGridTest|"
            "F4GalleryVideoSettingsPixelGridTest|WindowGeometryPersistenceTest)$",
            workflow,
        )
        self.assertIn("qmlImportsWithoutInstalledQt", workflow)

    def test_macos_qt_host_uses_the_deterministic_release_gate(self):
        workflow = WORKFLOW.read_text(encoding="utf-8")

        self.assertIn(
            "portable_ctest_regex='^(F4IconProviderTest|"
            "F4IconProviderFractionalDprFramebufferTest|QtMediaClientTest|"
            "F4GalleryVideoControlsPixelGridTest|"
            "F4GalleryVideoSettingsPixelGridTest|WindowGeometryPersistenceTest)$'",
            workflow,
        )
        self.assertIn(
            'ctest --test-dir qt/host/build-ci -C Release --output-on-failure \\\n'
            '              -R "$portable_ctest_regex"',
            workflow,
        )
        self.assertIn("for qt_test_attempt in 1 2; do", workflow)


if __name__ == "__main__":
    unittest.main()
