#!/usr/bin/env python3
"""Assemble the small, user-facing release set from CI artifacts.

The build matrix intentionally produces more artifacts than a release should
expose: sidecar Qt trees are useful for CI diagnostics, while portable Qt
builds are uploaded as single files so they can be tested independently.  A
release must contain archives with stable names and no raw executables or
internal build products.
"""

from __future__ import annotations

import argparse
import pathlib
import shutil
import tarfile
import zipfile


def require_file(path: pathlib.Path) -> pathlib.Path:
    if not path.is_file():
        raise SystemExit(f"release input is missing: {path}")
    return path


def copy_once(source: pathlib.Path, destination: pathlib.Path) -> None:
    if destination.exists():
        raise SystemExit(f"duplicate release asset: {destination.name}")
    shutil.copy2(require_file(source), destination)


def archive_members(path: pathlib.Path) -> list[str]:
    if path.name.endswith(".tar.gz"):
        with tarfile.open(path, "r:gz") as archive:
            return [member.name.rstrip("/") for member in archive.getmembers()]
    with zipfile.ZipFile(path) as archive:
        return [name.rstrip("/") for name in archive.namelist() if name.rstrip("/")]


def verify_single_file_archive(path: pathlib.Path, expected: str) -> None:
    members = archive_members(path)
    if members != [expected]:
        raise SystemExit(
            f"{path.name} must contain only {expected!r}; got {members!r}"
        )


def verify_app_archive(path: pathlib.Path) -> None:
    members = archive_members(path)
    required = {
        "F4.app/Contents/MacOS/f4",
        "F4.app/Contents/MacOS/f4-qt-host",
        "F4.app/Contents/Resources/qt.conf",
    }
    if not required.issubset(members):
        missing = sorted(required.difference(members))
        raise SystemExit(f"{path.name} is missing app-bundle files: {missing}")


def find_artifact_file(root: pathlib.Path, artifact: str, filename: str) -> pathlib.Path:
    return require_file(root / artifact / filename)


def package_release(input_root: pathlib.Path, output_root: pathlib.Path) -> list[pathlib.Path]:
    if output_root.exists():
        shutil.rmtree(output_root)
    output_root.mkdir(parents=True)

    # Linux and Windows use the embedded-Qt single-file launcher.  The names
    # are deliberately the same suffixes consumed by the updater.
    for platform, arch, extension, member in (
        ("linux", "amd64", "tar.gz", "f4"),
        ("linux", "arm64", "tar.gz", "f4"),
        ("windows", "amd64", "zip", "f4.exe"),
        ("windows", "arm64", "zip", "f4.exe"),
    ):
        name = f"f4-{platform}-{arch}.{extension}"
        source = find_artifact_file(
            input_root,
            f"f4-portable-{platform}-{arch}",
            name,
        )
        destination = output_root / name
        copy_once(source, destination)
        verify_single_file_archive(destination, member)

    # macOS's release unit is the complete classic bundle.  Keep the ordinary
    # CLI tarballs too: Homebrew and the existing macOS updater intentionally
    # use those instead of installing a GUI application into bin/.
    for arch in ("amd64", "arm64"):
        app_name = f"f4-darwin-{arch}.app.zip"
        app_source = find_artifact_file(
            input_root,
            f"f4-qt-darwin-{arch}-app",
            f"f4-qt-darwin-{arch}.app.zip",
        )
        app_destination = output_root / app_name
        copy_once(app_source, app_destination)
        verify_app_archive(app_destination)

        cli_name = f"f4-darwin-{arch}.tar.gz"
        cli_source = find_artifact_file(
            input_root,
            f"f4-darwin-{arch}",
            cli_name,
        )
        copy_once(cli_source, output_root / cli_name)

    # Preserve the useful non-desktop targets from the normal Go matrix.  Do
    # not expose Qt sidecar archives, raw portable launchers, or a second copy
    # of a primary asset.  These are all archives users can actually run;
    # keeping them also preserves the existing updater/README URLs.
    primary_names = {path.name for path in output_root.iterdir()}
    for source in sorted(input_root.rglob("f4-*")):
        if not source.is_file():
            continue
        name = source.name
        if name in primary_names or name.startswith("f4-qt-"):
            continue
        if not name.endswith((".tar.gz", ".zip", ".deb")):
            continue
        destination = output_root / name
        if destination.exists():
            continue
        shutil.copy2(source, destination)
        primary_names.add(name)

    assets = sorted(output_root.iterdir())
    if not assets:
        raise SystemExit("release packaging produced no assets")
    return assets


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("input", type=pathlib.Path, help="download-artifact root")
    parser.add_argument("output", type=pathlib.Path, help="release asset directory")
    args = parser.parse_args()

    assets = package_release(args.input, args.output)
    print("Release assets:")
    for asset in assets:
        print(f"  {asset.name} ({asset.stat().st_size} bytes)")


if __name__ == "__main__":
    main()
