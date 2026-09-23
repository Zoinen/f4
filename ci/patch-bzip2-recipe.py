#!/usr/bin/env python3
"""Allow Conan's bzip2/1.0.8 CMake project to run with CMake 4.x."""

from __future__ import annotations

import argparse
from pathlib import Path


_ANCHOR = '        tc.variables["BZ2_BUILD_EXE"] = self.options.build_executable\n'
_PATCH = _ANCHOR + '        tc.variables["CMAKE_POLICY_VERSION_MINIMUM"] = "3.5"\n'


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("recipe", type=Path)
    args = parser.parse_args()

    text = args.recipe.read_text(encoding="utf-8")
    if "CMAKE_POLICY_VERSION_MINIMUM" in text:
        return
    if text.count(_ANCHOR) != 1:
        raise SystemExit("unexpected bzip2 recipe: CMakeToolchain anchor is absent or ambiguous")
    args.recipe.write_text(text.replace(_ANCHOR, _PATCH), encoding="utf-8")


if __name__ == "__main__":
    main()
