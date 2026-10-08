"""Regression tests for ARM64 static-archive glibc compatibility checks."""

from __future__ import annotations

import importlib.util
import pathlib
import subprocess
import tempfile
import unittest
from unittest import mock


SCRIPT = pathlib.Path(__file__).with_name("arm64_glibc_archives.py")
SPEC = importlib.util.spec_from_file_location("arm64_glibc_archives", SCRIPT)
assert SPEC is not None and SPEC.loader is not None
archives = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(archives)


class Arm64GlibcArchiveTests(unittest.TestCase):
    def create_cache_archives(self, package_root: pathlib.Path) -> pathlib.Path:
        libffi_archive = package_root / "pkg-id" / "p" / "lib" / "libffi.a"
        for _, archive_name in archives.PACKAGE_ARCHIVES:
            archive = libffi_archive.with_name(archive_name)
            archive.parent.mkdir(parents=True, exist_ok=True)
            archive.touch()
        return libffi_archive

    def test_first_bootstrap_builds_every_guarded_archive(self) -> None:
        with tempfile.TemporaryDirectory(prefix="f4-arm64-glibc-") as temp_dir:
            package_root = pathlib.Path(temp_dir) / "p"
            marker = package_root / ".f4-arm64-glibc-2.27-video-libraries-ready"

            selected = archives.packages_to_rebuild(
                package_root,
                bootstrap=True,
                baseline_marker=marker,
                nm_runner=mock.Mock(),
            )

        self.assertIn("libffi", selected)

    def test_cached_libffi_with_new_glibc_symbol_is_rebuilt(self) -> None:
        with tempfile.TemporaryDirectory(prefix="f4-arm64-glibc-") as temp_dir:
            package_root = pathlib.Path(temp_dir) / "p"
            marker = package_root / ".f4-arm64-glibc-2.27-video-libraries-ready"
            marker.parent.mkdir(parents=True)
            marker.touch()
            archive = self.create_cache_archives(package_root)

            def fake_nm(command: list[str], **_: object) -> subprocess.CompletedProcess[str]:
                output = "                 U __isoc23_sscanf\n" if command[-1] == str(archive) else ""
                return subprocess.CompletedProcess(command, 0, stdout=output, stderr="")

            selected = archives.packages_to_rebuild(
                package_root,
                bootstrap=True,
                baseline_marker=marker,
                nm_runner=fake_nm,
            )

        self.assertEqual(selected, ["libffi"])

    def test_missing_libffi_after_video_checkpoint_is_rebuilt(self) -> None:
        with tempfile.TemporaryDirectory(prefix="f4-arm64-glibc-") as temp_dir:
            package_root = pathlib.Path(temp_dir) / "p"
            marker = package_root / ".f4-arm64-glibc-2.27-video-libraries-ready"
            marker.parent.mkdir(parents=True)
            marker.touch()
            for _, archive_name in archives.PACKAGE_ARCHIVES:
                if archive_name == "libffi.a":
                    continue
                archive = package_root / "pkg-id" / "p" / "lib" / archive_name
                archive.parent.mkdir(parents=True, exist_ok=True)
                archive.touch()

            selected = archives.packages_to_rebuild(
                package_root,
                bootstrap=True,
                baseline_marker=marker,
                nm_runner=mock.Mock(
                    return_value=subprocess.CompletedProcess(
                        ["nm", "-u"], 0, stdout="", stderr=""
                    )
                ),
            )

        self.assertEqual(selected, ["libffi"])

    def test_clean_libffi_archive_remains_reusable(self) -> None:
        with tempfile.TemporaryDirectory(prefix="f4-arm64-glibc-") as temp_dir:
            package_root = pathlib.Path(temp_dir) / "p"
            marker = package_root / ".f4-arm64-glibc-2.27-video-libraries-ready"
            marker.parent.mkdir(parents=True)
            marker.touch()
            self.create_cache_archives(package_root)

            def fake_nm(command: list[str], **_: object) -> subprocess.CompletedProcess[str]:
                return subprocess.CompletedProcess(command, 0, stdout="", stderr="")

            selected = archives.packages_to_rebuild(
                package_root,
                bootstrap=True,
                baseline_marker=marker,
                nm_runner=fake_nm,
            )

        self.assertEqual(selected, [])


if __name__ == "__main__":
    unittest.main()
