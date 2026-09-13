# History selection clipped by native menu geometry

**Date:** 2026-09-13
**Severity:** medium
**Files:** qt/host/qml/SemanticMenuPopup.qml, qt/host/tests/F4QuickViewSurfaceTests.cpp

## Problem
The last selected history item was below the visible menu body.

## Root Cause
The GUI positioned the list using the console's top index, ignoring the native title and row metrics. The failing 175% regression measured the last row bottom at 805.286 against a viewport height of 553.143.

## Solution
Honor the console top hint, then contain the selected row within the actual native viewport on opening, resizing and keyboard selection changes. Scroll-only acknowledgements do not reveal the unchanged cursor.

## Prevention
Test native row bounds independently of console viewHeight, including long titled lists, keyboard state patches, manual scroll acknowledgements, pixel-aligned text and screenshots at 175%.

## Tags
`#qml` `#history` `#scrolling` `#semantic-ui`
