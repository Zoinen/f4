# Avoid panel zoom and invert-selection shortcut collision

## Problem and Root Cause
The Qt panel router claimed Alt+Equal for density adjustment before the core's
Panel.InvertSelection action could receive it.

## Solution
Require Qt Control+Alt for panel zoom: Command+Option on macOS, Ctrl+Alt elsewhere.
Preserve Shift for Plus, keypad operators and zero to reset. Alt-only operators
and both halves of forwarded selection shortcuts remain Go-owned. Viewer zoom
is unchanged. Existing opt-in tracing uses [FIX:panel-command-option-zoom].

## Verification and Prevention
The key-routing regression first failed because Command+Option+Equal did not
change density. It also verifies Plus, Minus and reset in masonry and Details,
Alt-only forwarding with matching releases, and other commander modifiers.
Keep platform modifier semantics explicit: Qt Control maps to macOS Command.
Additional useful coverage is physical keyboard testing on non-US layouts.

Tags: #qt #qml #keyboard #selection #zoom #regression
