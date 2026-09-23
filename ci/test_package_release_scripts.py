from __future__ import annotations

import importlib.util
import pathlib
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
                archive.writestr("F4.app/Contents/Resources/qt.conf", "Prefix=.")

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
