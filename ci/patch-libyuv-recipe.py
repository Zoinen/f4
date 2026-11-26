#!/usr/bin/env python3
"""Avoid requiring unsupported ARM I8MM assembler support in the Linux baseline."""

from __future__ import annotations

import argparse
from pathlib import Path


_ANCHOR = "    def build(self):\n        cmake = CMake(self)\n"
_PATCHED_BUILD = '''    def build(self):
        # f4: the glibc-2.27 ARM64 toolchain supports dot-product but its
        # assembler does not recognize I8MM. The libyuv ARM64 sources use
        # dot-product instructions, so do not request the unrelated I8MM flag.
        if self.settings.os == "Linux" and self.settings.arch == "armv8":
            cmake_file = os.path.join(self.source_folder, "CMakeLists.txt")
            with open(cmake_file, "r", encoding="utf-8") as source_file:
                cmake_source = source_file.read()
            i8mm_flag = "-march=armv8-a+dotprod+i8mm"
            dotprod_flag = "-march=armv8-a+dotprod"
            if cmake_source.count(i8mm_flag) == 1:
                cmake_source = cmake_source.replace(i8mm_flag, dotprod_flag, 1)
                with open(cmake_file, "w", encoding="utf-8") as source_file:
                    source_file.write(cmake_source)
            elif cmake_source.count(dotprod_flag) != 1:
                raise RuntimeError(
                    "unexpected libyuv CMakeLists.txt: ARM64 dot-product flag is absent or ambiguous"
                )
        cmake = CMake(self)
'''


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("recipe", type=Path)
    args = parser.parse_args()

    text = args.recipe.read_text(encoding="utf-8")
    if "f4: the glibc-2.27 ARM64 toolchain supports dot-product" in text:
        return
    if text.count(_ANCHOR) != 1:
        raise SystemExit(
            "unexpected libyuv recipe: CMake build anchor is absent or ambiguous"
        )
    args.recipe.write_text(text.replace(_ANCHOR, _PATCHED_BUILD), encoding="utf-8")


if __name__ == "__main__":
    main()
