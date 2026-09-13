# Preserve command-menu shortcuts and mode icons

## Problem and root cause

Native macOS menu projection discarded semantic shortcut and icon fields.
The regression found an empty keyEquivalent for Ctrl+Shift+F3. Side menus also
did not supply the glyph names already used by the path dropdown.

## Solution

Project shortcuts to AppKit key equivalents with explicit physical modifiers.
Keep backend routing authoritative by declining key-equivalent dispatch in
command submenus: duplicate Left/Right hints must not steal active-panel keys.
Preserve unsupported or multiple shortcut strings in the title as a fallback.
Publish matching mode glyph names and render native menu template images via
the existing F4IconProvider. The app-icon popup uses the same raster source.
Opt-in f4.platform.menu diagnostics log [FIX:native-menu] decorations.

## Prevention

Test key/modifier projection and non-interception, native icon loading, semantic
mode glyphs, and app-popup icon/text leaves at DPR 1.75. Exercise user-remapped
shortcuts and both panels during live verification; additional coverage can
expand unusual keyboard layouts and multiple conditional bindings.

## Tags

`#macos` `#menus` `#shortcuts` `#icons` `#regression`
