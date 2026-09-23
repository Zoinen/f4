#!/usr/bin/env python3
"""Fix libffi's Windows ARM64 Autotools host triplet."""

from __future__ import annotations

import argparse
from pathlib import Path


_ANCHOR = '        if self.settings_build.compiler == "apple-clang":\n'
_MARKER = 'aarch64-win64-mingw64'
_PATCH = (
    '        if self.settings.os == "Windows" and self.settings.arch == "armv8":\n'
    '            # Conan can infer the x86 triplet for this MSVC/Autotools graph.\n'
    '            # libffi needs an ARM64 host so it selects win64_armasm.S.\n'
    '            tc.update_configure_args({"--host": "aarch64-win64-mingw64"})\n'
    '\n'
    + _ANCHOR
)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("recipe", type=Path)
    args = parser.parse_args()

    text = args.recipe.read_text(encoding="utf-8")
    if _MARKER in text:
        return
    if text.count(_ANCHOR) != 1:
        raise SystemExit("unexpected libffi recipe: AutotoolsToolchain anchor is absent or ambiguous")
    args.recipe.write_text(text.replace(_ANCHOR, _PATCH, 1), encoding="utf-8")


if __name__ == "__main__":
    main()
