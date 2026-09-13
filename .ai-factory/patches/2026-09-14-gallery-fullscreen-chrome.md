# Gallery fullscreen retains workspace tabs

**Date:** 2026-09-14
**Files:** qt/host/qml/ShellSurfaceHost.qml, qt/host/tests/F4QuickViewSurfaceTests.cpp
**Severity:** low

## Problem

Gallery fullscreen still reserved the title/tab strip.

## Root Cause

The viewer was always anchored below the title bar, including when the native
window entered fullscreen. Windowed expanded viewing intentionally keeps tabs.

## Solution

Hide title chrome and anchor the visible gallery to the window top only while
the window is fullscreen. Restore the existing title geometry outside this mode.

## Prevention

The regression first failed because tabs remained visible. At DPR 1.75 it now
checks full-window viewer bounds, a rendered top-edge pixel, tab restoration,
and the restored text/icon leaves' scene coordinates and unit transforms.
Diagnostics log the fullscreen bounds, DPR and tab visibility.

## Tags

`#qml` `#fullscreen` `#gallery` `#window-chrome`
