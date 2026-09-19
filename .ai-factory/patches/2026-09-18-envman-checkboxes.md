# Semantic Environment Manager profile checkboxes

## Cause

The native list inherited string rows with console [x]/[ ] markers. The custom
console TableRow foreground dimming was not part of its semantic projection.

## Fix

Publish clean profile names and explicit per-row checkbox/dimming metadata.
Handle indexed toggle actions in the owning manager window through the existing
profile persistence path. The Qt list presents acknowledged state with the
shared checkbox style; its viewport owns tap/drag gestures. Dim foreground
content to 75% while preserving the full-strength selection background.

## Regression

The Go baseline failed on console markers in native labels. Tests cover clean
names, literal marker characters, checked/dimmed changes after persistence,
ordinary selection, invalid rows, separators, empty lists and inline-edit mode.
An additional failing regression caught missing keyboard-focus transfer when a
checkbox was clicked from the Add button; the owner now focuses the profile list.
Typed protocol roundtripping is verified separately. Qt tests cover pointer
dispatch, state updates and physical-pixel origins/transforms at DPR 1.75.

## Prevention

Expose presentation state explicitly; do not infer it by parsing labels or make
dimmed rows disabled. The row owner remains responsible for toggles and saving.
