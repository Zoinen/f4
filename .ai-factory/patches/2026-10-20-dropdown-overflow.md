# Bound dropdown height and scroll overflowing menus

## Problem

Column-based sort and renderer popups could exceed the window height. Shared
Qt menus only had a scroll indicator, and combo popups used a fixed height cap
that did not account for small windows.

## Fix

- Limit popup heights to the host window with a small edge inset.
- Use the shared F4 scrollbar for overflowing menu content.
- Share wheel handling for mouse angle deltas and touchpad pixel deltas; consume
  boundary events so the underlying file panel does not scroll.
- Reveal renderer choices after keyboard navigation, not hover, to avoid pointer
  feedback moving the viewport unexpectedly.
- Clip disabled-row hover guards to the menu viewport and exclude its scrollbar.
- Disable ListView highlight movement animation so keyboard jumps reveal their
  target immediately instead of scrolling at the default fixed speed.

## Regression coverage

Qt tests open sort, renderer and text menus in a short window, verify bounded
geometry, wheel and touchpad scrolling, and access to the last item. A synthetic
80-item shared menu verifies keyboard navigation reveals the last item. Existing
text-menu hover and action checks remain enabled. The full offscreen suite passes
at 175% scale (87 passed, 2 opt-in profiling tests skipped).
The focused Cocoa checks also pass at 175% scale (7 passed).
