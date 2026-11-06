# Unify text editing menus without duplicate Qt context popups

**Date:** 2026-10-20
**Severity:** medium
**Files:** qt/host/qml/TextEditContextMenu.qml, F4TextField.qml,
DialogTextField.qml, CommandLineView.qml, FilePanelChrome.qml;
qt/host/cmake/F4HostQmlFiles.cmake; qt/host/tests/F4OperationsQueueTests.cpp;
third_party/ZoinGallery/ZGStyle/TextField.qml, qml/PathControl.qml

## Problem

The command line lacked an edit context menu. Path fields first opened Qt's
default black menu, followed by an independent ZoinGallery menu. Field menus
did not share the application's existing dropdown appearance or icon system.

## Root Cause

Qt 6.9+ adds a default attached ContextMenu to TextField. ZoinGallery also
manually opened its own Menu on release. Command-line painting and selection
were read-only Qt projections, so native text editing could not serve as the
authoritative edit handler.

## Solution

Use one host-owned, in-window menu with panel dropdown geometry/colours and
Lucide icons. Disable the underlying Qt default on ZoinGallery fields and pass
an optional host callback through PathControl; standalone fallback is retained.
Native fields edit their own text; Go-owned fields synchronize focus/selection
and dispatch the existing input path. Do not enable undo/redo for vtui.Edit,
which has no such capability. Command-line text and clipboard remain Go-owned.

Instantiate menu rows only on demand, and defer retirement after close because
some Qt styles close the menu before emitting MenuItem.triggered. Destroying
rows synchronously on visible=false silently swallowed Copy/Delete actions.
Debug diagnostics use [FIX:text-context-menu] and contain only the control ID.

## Prevention

The pre-fix check found no command-line menu. Regression coverage includes
action icons and colour, raw Go copy dispatch, one in-window path popup, native
selection deletion/undo and read-only copy. Wait for menu layout before synthetic
clicks on lazy rows. Include the new component in the shared production/test QML
manifest. Further useful coverage is keyboard context-menu activation and IME
selection in password and multiline dialog fields.

The live settings audit found the original zoin worktree/profile and saved
gui_theme.ini, settings.ini and highlight.ini intact. The live GUI section
already selected Zoin; no guessed backup was copied over user preferences.

## Tags

`#qml` `#context-menu` `#clipboard` `#focus` `#lazy-loading` `#qt69`
