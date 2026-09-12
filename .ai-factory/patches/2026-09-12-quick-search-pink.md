# Restore the GUI quick-search accent

## Problem

QML panel matches appeared gray. The GUI Quick Search Match default was the
ordinary text color, and the host unconditionally preferred a console palette
foreground over the GUI theme's dedicated match color.

## Solution

Restore the pink accent (#c678dd), including Reset Defaults. In neutral GUI
file-color mode use the GUI match color; in console file-color mode preserve
the existing semantic palette mapping. Explicit saved theme colors remain
untouched. No text geometry, search spans, or matching logic changes.

## Prevention

Exercise gray semantic input with the GUI theme, live GUI color changes,
switching back to console colors, search dismissal, and default/reset parity.
Rendered label coverage at DPR 1.75 detects loss of the accent independently
of the semantic match ranges. Further coverage can extend theme switching
across marked and inactive panel states.
