#!/usr/bin/env python3
"""Convert a Midnight Commander skin (.ini) into an f4 style file.

f4#239: importing mc color themes. This is deliberately a one-off script,
not a live in-app converter or a runtime .ini-format parser: the two
formats do not map 1:1 (f4's theme model is closer to Far/far2l than to
mc's), and the users this actually serves -- people migrating a handful of
mc setups to f4 -- run a conversion once per skin, not repeatedly. Ship a
couple of ready-converted themes for the common case, document the mapping
for anyone converting their own skin, and stop there; see the discussion in
the ticket for why a bigger MVP was not the right call.

Usage:
    python3 scripts/import_mc_theme.py path/to/mc-skin.ini "Display Name"

Prints an f4 style .ini (a [style] + [farcolors] section) to stdout. Redirect
it into internal/theme/styles/<name>.ini to ship it, or into your own
~/.config/f4/styles/<name>.ini to use it locally without a rebuild.

What this converts:
    - mc's [core], [dialog] and [error] sections, mapped to the closest f4
      farcolors keys (internal/theme/colors.go's ColorSlots table has the
      full list of valid keys). Not every mc key has an f4 counterpart --
      widget-local states like "gauge" or "inputhistory" have no equivalent
      concept in f4's model -- those are skipped rather than guessed at.
    - mc's named 16-color palette (black, red, ..., white and their
      "bright" counterparts) via the standard ANSI/VGA console RGB values.
      256-color mc skins (numbers instead of names) are not supported.

What this does NOT convert (by design, not an oversight):
    - [filehighlight] (per-filetype colors): mc's classes (source, doc,
      media, ...) are its own internal, undocumented extension lists, not
      something this script can reproduce faithfully from the skin file
      alone. f4's own per-extension highlighting already ships broad
      Highlight_N rules (see internal/theme/styles/classic.ini) that a
      converted theme keeps using unless you write your own.
    - [editor]/[viewer]/[diffviewer]/[menu]/[popupmenu]/[buttonbar]/[help]:
      lower value for a first pass (see the ticket discussion) and easy to
      extend the CORE_MAP/DIALOG_MAP/ERROR_MAP tables below later.
"""
import configparser
import sys

# Standard 16-color ANSI/VGA console palette. mc skins name colors this way
# (plus a 256-color numeric form this script does not support).
ANSI16 = {
    "black": "#000000",
    "red": "#AA0000",
    "green": "#00AA00",
    "brown": "#AA5500",
    "yellow": "#AA5500",  # mc uses "yellow" for the same dim tone as "brown"
    "blue": "#0000AA",
    "magenta": "#AA00AA",
    "cyan": "#00AAAA",
    "lightgray": "#AAAAAA",
    "gray": "#555555",
    "brightred": "#FF5555",
    "brightgreen": "#55FF55",
    "brightyellow": "#FFFF55",
    "brightblue": "#5555FF",
    "brightmagenta": "#FF55FF",
    "brightcyan": "#55FFFF",
    "white": "#FFFFFF",
}

# mc [core] key -> f4 farcolors canonical key. See internal/theme/colors.go.
CORE_MAP = {
    "_default_": "Panel.Text",
    "selected": "Panel.Cursor",
    "markselect": "Panel.Cursor.Selected",
    "marked": "Panel.Text.Highlight",
    "header": "Panel.Title.Column",
    "frame": "Panel.Box",
    "commandline": "CommandLine",
    "shellprompt": "CommandLine.Prefix",
    "commandlinemark": "CommandLine.Selected",
}

# mc [dialog] key -> f4 farcolors canonical key.
DIALOG_MAP = {
    "_default_": "Dialog.Text",
    "dtitle": "Dialog.Box.Title",
    "dframe": "Dialog.Box",
    "dhotnormal": "Dialog.Text.Highlight",
    "dhotfocus": "Dialog.Text.Highlight",
    "dselnormal": "Dialog.Edit.Selected",
    "dselfocus": "Dialog.Edit.Selected",
}

# mc [error] key -> f4 farcolors canonical key (f4's closest equivalent of
# an error/warning dialog is the WarnDialog.* group).
ERROR_MAP = {
    "_default_": "WarnDialog.Text",
    "errdframe": "WarnDialog.Box",
    "errdtitle": "WarnDialog.Box.Title",
    "errdhotnormal": "WarnDialog.Text.Highlight",
    "errdhotfocus": "WarnDialog.Text.Highlight",
}


def to_hex(name):
    name = name.strip().lower()
    if not name or name == "default":
        return None
    return ANSI16.get(name)


def convert_section(mc, section, key_map, out, seen):
    if section not in mc:
        return
    for mc_key, f4_key in key_map.items():
        if f4_key in seen:
            continue  # first mapping to a given f4 key wins
        raw = mc[section].get(mc_key)
        if not raw:
            continue
        parts = raw.split(";")
        fg = to_hex(parts[0]) if len(parts) > 0 else None
        bg = to_hex(parts[1]) if len(parts) > 1 else None
        if fg is None and bg is None:
            continue
        pieces = []
        if fg:
            pieces.append(f"foreground:{fg}")
        if bg:
            pieces.append(f"background:{bg}")
        out.append(f"{f4_key} = {' | '.join(pieces)}")
        seen.add(f4_key)


def main():
    if len(sys.argv) != 3:
        print(__doc__, file=sys.stderr)
        return 2
    mc = configparser.ConfigParser(strict=False)
    # mc ini values are indented; configparser handles that natively, but
    # some skins repeat a key across a continuation line f4 doesn't need,
    # so keep only the first value per key (default configparser behavior).
    with open(sys.argv[1], encoding="utf-8") as f:
        mc.read_file(f)
    name = sys.argv[2]

    out = []
    seen = set()
    convert_section(mc, "core", CORE_MAP, out, seen)
    convert_section(mc, "dialog", DIALOG_MAP, out, seen)
    convert_section(mc, "error", ERROR_MAP, out, seen)

    print("[style]")
    print(f"Name = {name}")
    print()
    print("[farcolors]")
    print(f"# Converted from an mc skin by scripts/import_mc_theme.py.")
    print(f"# Per-filetype highlighting ([Highlight_N]) is not converted -- see")
    print(f"# the script's docstring -- so this theme keeps f4's own defaults")
    print(f"# for directories/executables/archives/etc. Add your own")
    print(f"# [Highlight_N] sections (see classic.ini for the syntax) to change that.")
    for line in out:
        print(line)
    return 0


if __name__ == "__main__":
    sys.exit(main())
