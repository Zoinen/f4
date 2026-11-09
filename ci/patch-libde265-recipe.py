#!/usr/bin/env python3
"""Keep libde265's optional decoder CLI out of static MSVC packages."""

from __future__ import annotations

import argparse
from pathlib import Path


_ANCHOR = '        tc.variables["ENABLE_SDL"] = False\n'
_PATCH = (
    _ANCHOR
    + '        # f4: the application needs the library, not the decoder CLI.\n'
    + '        tc.variables["ENABLE_DECODER"] = False\n'
)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("recipe", type=Path)
    args = parser.parse_args()

    text = args.recipe.read_text(encoding="utf-8")
    if 'tc.variables["ENABLE_DECODER"] = False' in text:
        return
    if text.count(_ANCHOR) != 1:
        raise SystemExit(
            "unexpected libde265 recipe: CMakeToolchain anchor is absent or ambiguous"
        )
    args.recipe.write_text(text.replace(_ANCHOR, _PATCH), encoding="utf-8")


if __name__ == "__main__":
    main()
