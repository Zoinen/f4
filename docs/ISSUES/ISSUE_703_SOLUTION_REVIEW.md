# Issue #703 solution review

## Problem model

`App.CopyWindowTitle` was first changed to use the top frame's visible title,
but the issue owner clarified that the useful value is the frame's stable help
identity. `currentWindowTitle()` deliberately reports the host terminal/GUI
title generated from `ConsoleTitleTemplate` and uses the active workspace
title, so it must remain separate from this debugging action.

The fix must distinguish the host title used by `UpdateWindowTitle` and
`Far.Title` from the active UI frame identity used by the debugging clipboard
action.

## Candidate solutions

1. Change `currentWindowTitle()` to return the top frame identity. Rejected: this
   would change the visible terminal/GUI title and the documented `Far.Title`
   API, and would make transient menus/dialogs leak into host window titles.
2. Add a separate `currentFrameIdentity()` helper using
   `FrameManager.GetTopFrame().GetHelp()`, with the visible frame title as a
   fallback for legacy frames and the existing host-title function as a
   startup/shutdown fallback. Selected: it follows the requested help
   identity and keeps the existing host-title contract unchanged.
3. Add a new vtui API for this single action. Rejected: `Frame.GetHelp()` is
   already the stable identity used by the help system, so a cross-repository
   dependency change would add scope without improving correctness.

## Three-pass review

### Pass 1: correctness

The action now reads the exact frame that receives input. It returns the frame's
help identity when one is declared and otherwise trims the frame title, while
the host title remains independent.

### Pass 2: lifecycle and edge cases

The helper tolerates an uninitialized frame manager and an empty frame stack.
The action falls back to the host title during startup/shutdown, preserving a
useful result without changing normal behavior. The existing host title path,
template expansion, and Lua `Far.Title` behavior remain untouched.

### Pass 3: regression and scope

The action test covers a workspace frame and a modal dialog whose help identity
differs from its visible title, including the asynchronous clipboard write.
Existing title-rendering tests continue to assert that transient menus do not
alter the host terminal title. The change is limited to the debugging action,
its documentation, and regression coverage.
