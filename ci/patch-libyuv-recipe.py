#!/usr/bin/env python3
"""Keep libyuv's ARM64 kernels compatible with the glibc 2.27 baseline."""

from __future__ import annotations

import argparse
from pathlib import Path


_ANCHOR = "    def build(self):\n        cmake = CMake(self)\n"
_PATCHED_BUILD = '''    def build(self):
        # f4: Linux ARM64 baseline binutils cannot assemble libyuv's inline
        # DotProd/I8MM kernels. Disable those kernels and build the portable
        # implementation instead.
        if self.settings.os == "Linux" and self.settings.arch == "armv8":
            cmake_file = os.path.join(self.source_folder, "CMakeLists.txt")
            with open(cmake_file, "r", encoding="utf-8") as source_file:
                cmake_source = source_file.read()
            advanced_options = (
                "TARGET_COMPILE_OPTIONS(${ly_lib_name}_neon64 PRIVATE "
                "-march=armv8-a+dotprod+i8mm)"
            )
            baseline_options = (
                "TARGET_COMPILE_OPTIONS(${ly_lib_name}_neon64 PRIVATE "
                "-march=armv8-a)"
            )
            if cmake_source.count(advanced_options) == 1:
                cmake_source = cmake_source.replace(
                    advanced_options, baseline_options, 1
                )
            elif cmake_source.count(baseline_options) != 1:
                raise RuntimeError(
                    "unexpected libyuv CMakeLists.txt: ARM64 compile options are absent or ambiguous"
                )
            disable_marker = (
                "# f4: Linux ARM64 baseline disables DotProd/I8MM inline assembly"
            )
            if disable_marker not in cmake_source:
                object_target = (
                    "ADD_LIBRARY( ${ly_lib_name}_common_objects OBJECT "
                    "${ly_common_source_files} )"
                )
                if cmake_source.count(object_target) != 1:
                    raise RuntimeError(
                        "unexpected libyuv CMakeLists.txt: common object target is absent or ambiguous"
                    )
                cmake_source = cmake_source.replace(
                    object_target,
                    disable_marker
                    + chr(10)
                    + "ADD_DEFINITIONS(-DLIBYUV_DISABLE_NEON=1)"
                    + chr(10) * 2
                    + object_target,
                    1,
                )
            with open(cmake_file, "w", encoding="utf-8") as source_file:
                source_file.write(cmake_source)
        cmake = CMake(self)
'''


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("recipe", type=Path)
    args = parser.parse_args()

    text = args.recipe.read_text(encoding="utf-8")
    if "f4: Linux ARM64 baseline binutils cannot assemble" in text:
        return
    if text.count(_ANCHOR) != 1:
        raise SystemExit(
            "unexpected libyuv recipe: CMake build anchor is absent or ambiguous"
        )
    args.recipe.write_text(text.replace(_ANCHOR, _PATCHED_BUILD), encoding="utf-8")


if __name__ == "__main__":
    main()
