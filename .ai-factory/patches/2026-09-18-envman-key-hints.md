# Environment Manager keyboard legend in Qt

## Cause

Environment Manager painted its key legend, pane titles and divider directly
into the console frame.
The inherited Window semantic projection knew only the child controls, so the
Qt dialog never received the legend or its browse/edit state changes.

## Fix

Export the active parsed legend from managerWindow.SemanticNode as keyHints.
Carry it through the typed ExtUI dialog model and render it below the shared
dialog body. Reuse the console's localized strings and active-mode selection.
Publish paneSplit metadata for the titles, divider column and active pane.

## Regression

The Go test initially failed because keyHints was absent. It now checks list,
edit and return-to-list states. A separate test verifies typed wire projection
without Legacy passthrough. QML coverage checks the footer, wrapping and every
key/label leaf at DPR 1.75, including scene-space translation and unit vectors.
The input origin measured 173.500019 physical pixels immediately after a forced
unsnapped scroll; the required rendered-settle check confirms the existing
ScenePixelAlignment correction runs before assessing the resting geometry.

## Prevention

Custom TUI painting needs an explicit semantic representation for native GUIs.
Do not derive a second shortcut list in QML: publish the owner’s current legend.
