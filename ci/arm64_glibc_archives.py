#!/usr/bin/env python3
"""Select ARM64 Conan archives that must be rebuilt for the glibc 2.27 baseline."""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
from pathlib import Path
from typing import Callable


PACKAGE_ARCHIVES = (
    ("fontconfig", "libfontconfig.a"),
    ("freetype", "libfreetype.a"),
    ("libde265", "libde265.a"),
    ("libraw", "libraw.a"),
    ("libffi", "libffi.a"),
)
_NEWER_GLIBC_SYMBOL = re.compile(
    r"__isoc23_|__libc_single_threaded|(?:^|\s)fcntl64\s*$",
    re.MULTILINE,
)
_NmRunner = Callable[..., subprocess.CompletedProcess[str]]


def packages_to_rebuild(
    package_root: Path,
    *,
    bootstrap: bool,
    baseline_marker: Path,
    nm_runner: _NmRunner | None = None,
) -> list[str]:
    """Return packages with missing or glibc-incompatible ARM64 archives."""
    if nm_runner is None:
        nm_runner = subprocess.run

    if bootstrap and not baseline_marker.is_file():
        print(
            "[FIX] No repaired ARM64 static-archive checkpoint found; building the "
            "glibc 2.27 set once",
            file=sys.stderr,
        )
        return [package for package, _ in PACKAGE_ARCHIVES]

    selected: list[str] = []
    for package, archive_name in PACKAGE_ARCHIVES:
        archives = sorted(package_root.rglob(archive_name)) if package_root.is_dir() else []
        incompatible = False
        for archive in archives:
            result = nm_runner(
                ["nm", "-u", str(archive)],
                capture_output=True,
                check=False,
                text=True,
            )
            if result.returncode != 0:
                raise RuntimeError(f"Unable to inspect cached static archive: {archive}")
            if _NEWER_GLIBC_SYMBOL.search(result.stdout):
                print(
                    f"[FIX] Cached ARM64 {package} archive requires newer glibc: {archive}",
                    file=sys.stderr,
                )
                incompatible = True
                break

        if incompatible:
            selected.append(package)
        elif not archives and bootstrap:
            print(
                f"[FIX] No cached ARM64 {package} archive found; building it in the "
                "glibc 2.27 / GCC 11 container",
                file=sys.stderr,
            )
            selected.append(package)

    return selected


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("package_root", type=Path)
    parser.add_argument("baseline_marker", type=Path)
    parser.add_argument("bootstrap", choices=("0", "1"))
    args = parser.parse_args()

    for package in packages_to_rebuild(
        args.package_root,
        bootstrap=args.bootstrap == "1",
        baseline_marker=args.baseline_marker,
    ):
        print(package)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
