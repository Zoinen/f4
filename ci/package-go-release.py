#!/usr/bin/env python3
"""Assemble a release from the ordinary Go-only CI artifacts.

Portable Qt launchers and Qt host sidecars are deliberately excluded. This is
the opt-in packager for terminal-only and other non-Qt Go targets.
"""

from __future__ import annotations

import argparse
import pathlib
import shutil


ARCHIVE_SUFFIXES = (".tar.gz", ".zip", ".deb")


def package_go_release(
    input_root: pathlib.Path, output_root: pathlib.Path
) -> list[pathlib.Path]:
    if output_root.exists():
        shutil.rmtree(output_root)
    output_root.mkdir(parents=True)

    for source in sorted(input_root.rglob("f4-*")):
        if not source.is_file() or not source.name.endswith(ARCHIVE_SUFFIXES):
            continue
        relative_parts = source.relative_to(input_root).parts
        artifact = relative_parts[0] if relative_parts else ""
        if artifact.startswith("f4-portable-") or artifact.startswith("f4-qt-"):
            continue

        destination = output_root / source.name
        if destination.exists():
            raise SystemExit(f"duplicate Go release asset: {destination.name}")
        shutil.copy2(source, destination)

    assets = sorted(output_root.iterdir())
    if not assets:
        raise SystemExit("Go release packaging produced no assets")
    return assets


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("input", type=pathlib.Path, help="download-artifact root")
    parser.add_argument("output", type=pathlib.Path, help="release asset directory")
    args = parser.parse_args()

    assets = package_go_release(args.input, args.output)
    print("Go release assets:")
    for asset in assets:
        print(f"  {asset.name} ({asset.stat().st_size} bytes)")


if __name__ == "__main__":
    main()
