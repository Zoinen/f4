#!/usr/bin/env python3
"""Avoid libiconv's x64 resource object in static Windows ARM64 builds."""

from __future__ import annotations

import argparse
from pathlib import Path


_ANCHOR = "    def _apply_resource_patch(self):\n"
_RESOURCE_BLOCK = (
    '        if self.settings.os == "Windows" and self.settings.arch == "armv8" and not self.options.shared:\n'
    '            resource_makefiles = (\n'
    '                ("lib/Makefile.in", "libiconv.res.lo"),\n'
    '                ("src/Makefile.in", "iconv.res"),\n'
    "            )\n"
    "            for makefile_name, resource_object in resource_makefiles:\n"
    '                makefile_path = os.path.join(self.source_folder, makefile_name)\n'
    '                self.output.info("Skipping the Windows ARM64 resource object for a static package: {}".format(makefile_path))\n'
    "                replace_in_file(\n"
    "                    self,\n"
    "                    makefile_path,\n"
    '                    "OBJECTS_RES_yes = {}".format(resource_object),\n'
    '                    "OBJECTS_RES_yes =",\n'
    "                    strict=True,\n"
    "                )\n"
    "\n"
)
_MARKER = "Skipping the Windows ARM64 resource object for a static package"


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("recipe", type=Path)
    args = parser.parse_args()

    text = args.recipe.read_text(encoding="utf-8")
    if _MARKER in text:
        return
    if text.count(_ANCHOR) != 1:
        raise SystemExit("unexpected libiconv recipe: resource patch anchor is absent or ambiguous")
    args.recipe.write_text(text.replace(_ANCHOR, _ANCHOR + _RESOURCE_BLOCK, 1), encoding="utf-8")


if __name__ == "__main__":
    main()
