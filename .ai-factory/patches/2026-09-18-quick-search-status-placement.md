# Keep quick search beside the panel status overlay

**Date:** 2026-09-18
**Files:** qt/host/qml/FilePanelView.qml, qt/host/tests/F4QuickViewSurfaceTests.cpp, qt/host/CMakeLists.txt, qt/host/README.md
**Severity:** low

## Problem
The new status card pushed quick search upward because its bottom anchor used the card's top edge.

## Solution
Anchor quick search to the panel's bottom edge. Keep it centered when its right edge fits before the status card with an eight-logical-pixel gap. Otherwise place it immediately left of the card and constrain its width to the available space. Long queries use the existing left elision. Snap the frame, text, icon and caret through their final scene coordinates.

## Verification
- Before the fix, the bottom-alignment regression failed.
- A separate leaf regression at DPR 1.75 recorded panelFastFindText-0 at physical (698.5, 1317.75) before alignment was corrected.
- Both panels pass centered and shifted placement checks through wide/narrow resizing, long queries and changing selection totals.
- Every affected text/image leaf, caret and frame passes scene origin, unit-vector transform, dimensions and clipping checks at DPR 1.75.
- Wide and narrow rendered captures inspected.
- F4PanelStatusOverlayTest, static-QML smoke, Windows import audit and embedded payload tests pass.

## Prevention
Position independent overlays against the panel edge, then handle collisions along the available horizontal space. Test final scene coordinates on actual text/image leaves at fractional scale.

## Tags
`#qt` `#quick-search` `#status` `#layout` `#fractional-dpr`
