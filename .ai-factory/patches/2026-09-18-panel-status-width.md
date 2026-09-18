# Fit panel status to its longest row

**Date:** 2026-09-18
**Files:** qt/host/qml/PanelStatusOverlay.qml, qt/host/tests/F4QuickViewSurfaceTests.cpp, qt/host/CMakeLists.txt
**Severity:** low

## Problem
The bottom-right status overlay sometimes wrapped the file-size metric below the file and directory counts despite available panel space.

## Root Cause
The summary used Flow with a separately calculated width. At fractional DPR, a tight width could make Flow wrap even though the intended layout was a single row.

## Solution
Use Row with each metric's natural width. Derive the overlay's width from the larger of the summary's actual implicit width and the disk-space row's natural width, plus padding. Preserve bottom-right anchoring and physical-pixel alignment.

## Verification
- Regression recorded before the fix at DPR 1.75: files physical y=1396, size physical y=1445, overlay width=211.429 logical pixels.
- Regression now passes through 200 count/size combinations, plus default and selected statistics and shrinking after selection is cleared.
- F4PanelStatusOverlayTest passes, including leaf origins, identity unit-vector transforms, rendered captures, and compact status updates.
- Rendered 175% capture inspected; both rows fit and the summary stays on one line.
- Static-QML smoke, Windows import audit, and embedded payload tests pass.

## Prevention
Use a non-wrapping positioner for a row that must remain intact, and derive container width from that row rather than duplicating its layout arithmetic. Test fractional widths across changing values.

## Tags
`#qt` `#status` `#layout` `#fractional-dpr`
