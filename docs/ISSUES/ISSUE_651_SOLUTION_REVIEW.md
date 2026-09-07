# Issue #651 solution review

This review is for the actionable checklist items on current `upstream/main`.
The disputed Panel settings layout is intentionally excluded.

## Candidate solutions

1. **Long menus:** clamp a `vtui.VMenu` created by `MenuBar` to the available
   screen height and rely on its existing `ScrollView`/scrollbar behavior.
2. **Long menus:** replace long menus with a paged menu in f4. Rejected because
   every vtui consumer would retain the same overflow bug.
3. **Long menus:** shorten or hide menu entries. Rejected because it removes
   commands instead of making them reachable.

1. **Hotkey display:** make the single shortcut chosen for a menu item
   deterministic, while keeping all active bindings available to the command
   palette. Add real Far-style archive shortcuts for the two archive actions.
2. **Hotkey display:** render every alternate shortcut in every menu. Rejected
   because it makes narrow menus wider and changes the established single-slot
   menu layout.
3. **Hotkey display:** remove alternate bindings. Rejected because it breaks
   existing navigation behavior.

1. **Autosave:** retain the legacy master switch for compatibility and add
   independent switches for dialog settings, panel/workspace state, current
   panel location/cursor, and GUI geometry. Automatic writes consult the
   relevant switch; explicit Shift+F9 saves remain explicit and unaffected.
2. **Autosave:** add only one new master switch. Rejected because it cannot
   express the four groups requested by the issue.
3. **Autosave:** split settings into unrelated files. Rejected because it
   changes the existing session/config format and complicates compatibility.

## Three review passes

### Pass 1 — scope and compatibility

Current main already has windowed GUI dimensions, Left workspace shortcuts,
the Command palette action, and a selective manual save dialog. The remaining
code defects are the vtui menu geometry, map-order shortcut selection, missing
archive command metadata/handlers, and the single autosave switch.

### Pass 2 — failure modes

Menu clamping must preserve the inclusive terminal coordinate convention and
leave at least one selectable row; vtui already owns `ViewHeight`, `TopPos`,
and scrollbar drawing. Shortcut selection must not remove any binding. New
autosave keys must default from the old `AutoSaveSettings` key so existing
profiles retain behavior. Manual saves must not be blocked by autosave flags.

### Pass 3 — regression risk

The changes will be covered by focused tests for menu viewport bounds,
deterministic alternate-key selection, archive registrations, autosave
round-tripping/gating, and session-field merging. Native Linux ANSI execution
will verify the generated menu remains navigable at a small terminal height;
the GUI window-size behavior will be reported as already present on current
main rather than changed speculatively.

## Decision

Implement the common vtui fix plus the f4-side deterministic shortcut,
archive, and autosave changes. Leave the disputed settings layout and already
working GUI/Command-palette items unchanged.

## Follow-up review after the reporter's retest

The retest used a commit that already contained PR #740, so the remaining
failures were checked against current `upstream/main` rather than assumed to be
stale.

1. **Generic plugin menus:** cap `PanelsFrame.Menu` by both its normal compact
   limit and the actual screen height, then let `vtui.VMenu` scroll the rows.
2. **Generic plugin menus:** keep the fixed 15-row height. Rejected because a
   10-row terminal still receives a 15-row menu and loses its bottom rows.
3. **Generic plugin menus:** paginate in every caller. Rejected because the
   reusable `VMenu` already supplies the required viewport and scrollbar.

1. **Archive shortcuts:** match far2l's Files menu with Shift+F1 for Add and
   Shift+F2 for Extract; keep the legacy two-command menu on Shift+F3.
2. **Archive shortcuts:** keep Alt+F5/Alt+F6 and only change the displayed
   labels. Rejected because the actual keys would still differ from the
   expected Files-menu behavior.
3. **Archive shortcuts:** remove the legacy command menu. Rejected because it
   would remove an existing entry point without helping the requested actions.

The follow-up is covered by a small-screen `PanelsFrame.Menu` regression and
updated archive registration/hotkey tests. The remaining startup-mode,
deterministic drive-menu display, and Command-palette paths are already
represented by current code and focused tests; the native Windows ANSI run
also exercised the current menu and exit paths before reporting completion.

## Current Linux follow-up: GUI window mode and Left workspace shortcuts

The reporter's latest retest leaves two actionable observations. The current
f4 `Left` menu already contains `New workspace` and `Close workspace` with
`Ctrl+N` and `Ctrl+W`; the missing regression was coverage of those two menu
fields, not a missing binding in current `upstream/main`. The GUI-mode failure
is reproducible through the shared vtui X11 backend: it unconditionally writes
`_NET_WM_STATE_MAXIMIZED_VERT/HORZ` before mapping every new window, so the
requested `GuiCols`/`GuiRows` are immediately replaced by the monitor-sized
geometry.

### Three solution passes

#### Pass 1 — scope and compatibility

1. Add an f4-specific persisted `GuiWindowMode` and thread it through every
   GUI backend. Rejected: it duplicates native-window policy in f4 and would
   require new API and persistence semantics for each backend.
2. Stop vtui's unconditional initial X11 maximization and let the existing
   requested geometry, including f4's saved `GuiCols`/`GuiRows`, determine the
   first window. Selected: it fixes the common backend without changing f4's
   configuration format.
3. Infer maximized mode from the saved size or the current monitor size.
   Rejected: a large ordinary window is not equivalent to a maximized window,
   and monitor work areas differ across desktops and scaling modes.

#### Pass 2 — failure modes

With the initial EWMH state request removed, X11 maps the dimensions supplied by
`RunInGUIWindow`. The existing `ResizeWindow` path still sends EWMH add/remove
messages when a native resize crosses the initial geometry, so native maximize
and restore remain available after startup. Wayland and non-X11 backends are
untouched. The Left-menu entries remain command-routed and keep their current
physical framework bindings.

#### Pass 3 — regression risk and validation

The f4 test will assert both workspace menu labels expose their `Ctrl+N` and
`Ctrl+W` hints. The vtui change will be validated by the full vtui test suite,
an ARM64 f4 build linked against the patched vtui worktree, and a live X11
window run on Linux aarch64/KDE Wayland through XWayland: the window must start
at the requested geometry and not advertise the maximized EWMH state. The
Wayland backend itself currently panics before f4 setup on this machine, so it
will not be used as evidence for this X11-specific fix.

### Decision

Submit the generic initial-window fix to vtui and keep the disputed Panel
settings layout out of scope. Do not change f4's already-correct Left-menu
bindings or invent a second GUI-mode setting until a backend-neutral native
window-state API exists.

## Follow-up: the refreshed Left menu erased native workspace shortcuts

The reporter's next retest identified a real regression in the previous
workspace-menu coverage. `leftMenu` initially supplied `Ctrl+N` and `Ctrl+W`,
but `GetMenuBar` rebuilt the menu and then replaced those values with the
empty result of `HotkeyManager.GetKeyForAction`. The two shortcuts are
framework-owned `NativeKeys`, not configurable defaults, so the displayed
keys disappeared even though the actions still worked.

### Three solution passes

1. Keep the literal `Ctrl+N`/`Ctrl+W` values in the side menu. Rejected: the
   dynamic refresh still erases them and it would ignore user overrides,
   terminal ownership, queue vetoes, and future native actions.
2. Install native workspace keys as ordinary hotkey defaults. Rejected: the
   focused frame must receive these keys before the framework fallback, and
   turning them into configurable defaults changes dispatch precedence.
3. Resolve menu shortcuts through the action registry, merging the active
   configurable binding with `NativeShortcutsForAction`. Selected: it keeps
   dispatch ownership unchanged while making every menu refresh truthful and
   respecting explicit unbinds or context-specific ownership.

The follow-up regression covers the actual `GetMenuBar` refresh path for both
Left-menu workspace entries and verifies that an explicit `Ctrl+N=None`
override is reflected. The disputed Panel settings layout remains out of
scope.
