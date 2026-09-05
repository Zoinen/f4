# Issue #793 solution review

## Reproduction

On Windows 10 x64, using the native Win32 GUI build from `upstream/main` at
`06281917f6b6bdaad9864802aae507b52d6c0639`:

1. Open the main menu with F9.
2. Open `Commands`.
3. Select the final `Command palette` item and press Enter.

The submenu closes, but the command palette is not shown. Ctrl+Shift+P opens
the same palette successfully, so the action registration and dialog itself
work.

## Three-pass review

### Pass 1: change the action/menu callback

Change `BuildMenuBarItems` so the command-palette item closes the menu or calls
a special menu-only handler. This would duplicate menu lifecycle knowledge in
f4 and make the generic action path behave differently from the hotkey path.
Reject: the existing action registry already provides one shared execution
path, and the bug is in the precondition order inside `ShowCommandPalette`.

### Pass 2: keep the deferred retry, but make it reachable

`ShowCommandPalette` already defers itself while `GetActiveMenuBar().Active` is
true because vtui invokes `OnClick` before the submenu is closed. However, the
unknown-modal guard currently runs first. A `VMenu` is modal, so it consumes the
request before the active-menu branch can post the retry. Move the active-menu
check before the modal whitelist. After the current input dispatch removes the
submenu, the posted retry sees the underlying workspace frame and opens the
palette normally.

This preserves the modal whitelist for real modal dialogs and keeps the hotkey
path unchanged.

### Pass 3: consider a vtui lifecycle change

Change vtui so a menu item's callback runs after the menu is removed, or add a
new explicit close-and-run API. That would affect every menu action and risks
changing callback ordering for existing commands, including actions that rely
on the current owner/menu state.

Reject for this ticket: no vtui change is needed once f4 reaches its existing
deferred path. The f4-only change is smaller, testable, and avoids a dependency
update.

## Selected solution and safety checks

Select Pass 2. The regression test will invoke `RunAction` with an active
submenu, close the submenu as the input dispatcher does, drain the posted UI
task, and verify that a `commandPaletteDialog` is pushed. Existing direct
palette tests continue to cover the hotkey path. The native validation will
repeat both paths and confirm that the menu path displays the palette without
breaking ordinary menu closing.
