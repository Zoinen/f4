# GUI workspace tabs accept middle-click to close

**Date:** 2026-09-15
**Files:** qt/host/qml/WorkspaceTabs.qml, qt/host/tests/F4QuickViewSurfaceTests.cpp
**Severity:** low

## Problem

Middle-click closed console workspace tabs but did nothing in Qt.

## Root Cause

The tab MouseArea accepted only its default left button and only dispatched activation.

## Solution

Accept left and middle buttons, dispatch the existing close action for middle-click,
and honor the same close capability as the close icon. Action dispatch retains
the existing host diagnostic path. No visual geometry changes are needed.

## Prevention

Exercise real mouse events for both active and inactive tabs, non-closable tabs,
and ordinary left-click activation. The new regression failed before the fix
with zero dispatched actions and passed afterward at 175% scale.

## Tags

`#qml` `#mouse-input` `#workspace` `#frontend-parity`
