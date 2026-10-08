# Native macOS menu shortcuts targeted the wrong panel

**Date:** 2026-10-08
**Files:** `qt/host/src/MacApplicationMenu.mm`, `qt/host/tests/MacApplicationMenuTests.mm`
**Severity:** medium

## Problem

Ctrl+F3/F4 and related panel shortcuts activated the fixed Right menu entry
even while the left panel had focus.

## Root Cause

Both native Left and Right menus display identical key equivalents. AppKit
registers these equivalents and can dispatch their side-specific actions
despite the submenu's `performKeyEquivalent:` override declining them.
Live diagnostics confirmed a Right menu action for Ctrl+F3 with left-panel
focus. Declining the equivalent through NSMenuDelegate did not prevent it.

## Solution

Intercept matching command-menu key-down events with a local NSEvent monitor
and deliver the original event to the focused native responder. Qt/Go then
apply normal context-sensitive shortcut routing. Native shortcut labels and
side-specific mouse menu actions remain intact. Remove the monitor when its
menu owner is destroyed. Diagnostics use the gated native-menu logging category.

## Regression Test

`duplicatedPanelShortcutsReachFocusedResponder` sends a native Ctrl+F3 event
with duplicate Left/Right menu equivalents. It verifies one focused-responder
delivery and no fixed-side action. With forwarding disabled, it fails with
zero deliveries; with the fix enabled, the native menu suite passes.

## Verification

- Native menu tests pass, including existing mouse-action and Settings checks.
- Go core and Qt host builds succeed with video and FFmpeg enabled.
- Live Ctrl+F3/F4/F5 on the left panel and Ctrl+F3/F4/F6 after switching to
  the right panel affect only the active panel.
- Restored the original time sorting directions and left-panel focus.

## Tags

`#macos` `#qt` `#native-menu` `#shortcuts` `#panel` `#regression`
