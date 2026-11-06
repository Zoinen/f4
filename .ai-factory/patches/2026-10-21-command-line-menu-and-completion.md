# Command-line selection and completion anchoring

**Date:** 2026-10-21
**Files:** qt/host/qml/CommandLineView.qml, qt/host/qml/AutocompletePopup.qml, qt/host/tests/F4OperationsQueueTests.cpp
**Severity:** medium

## Problem

Command completion appeared to the right of the actual input. Select All in the
command-line context menu opened panel attributes instead of selecting text.

## Root Cause

Completion translated the Go console input column using Qt font metrics, despite
the Qt prompt having its own measured and clipped width. Select All synthesized
Ctrl+A, which the application routes to the panel attributes action.

## Solution

Register the actual command input viewport in the existing anchor registry and
map its geometry into the popup coordinate space, retaining the legacy fallback
and the separate dialog-completion anchor. Refresh geometry after layout changes.
Select All updates the local selection and sends commandLine.select with UTF-16
source offsets, clearing block preview, without synthesizing a panel key.

## Prevention

Use rendered geometry for native popup anchors, not console-cell assumptions.
Dispatch explicit editing intents when shortcuts can mean different things in
the application. Regression tests verify prompt alignment before and after
resize and selection of Unicode/multiline input with no forwarded shortcut.
Both tests failed before the implementation and pass after it.

## Tags

`#qml` `#autocomplete` `#selection` `#shortcut-routing`
