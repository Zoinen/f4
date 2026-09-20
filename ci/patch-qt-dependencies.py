#!/usr/bin/env python3
"""Pin Qt's freetype requirement to the version used by harfbuzz."""

from __future__ import annotations

import argparse
from pathlib import Path


_ANCHOR = '            self.requires("freetype/[>=2.13 <3]")\n'
_PATCH = _ANCHOR + '            self.requires("freetype/2.13.2", override=True)\n'
_MARKER = 'self.requires("freetype/2.13.2", override=True)'


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("recipe", type=Path)
    args = parser.parse_args()

    text = args.recipe.read_text(encoding="utf-8")
    if _MARKER in text:
        return
    if text.count(_ANCHOR) != 1:
        raise SystemExit(
            "unexpected Qt recipe: freetype requirement is absent or ambiguous"
        )
    args.recipe.write_text(text.replace(_ANCHOR, _PATCH), encoding="utf-8")


if __name__ == "__main__":
    main()
