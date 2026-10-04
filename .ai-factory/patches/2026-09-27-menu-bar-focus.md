# Treat the active menu bar as an input owner before its popup exists

**Date:** 2026-09-27
**Files:** `qt/host/qml/ShellSceneStore.qml`, `qt/host/tests/F4QuickViewSurfaceTests.cpp`
**Severity:** medium

## Problem

After F9 displayed the menu bar, arrow keys still navigated the file panel.

## Root Cause

The shell considered only popup/dialog frames blocking overlays. F9 first
activates a bar without a popup. A compact menu-bar update neither disabled
Gallery input nor transferred focus to the Go key-forwarding grid.

## Solution

Include active menu-bar state in the shared input-blocking predicate and emit
the existing synchronous menu focus notification when that state changes.
Panel cursor painting remains independent of input ownership. Optional
`debugMenuFocus` diagnostics report the boundary without logging input text.

## Verification

The regression delivers bar-only compact updates and sends all four arrows to
the window. Before the fix the forwarding grid received zero arrows; afterward
it receives all four, without panel cursor actions. Two open/close cycles check
focus restoration and retained cursor visibility. Five targeted test functions
pass at DPR 1.75, including the existing menu chrome/pixel-grid regression.

## Prevention

Test input ownership in intermediate UI states, not only after popup creation.
Use observable key receipt rather than a mock transport method that does nothing.
Additional useful coverage: F9 activation while a native command input owns focus.

## Tags

`#qml` `#focus` `#keyboard` `#menu-bar` `#incremental-state`
