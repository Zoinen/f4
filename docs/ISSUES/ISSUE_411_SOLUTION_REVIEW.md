# Issue #411 solution review

## Problem model

The hotkey configuration dialog currently edits `GlobalHotkeysMgr` directly.
Both unbinding and assigning a key persist immediately, so an accidental click
changes runtime dispatch and writes `hotkeys.ini` before the user can review or
cancel the rest of the dialog session.

## Candidate solutions

1. Add only a confirmation prompt to unbinding. This reduces accidental
   deletion, but assignment still persists immediately and there is no way to
   cancel a sequence of accepted edits.
2. Give the dialog an isolated `HotkeyManager` draft, confirm destructive
   unbinds, and commit the draft only from an explicit Save button. This keeps
   runtime dispatch and disk state unchanged until the user commits, while
   preserving the existing manager and INI format. **Selected.**
3. Snapshot and restore the global manager when the dialog closes. This would
   require temporarily exposing uncommitted bindings to the whole application,
   and makes nested assignment dialogs and unexpected close paths harder to
   reason about than an isolated draft.

## Three-pass review

### Pass 1: correctness

The draft deep-copies all nested binding maps, so Bind does not alias the live
manager. The main dialog renders and refreshes from the draft. Save copies the
draft into the live manager and calls the existing Save method; Cancel simply
closes the dialog. The assignment capture frame also receives the draft rather
than reaching for the global manager.

### Pass 2: adversarial lifecycle

Cancel after one or more accepted assignments or unbinds leaves both the live
manager and `hotkeys.ini` unchanged. Cancel in the unbind confirmation leaves
the draft unchanged. Esc in the assignment capture frame leaves the draft
unchanged. An accepted unbind is still reviewable in the table but is not
runtime-effective until Save.

### Pass 3: regression and scope

The manager transaction test covers isolation, no early persistence, and the
explicit commit path. The UI test checks that Save and Cancel are present in
the native dialog. Existing hotkey, UI, full-suite, race, lint, native ARM64,
and PTY smoke checks remain the final validation gate. This change is limited
to configurable hotkey settings and does not alter the hotkey file format.
