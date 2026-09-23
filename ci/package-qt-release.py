#!/usr/bin/env python3
"""Assemble exactly the six user-facing Qt desktop release assets."""

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


def package_qt_release(
    input_root: pathlib.Path, output_root: pathlib.Path
) -> list[pathlib.Path]:
    if output_root.exists():
        shutil.rmtree(output_root)
    output_root.mkdir(parents=True)

    # Linux and Windows use one Go launcher containing the matching static Qt
    # host. Keep the stable updater-facing names, but take the files only from
    # the dedicated portable-qt artifacts.
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

    # macOS ships the complete classic bundle, one native Qt build per
    # architecture. Do not add the ordinary Go CLI tarballs here.
    for arch in ("amd64", "arm64"):
        name = f"f4-darwin-{arch}.app.zip"
        source = find_artifact_file(
            input_root,
            f"f4-qt-darwin-{arch}-app",
            f"f4-qt-darwin-{arch}.app.zip",
        )
        destination = output_root / name
        copy_once(source, destination)
        verify_app_archive(destination)

    assets = sorted(output_root.iterdir())
    if len(assets) != 6:
        raise SystemExit(
            f"Qt release must contain exactly six assets; got {[p.name for p in assets]}"
        )
    return assets


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("input", type=pathlib.Path, help="download-artifact root")
    parser.add_argument("output", type=pathlib.Path, help="release asset directory")
    args = parser.parse_args()

    assets = package_qt_release(args.input, args.output)
    print("Qt release assets:")
    for asset in assets:
        print(f"  {asset.name} ({asset.stat().st_size} bytes)")


if __name__ == "__main__":
    main()
