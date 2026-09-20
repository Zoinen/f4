#!/usr/bin/env python3
"""Make libgettext's MSVC preprocessor usable by Autoconf."""

from __future__ import annotations

import argparse
from pathlib import Path


_ANCHOR = '            env.define("LD", link)\n'
_MARKER = 'env.define("CPP", "cl -nologo -E")'
_PATCH = _ANCHOR + (
    '            if is_msvc(self):\n'
    '                # Gnulib needs preprocessor output to find absolute MSVC\n'
    '                # header names when #include_next is unavailable.\n'
    '                env.define("CXXCPP", "cl -nologo -E")\n'
    '                env.define("CPP", "cl -nologo -E")\n'
)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("recipe", type=Path)
    args = parser.parse_args()

    text = args.recipe.read_text(encoding="utf-8")
    if _MARKER in text:
        return
    if text.count(_ANCHOR) != 1:
        raise SystemExit("unexpected libgettext recipe: MSVC LD anchor is absent or ambiguous")
    args.recipe.write_text(text.replace(_ANCHOR, _PATCH, 1), encoding="utf-8")


if __name__ == "__main__":
    main()
