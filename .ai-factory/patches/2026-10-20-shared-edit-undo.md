# Add shared Go Edit undo/redo and Qt capability transport

**Date:** 2026-10-20
**Severity:** medium
**Files:** third_party/vtui/edit_undo.go, edit.go, edit_undo_test.go,
semantic.go, autocomplete.go; internal/cmdline/qt_semantic.go;
internal/panel/qt_semantic.go, qt_semantic_test.go;
internal/nativeui/scene.go, settings_semantic_test.go; sdk/extui/model.go;
qt/host/qml/CommandLineView.qml, F4TextField.qml, DialogTextField.qml;
qt/host/tests/F4OperationsQueueTests.cpp

## Problem

Undo/Redo were always disabled in Go-owned text context menus because Edit had
no undo history. Qt's read-only projection cannot supply authoritative undo.

## Root Cause

Text mutations were performed through keyboard editing, selection deletion,
insertion, bracketed paste and semantic replacements without a shared change
history. Native Qt history would only change the rendered copy.

## Solution

Bounded Go-owned before/after snapshots restore text, cursor, scrolling and
linear/rectangular selection. Nest transactions so selection deletion followed
by insertion is one operation. Coalesce adjacent typing/deletion. Clear redo
only on real text mutation. Password fields retain no history. SetText establishes
a new baseline; ReplaceText/history/completion acceptance are user operations.
Completion preview neither records an operation nor resets earlier history.

Export optional canUndo/canRedo through typed ExtUI command/control models and
dispatch dedicated semantic actions from Qt menus. Terminal Ctrl+Z,
Ctrl+Shift+Z/Ctrl+Y use the same Edit methods. Diagnostics [FIX:edit-undo] log
stack lengths only, never command text.

Restoring an empty multiline buffer must invalidate both the cache text and
cache width/lines; resetting only its text key can accidentally reuse stale
nonempty ranges for the empty string and panic while painting.

## Prevention

Regression-first keyboard checks failed before implementation. Tests cover
Unicode, selection/cursor, paste, overtype, word deletion, block selection,
notifications, no-ops, history limits, password/disabled fields, completion
preview/accept, command clear, typed capability roundtrips and Qt menu dispatch.
Further useful coverage: validator changes during an undo session and extended
IME composition on macOS.

## Tags

`#go` `#undo` `#redo` `#extui` `#qml` `#command-line` `#multiline`
