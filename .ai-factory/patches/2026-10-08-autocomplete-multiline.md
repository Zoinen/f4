# Multiline command suggestions overlap adjacent rows

**Date:** 2026-10-08
**Severity:** medium
**Files:** qt/host/qml/AutocompletePopup.qml,
qt/host/tests/F4QuickViewSurfaceTests.cpp

## Problem and root cause

Command history suggestions rendered line breaks inside two side-by-side text
labels, while every delegate retained a single-line height. Continuation lines
overflowed into subsequent items and their prefix offset was inconsistent.

## Solution

Normalize CR/LF and Unicode line/paragraph separators for display only. Render
multiline suggestions with one escaped styled label, preserving prefix coloring,
and size each row from its text height. Retain existing single-line rendering
and original model text for accepting commands. Measure popup width by individual
lines, constrain its height to available space, and use the shared scrollbar.

## Verification

At DPR 1.75 the new regression failed before the fix because the row was smaller
than the text. Afterward explicit line breaks and nonoverlapping geometry pass;
actual multiline text leaves are checked on the scene pixel grid and the window
is rendered. Existing hover-resize, dismissal, and command-preview tests pass.
The test emits [FIX:autocomplete-multiline].

## Prevention

Variable-length text requires matching delegate geometry. Test multiline input,
Unicode separators, escaped markup and narrow-popup wrapping, not only ordinary
single-line suggestions.

## Tags

`#qml` `#command-history` `#autocomplete` `#multiline`
