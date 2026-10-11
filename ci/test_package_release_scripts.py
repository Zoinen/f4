from __future__ import annotations

import importlib.util
import pathlib
import os
import re
import shutil
import subprocess
import tarfile
import tempfile
import unittest
import zipfile


CI_ROOT = pathlib.Path(__file__).parent


def load_script(name: str, module_name: str):
    spec = importlib.util.spec_from_file_location(module_name, CI_ROOT / name)
    if spec is None or spec.loader is None:
        raise AssertionError(f"cannot load {name}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


QT_PACKAGER = load_script("package-qt-release.py", "package_qt_release")
GO_PACKAGER = load_script("package-go-release.py", "package_go_release")
FULL_PACKAGER = load_script("package-release.py", "package_release")


def write_tar(path: pathlib.Path, member: str) -> None:
    with tarfile.open(path, "w:gz") as archive:
        source = path.with_suffix("")
        source.write_text("placeholder", encoding="utf-8")
        archive.add(source, arcname=member)
        source.unlink()


def write_zip(path: pathlib.Path, member: str) -> None:
    with zipfile.ZipFile(path, "w") as archive:
        archive.writestr(member, "placeholder")


class PackageReleaseScriptsTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory()
        self.root = pathlib.Path(self.tempdir.name) / "dist"
        self.root.mkdir()

    def tearDown(self) -> None:
        self.tempdir.cleanup()

    def add_tar(self, artifact: str, name: str, member: str) -> None:
        directory = self.root / artifact
        directory.mkdir()
        write_tar(directory / name, member)

    def add_zip(self, artifact: str, name: str, member: str) -> None:
        directory = self.root / artifact
        directory.mkdir()
        write_zip(directory / name, member)

    def add_qt_inputs(self) -> None:
        for arch in ("amd64", "arm64"):
            self.add_tar(
                f"f4-portable-linux-{arch}",
                f"f4-linux-{arch}.tar.gz",
                "f4",
            )
            self.add_zip(
                f"f4-portable-windows-{arch}",
                f"f4-windows-{arch}.zip",
                "f4.exe",
            )
            self.add_zip(
                f"f4-qt-darwin-{arch}-app",
                f"f4-qt-darwin-{arch}.app.zip",
                "F4.app/Contents/MacOS/f4",
            )
            app = self.root / f"f4-qt-darwin-{arch}-app" / f"f4-qt-darwin-{arch}.app.zip"
            with zipfile.ZipFile(app, "a") as archive:
                archive.writestr("F4.app/Contents/MacOS/f4-qt-host", "placeholder")
                archive.writestr(
                    "F4.app/Contents/Resources/AppIcon.icns", "placeholder"
                )
                archive.writestr("F4.app/Contents/Resources/qt.conf", "Prefix=.")

    def add_full_inputs(self) -> None:
        self.add_qt_inputs()
        for arch in ("amd64", "arm64"):
            self.add_tar(f"f4-darwin-{arch}", f"f4-darwin-{arch}.tar.gz", "f4")
            self.add_tar(f"f4-linux-{arch}", f"f4-linux-{arch}.tar.gz", "f4/f4")
            # The archive content is checked by tools/releasecheck in CI;
            # this fixture tests that the portable artifact is selected.
            (self.root / f"f4-portable-windows-{arch}" /
             f"f4-windows-{arch}.7z").write_bytes(b"portable Qt fixture")
        for plugin in FULL_PACKAGER.PLUGINS:
            for platform in FULL_PACKAGER.PLUGIN_PLATFORMS:
                stem = f"{plugin}-plugin-{platform}"
                directory = self.root / stem
                directory.mkdir()
                write_tar(directory / (stem + ".tgz"), plugin + "-plugin")

    def test_full_packager_retains_matrix_plugins_and_qt_primary_assets(self) -> None:
        self.add_full_inputs()
        self.add_tar("f4-linux-386", "f4-linux-386.tar.gz", "f4/f4")
        self.add_tar("f4-lite-windows-amd64", "f4-lite-windows-amd64.tar.gz", "f4/f4.exe")
        self.add_zip("f4-windows-amd64", "f4-windows-amd64.zip", "f4/f4.exe")
        (self.root / "f4-windows-amd64" / "f4-windows-amd64.7z").write_bytes(b"terminal fixture")
        self.add_zip("f4-qt-linux-amd64", "f4-qt-linux-amd64.zip", "f4-qt-host")
        output = self.root.parent / "full-release"
        assets = FULL_PACKAGER.package_release(self.root, output)
        names = {asset.name for asset in assets}
        self.assertEqual(len(names), 26)
        self.assertIn("f4-linux-386.tar.gz", names)
        self.assertIn("f4-lite-windows-amd64.tar.gz", names)
        self.assertFalse(any(name.startswith("f4-qt-") for name in names))
        self.assertEqual((output / "f4-windows-amd64.7z").read_bytes(), b"portable Qt fixture")
        FULL_PACKAGER.verify_single_file_archive(output / "f4-windows-amd64.zip", "f4.exe")
        self.assertEqual(FULL_PACKAGER.archive_members(output / "f4-linux-musl-amd64.tar.gz"), ["f4/f4"])
        for plugin in FULL_PACKAGER.PLUGINS:
            for platform in FULL_PACKAGER.PLUGIN_PLATFORMS:
                self.assertIn(f"{plugin}-plugin-{platform}.tgz", names)

    def test_full_packager_rejects_missing_plugin_cell(self) -> None:
        self.add_full_inputs()
        missing = self.root / "ios-plugin-linux-arm64" / "ios-plugin-linux-arm64.tgz"
        missing.unlink()
        with self.assertRaisesRegex(SystemExit, "ios-plugin-linux-arm64.tgz"):
            FULL_PACKAGER.package_release(self.root, self.root.parent / "full-release")

    def test_full_packager_requires_portable_windows_7z(self) -> None:
        self.add_full_inputs()
        (self.root / "f4-portable-windows-amd64" / "f4-windows-amd64.7z").unlink()
        with self.assertRaisesRegex(SystemExit, "f4-portable-windows-amd64"):
            FULL_PACKAGER.package_release(self.root, self.root.parent / "full-release")

    def test_full_packager_requires_app_icon(self) -> None:
        self.add_full_inputs()
        archive_path = self.root / "f4-qt-darwin-amd64-app" / "f4-qt-darwin-amd64.app.zip"
        with zipfile.ZipFile(archive_path, "w") as archive:
            for name in ("MacOS/f4", "MacOS/f4-qt-host", "Resources/qt.conf"):
                archive.writestr("F4.app/Contents/" + name, "fixture")
        with self.assertRaisesRegex(SystemExit, "AppIcon.icns"):
            FULL_PACKAGER.package_release(self.root, self.root.parent / "full-release")

    @unittest.skipUnless(shutil.which("go"), "Go is required for the historical-updater integration check")
    def test_full_matrix_names_pass_the_unmodified_upstream_release_guard(self) -> None:
        self.add_full_inputs()
        # Reuse upstream's historical release fixture, repairing only the
        # documented #1656 names. This catches dropped exotic/lite/legacy
        # assets with the real guard, not a second Python implementation.
        fixture = (CI_ROOT.parent / "internal/update/release_audit_test.go").read_text()
        names = re.search(r"const nightly20260929 = `([^`]+)`", fixture).group(1).split()
        for name in names:
            if "-plugin-" in name or name.endswith(".app.zip"):
                continue
            if name == "f4-lite-windows-amd64.zip":
                name = "f4-lite-windows-amd64.tar.gz"
            stem = name.removesuffix(".tar.gz").removesuffix(".zip").removesuffix(".deb")
            directory = self.root / stem
            directory.mkdir(exist_ok=True)
            path = directory / name
            if not path.exists():
                path.write_bytes(b"matrix fixture; names-only audit")
        output = self.root.parent / "full-release"
        assets = FULL_PACKAGER.package_release(self.root, output)
        listed = self.root.parent / "assets.txt"
        listed.write_text("\n".join(asset.name for asset in assets) + "\n")
        env = {**os.environ, "CGO_ENABLED": "0"}
        env.pop("GOOS", None)
        env.pop("GOARCH", None)
        result = subprocess.run(["go", "run", "./tools/releasecheck", "-listed", str(listed)],
                                cwd=CI_ROOT.parent, env=env, text=True, capture_output=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_stable_and_nightly_use_full_packaging_and_both_guards(self) -> None:
        workflow = (CI_ROOT.parent / ".github/workflows/build.yml").read_text()
        for job in ("release", "nightly"):
            block = workflow.split(f"\n  {job}:\n", 1)[1]
            block = re.split(r"\n  [a-z][a-z-]+:\n", block, maxsplit=1)[0]
            self.assertIn("python3 ci/package-release.py dist release", block)
            self.assertIn("go run ./tools/releasecheck release", block)
            self.assertIn("go run ./tools/releasecheck -listed published-assets.txt", block)
            self.assertIn("--draft=true", block)
            for prerequisite in ("build-batch", "build-lite", "build-win7", "build-reactos",
                                 "build-termux", "build-cloudfox-plugin", "build-android-plugin",
                                 "build-ios-plugin", "qt-host", "portable-qt", "portable-qt-arm64-version", "test", "race"):
                self.assertIn(f"needs.{prerequisite}.result != 'success'", block)

    def test_linux_only_feature_release_is_not_a_stable_channel_bypass(self):
        workflow = (CI_ROOT.parent / ".github/workflows/build.yml").read_text()
        feature = workflow.split("\n  qt-release:\n", 1)[1].split("\n  release:\n", 1)[0]
        self.assertIn("--prerelease", feature)
        self.assertIn("package-qt-release.py dist release --platform linux", feature)

    def test_qt_packager_emits_only_six_qt_assets(self) -> None:
        self.add_qt_inputs()
        self.add_tar("f4-linux-386", "f4-linux-386.tar.gz", "f4")
        self.add_tar("f4-darwin-amd64", "f4-darwin-amd64.tar.gz", "f4")

        output = self.root.parent / "qt-release"
        assets = QT_PACKAGER.package_qt_release(self.root, output)

        self.assertEqual(
            [asset.name for asset in assets],
            [
                "f4-darwin-amd64.app.zip",
                "f4-darwin-arm64.app.zip",
                "f4-linux-amd64.tar.gz",
                "f4-linux-arm64.tar.gz",
                "f4-windows-amd64.zip",
                "f4-windows-arm64.zip",
            ],
        )

    def test_qt_linux_packager_emits_only_amd64_and_arm64_assets(self) -> None:
        for arch in ("amd64", "arm64"):
            self.add_tar(
                f"f4-portable-linux-{arch}",
                f"f4-linux-{arch}.tar.gz",
                "f4",
            )

        output = self.root.parent / "qt-linux-release"
        assets = QT_PACKAGER.package_qt_release(
            self.root,
            output,
            platform="linux",
        )

        self.assertEqual(
            [asset.name for asset in assets],
            ["f4-linux-amd64.tar.gz", "f4-linux-arm64.tar.gz"],
        )

    def test_go_packager_excludes_qt_artifacts(self) -> None:
        self.add_qt_inputs()
        self.add_tar("f4-linux-386", "f4-linux-386.tar.gz", "f4")
        self.add_tar("f4-darwin-amd64", "f4-darwin-amd64.tar.gz", "f4")

        output = self.root.parent / "go-release"
        assets = GO_PACKAGER.package_go_release(self.root, output)

        self.assertEqual(
            [asset.name for asset in assets],
            ["f4-darwin-amd64.tar.gz", "f4-linux-386.tar.gz"],
        )


if __name__ == "__main__":
    unittest.main()
