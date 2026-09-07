# Issue #492 solution review

## Report and current behavior

Issue #492 was previously fixed for runtime dispatch, but a follow-up report
says that Right Ctrl+A still cannot be reassigned to file properties and that
the default AI shortcut remains unusable. The current code has two deliberate
key-string layers:

- `EventToFarString` normalizes Left Ctrl and Right Ctrl to the same `Ctrl...`
  spelling for macros and historical compatibility.
- `EventToHotkeyString` preserves a distinct `RCtrl...` spelling for the
  configurable action dispatcher.

The hotkey assignment dialog still uses the first function, so a captured
Right Ctrl+A is stored as `CtrlA`. That does not override the default
`RCtrlA=AI.TogglePanel` binding. This is the direct cause of the follow-up
report.

## Three-pass review

### Pass 1: change the global key normalizer

Make `EventToFarString` preserve Right Ctrl everywhere. Reject: macro storage,
bookmark shortcuts, and existing compatibility behavior intentionally treat
both Ctrl keys as the same key. A global change would alter unrelated input
surfaces and existing user configurations.

### Pass 2: add a second ad-hoc Right Ctrl conversion in the dialog

Teach `HotkeyAssignFrame.ProcessKey` to inspect modifier flags and prepend
`RCtrl` itself. Reject: this duplicates the canonical configurable-hotkey
conversion already used by `MacroManager.Filter` and `LookupHotkey`, making
future modifier fixes diverge between runtime and assignment.

### Pass 3: use the configurable-hotkey representation at capture time

Replace the assignment dialog's `EventToFarString(e)` call with
`EventToHotkeyString(e)`. This stores `RCtrlA` as `RCtrlA`, leaves ordinary
Left Ctrl capture as `CtrlA`, and makes the dialog use the same spelling the
runtime dispatcher resolves. Select this pass.

## Safety checks

Add a regression test that feeds Right Ctrl+A through the real
`HotkeyAssignFrame.ProcessKey` and verifies the draft binds `RCtrlA`, while a
Left Ctrl+A test still binds `CtrlA`. Native Win32 validation will confirm that
the assignment dialog can capture Right Ctrl+A and that the saved binding
overrides the AI default without changing ordinary Ctrl behavior.
