#!/usr/bin/env python3
"""Apply the small Qt recipe fixes needed by the portable build matrix."""

from __future__ import annotations

import argparse
from pathlib import Path


_ANCHOR = '            self.requires("freetype/[>=2.13 <3]")\n'
_PATCH = '            self.requires("freetype/2.13.2")\n'
_MARKER = 'self.requires("freetype/2.13.2")'
_HOST_PATH_ANCHOR = (
    '            tc.cache_variables["QT_HOST_PATH"] = '
    'self.dependencies.direct_build["qt"].package_folder\n'
)
_HOST_PATH_PATCH = _HOST_PATH_ANCHOR + (
    '            tc.cache_variables["QT_HOST_PATH_CMAKE_DIR"] = os.path.join(\n'
    '                self.dependencies.direct_build["qt"].package_folder, "lib", "cmake"\n'
    '            )\n'
)
_HOST_PATH_MARKER = 'tc.cache_variables["QT_HOST_PATH_CMAKE_DIR"]'


def _patch_freetype(text: str) -> str:
    if _MARKER in text:
        return text
    if text.count(_ANCHOR) != 1:
        raise SystemExit(
            "unexpected Qt recipe: freetype requirement is absent or ambiguous"
        )
    return text.replace(_ANCHOR, _PATCH)


def _patch_host_path(text: str) -> str:
    if _HOST_PATH_MARKER in text:
        return text
    if text.count(_HOST_PATH_ANCHOR) != 1:
        raise SystemExit(
            "unexpected Qt recipe: cross-build host path is absent or ambiguous"
        )
    return text.replace(_HOST_PATH_ANCHOR, _HOST_PATH_PATCH)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("recipe", type=Path)
    args = parser.parse_args()

    text = args.recipe.read_text(encoding="utf-8")
    patched = _patch_host_path(_patch_freetype(text))
    if patched != text:
        args.recipe.write_text(patched, encoding="utf-8")


if __name__ == "__main__":
    main()
