# Route Qt panel image paste through the core shortcut

**Date:** 2026-10-20
**Severity:** medium
**Files:** qt/host/src/VtuiGridItem.h, VtuiGridItem.cpp; qt/host/qml/main.qml;
qt/host/tests/F4GalleryPointerTests.cpp, F4OperationsQueueTests.cpp,
F4DocumentSurfaceTests.cpp, F4QuickViewSurfaceTests.cpp; docs/KEYMAP.md

## Problem

Command+V did not save a clipboard image from panels when the visible command
line owned keyboard focus. Mixed image/text clipboard contents lost their image.

## Root Cause

Native grid shortcuts and QML panel forwarding converge on sendClipboardPaste,
which unconditionally serialized only clipboard text. An image-only clipboard
therefore sent no input, bypassing Go's Panel.Paste action and image save workflow.

## Solution

Declare panel-paste ownership independently of command-line focus. For image MIME
data in that context, send a balanced core Ctrl+V press/release. Keep the existing
transactional text protocol for text-only data, terminals, documents and dialogs.
Go retains clipboard decoding, mixed-content choice, naming and persistence.
Disabled terminal input still blocks both paths. Optional F4_TRACE_GRID_INPUT
diagnostics use [FIX:panel-image-paste] and never include clipboard contents.

## Verification

Before the fix the image regression received no protocol input. Afterward native
key events and semantic calls both send balanced shortcuts; mixed text does not
leak into the command line. Text-only multiline paste and non-panel mixed paste
retain their protocol. The QML context test covers focused visible command input,
terminal activity/busy state and a modal dialog. Cocoa and offscreen checks pass
at 175%, as do the Go clipboard save/choice tests and 80 dialog-suite tests.

A neighboring viewer keyboard test accidentally enabled pointer hover on its
standalone hidden grid. Matching main.qml's pointer gate removes unrelated hover
packets without weakening the no-keyboard-input assertion.

## Prevention

Do not reduce rich clipboard data to text before its context owner can inspect
it. Cover pure image, mixed data, multiline text, both focus paths and modal input
gates. Preserve clipboard MIME formats around native-platform tests. Additional
useful coverage is copying an image from several external macOS applications and
testing user-remapped paste chords.

## Tags

`#clipboard` `#image` `#qml` `#macos` `#focus` `#input` `#regression`
