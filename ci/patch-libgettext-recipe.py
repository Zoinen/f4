#!/usr/bin/env python3
"""Make libgettext's MSVC preprocessor usable by Autoconf."""

from __future__ import annotations

import argparse
from pathlib import Path


_ANCHOR = '            env.define("LD", link)\n'
_CPP_E = 'env.define("CPP", "cl -nologo -E")'
_CXXCPP_E = 'env.define("CXXCPP", "cl -nologo -E")'
_CPP_EP = 'env.define("CPP", "cl -nologo -EP")'
_CXXCPP_EP = 'env.define("CXXCPP", "cl -nologo -EP")'
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
    # A previous CI attempt may have left the recipe export in the Conan
    # package cache with the old /EP form. Normalize it in place instead of
    # inserting a second block whose later /EP definitions would win.
    normalized = text.replace(_CXXCPP_EP, _CXXCPP_E).replace(_CPP_EP, _CPP_E)
    if normalized != text:
        args.recipe.write_text(normalized, encoding="utf-8")
        text = normalized

    if _CPP_E in text or _CXXCPP_E in text:
        if _CPP_E in text and _CXXCPP_E in text:
            return
        raise SystemExit("unexpected libgettext recipe: only one MSVC preprocessor marker is present")
    if _CPP_EP in text or _CXXCPP_EP in text:
        raise SystemExit("unexpected libgettext recipe: legacy MSVC preprocessor marker remains")
    if text.count(_ANCHOR) != 1:
        raise SystemExit("unexpected libgettext recipe: MSVC LD anchor is absent or ambiguous")
    args.recipe.write_text(text.replace(_ANCHOR, _PATCH, 1), encoding="utf-8")


if __name__ == "__main__":
    main()
