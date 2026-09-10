# Phase 9: The Command Line and the Panels

Plan: [index.md](index.md)
Tasks: 35 then 34
Depends on: Phase 8

## Objective

The two largest interactive subsystems leave `cmd/f4`: `internal/panel` (30
outbound edges, and the home of the two biggest types in the codebase) and
`internal/cmdline` (41 — the highest count in the tree, which is why it goes
last among the subsystems). After this phase only the composition root remains
inside the flat package.

This is also the phase that performs the type surgery the earlier waves deferred:
`semantic.go`'s final split, `panel_plugins.go`, `dragdrop.go`, `translator.go`,
and filling `internal/paneltest`.

## The Extraction Gate

By this point `PanelsFrame`, `FileSystemPanel` and `pluginPanelInstance` are
Task 34's own types and `CommandLine` is Task 35's. The only remaining "later"
direction is panel → cmdline: a file moving into `internal/panel` must not
reference `CommandLine`.

```
grep -c '\bCommandLine\b' cmd/f4/<file>.go     # must be 0 for a Task 34 file
```

Move by `//go:build` line, never by filename. Take the `_test.go` files Task 43
assigns to this wave — not the same-named neighbours.
Rename to `<topic>.go` / `<topic>_<aspect>.go`. Export only what has an external
caller. One line to `architecture_test.go`'s layer map, one to the palette
auditor's file→target-package map. Close every `docs/` reference in the same commit.

## Current-Code Evidence

Per-type gate scores. Columns: `PanelsFrame` / `FileSystemPanel` /
`pluginPanelInstance` / `CommandLine`.

Five of the fourteen rows below were wrong from the start — the numbers match
the base revision as well as the tree, so this is not drift. `panels_frame.go`'s
197 is no reading of any revision (115 and 117 there, 117 and 119 here) and
looks like transposed digits. The other four put a `FileSystemPanel` count in
the `PanelsFrame` column and left a dash where a number belongs, which made
`reconnect.go` read as frame code when it names the frame nowhere. The
destinations do not change — both types go to `internal/panel` — but measure
each file rather than trusting the columns.


| File | PF | FSP | PPI | CL | Reading |
|---|---|---|---|---|---|
| `panels_frame.go` | 117 | 81 | — | 0 | the type's home; 5771 lines |
| `file_panel.go` | — | 107 | — | 0 | the type's home |
| `panel_plugins.go` | 5 | 4 | 17 | 0 | plus **one** `coreAPI` method, already cut out in Task 26 |
| `dragdrop.go` | 6 | 8 | — | 0 | five `*PanelsFrame` methods + one `*FileSystemPanel` — moves **whole** |
| `translator.go` | 3 | 2 | — | 0 | one method each — moves **whole** |
| `console_passthrough.go` | 16 | 0 | — | 0 | panel code; its three "CommandLine" hits are two comments and a field access |
| `temp_panel.go` | 5 | 4 | — | 0 | move whole |
| `info_panel.go` | 2 | 5 | — | 0 | move whole; localizes the GPU `ModelKey` from Task 7 |
| `quick_view_panel.go` | 2 | 3 | — | 0 | move whole; uses `numeric.NonNegativeUint64` |
| `path_hints.go` | 2 | 1 | — | 0 | move whole |
| `reconnect.go` | 0 | 2 | — | 0 | move whole → `list_reconnect.go`; it is about the file panel, not the frame |
| `panel_actions.go` | 1 | 1 | — | 0 | move whole |
| `viewer_editor_history.go` | 3 | 1 | — | 0 | **not** `internal/history` — comes here |
| `text_editor_bridge.go`, `visren_editor_bridge.go` | ≥1 | — | — | 0 | the `vfs.TextEditorHost` assertion binds them to `PanelsFrame` |
| `command_line.go` | 0 | — | — | 16 | the type's home |
| `command_prefix_registry.go` | 1 | — | — | — | resolve |
| `cmd_session.go`, `simple_exec.go` | 2 each | — | — | — | resolve |
| `commands.go` | 0 | — | — | 0 | move whole |
| `apply_command.go` | 5 | — | — | — | resolve |
| `semantic.go` | 5 | 4 | — | 2 | 808 lines, methods of six types + 15 free functions |

`semantic.go`'s receiver census, measured: `*EditorView` ×5, `*PanelsFrame` ×4,
`*ViewerView` ×3, `*TerminalView` ×1, `*FileSystemPanel` ×1, `*CommandLine` ×1,
plus 15 free functions. Tasks 29, 30 and 33 already took the viewer, terminal and
editor slices.

## Files to Change

| Path | Action | Required change |
|---|---|---|
| `internal/panel/` | create | `PanelsFrame`, `FileSystemPanel`, plugin panels, quick view, info |
| `internal/paneltest/` | fill | `SetupMockPanelsFrame`, `MockPty` |
| `internal/cmdline/` | create | Command line, prefixes, apply-command, sessions |
| `cmd/f4/semantic.go` | delete | Fully distributed across five packages |
| `cmd/f4/architecture_test.go` | modify | Two layer-map entries |
| tests inside `panel`/`cmdline`/`term` using the mock frame | modify | Become `package X_test` |

---

## Task 34: Extract `internal/panel`


### Two files arriving from Task 32

`fuse_mount_action.go` (227 lines, two `init()`) and `fuse_mount_list.go` (285
lines, one `init()`) were rostered for `internal/fileops` and measured into this
wave instead: both read `fsp.vfs` and `pf.getActivePanel`, private members of
the types this package owns. 512 lines between them.

They carry the three action registrations this restructuring has left in
`cmd/f4`, which makes **this** the wave that can reorder the menu. Run
`go test ./cmd/f4 -run '^TestActionOrderIsStable'` after the move, and if it
fails read it as a real reordering rather than as a stale golden.

### Intent

Thirty outbound edges and the two largest types in the codebase: `PanelsFrame`
(160 methods across 16 non-test files) and `FileSystemPanel` (106 across 6),
counted receiver-anchored with `grep -hE '^func \([a-z]+ \*T\)'` — the looser
`grep 'func (.*T)'` returns 161/17 and 109/7 by catching methods of one type that
merely take the other as a parameter (`panels_frame.go:71,72,4038,5533`). Go
requires a type's methods in the type's package, so the file count is a
consequence of the types, not a choice — and it is exactly why the file-naming
convention matters here more than anywhere else.

### Implementation Steps

1. Move the type homes: `panels_frame.go` (5728 lines) → `frame.go`,
   `file_panel.go` → `list.go`.
2. Move the aspect files whole, renaming to the topic convention. **The topic is
   the subject inside the package, never the package name** — `frame_dragdrop.go`,
   not `panel_frame_dragdrop.go`:
   - `dragdrop.go` → `frame_dragdrop.go`. Five `*PanelsFrame` methods and one
     `*FileSystemPanel` method; both types are here, so it moves whole. **Do not
     split it** — the call graph associates it with the GUI, which is why Task 27
     explicitly declined it.
   - `translator.go` → `frame_translator.go`. One method of each type; whole.
   - `reconnect.go` → `list_reconnect.go`.
   - `panel_actions.go` → `actions.go`, `temp_panel.go` → `temp.go`,
     `info_panel.go` → `info.go`, `quick_view_panel.go` → `quickview.go`,
     `quick_view_provider_api.go` → `quickview_api.go`,
     `path_hints.go` → `hints.go`, `sort_groups.go` → `sort.go`,
     `folder_pins.go` → `pins.go`, `bookmarks.go` → `bookmarks.go`,
     `file_associations*.go` → `associations*.go`,
     `user_menu*.go` → `usermenu*.go`,
     `drive_bookmarks*.go` → `drives_bookmarks*.go`,
     `drive_menu_options*.go` → `drives_menu*.go` (by build tag),
     `workspace_session.go` → `workspace.go`,
     `viewer_editor_history.go` → `history_bridge.go`,
     `text_editor_bridge.go` → `bridge_texteditor.go`,
     `visren_editor_bridge.go` → `bridge_visren.go`.
3. `panel_plugins.go` → `plugins.go`. It scores `pluginPanelInstance` ×17,
   `PanelsFrame` ×5, `FileSystemPanel` ×4 — all three types are here — and one
   `coreAPI` method, which **Task 26 already cut out** into `internal/plughost` as
   a free function. Verify the cut landed before moving; if it did not, do it now
   rather than importing plughost from panel.
4. `console_passthrough.go` scores `PanelsFrame` ×16 and `CommandLine` ×3. The
   three command-line references are the only panel → cmdline edge in this wave.
   Resolve them: either the passthrough takes the command line as a parameter, or
   the three call sites stay in `cmd/f4` and move with the composition root.
   Choosing the parameter keeps `internal/panel` free of `internal/cmdline`, which
   the layer table wants — both are layer 3 and a mutual import is a cycle waiting
   for the next change.
5. **Finish `semantic.go`.** Take the four `*PanelsFrame` methods as
   `frame_semantic.go` and the one `*FileSystemPanel` method as
   `list_semantic.go`. Distribute the 15 free functions to the package of their
   caller — `semanticInt` uses `numeric.BoundedUint64ToInt` and is called only from
   within `semantic.go`, so it follows whichever methods use it, duplicated only if
   two packages need it. After this task and Task 35, `cmd/f4/semantic.go` is
   empty: `git rm` it.
6. **Fill `internal/paneltest`** (created empty in Task 9):
   - `SetupMockPanelsFrame(t *testing.T) *panel.PanelsFrame` — was
     `setupMockPanelsFrame` (`panels_frame_test.go:794`). It calls
     `term.NewTerminalView`, `cmdline.NewCommandLine`, `panel.NewFileSystemPanel`,
     `vfs.NewOSVFS` and `(*PanelsFrame).initPTY`, so the package imports
     `internal/panel`, `internal/cmdline` and `internal/terminal`. That is why it is a
     separate package from `internal/testutil`.
   - `MockPty` — the fixture from `ansi_parser_test.go:22`, moved here per Task
     30's decision, with a local copy left in `internal/terminal` for its own tests.
   - `WaitForDirectoryLoads(t *testing.T)` — the drain that waits on the
     `directoryLoadWorkers` global, which now lives in `internal/panel`. Callers
     pass it to `testutil.SwapFrameManager`.
   - `WaitForLoad(t *testing.T, fp *panel.FileSystemPanel)` — was `waitForLoad`
     (`file_panel_test.go`), used by 19 test files in eight packages. It reads
     `fp.isLoading`, so `FileSystemPanel` gains an exported `IsLoading()`
     accessor in this wave.
7. **Convert the in-package tests that use the mock frame.** Any test file inside
   `internal/panel`, `internal/cmdline` or `internal/terminal` that calls
   `paneltest.SetupMockPanelsFrame` must become an external test package —
   `package panel_test` in the same directory — because `internal/paneltest`
   imports those packages. Go allows an external test package to import a package
   that imports the package under test; it does not allow the in-package form.
   Of the 28 `setupMockPanelsFrame` users, the ones landing in these three
   packages are: `panels_frame_test.go`, `panel_plugins_test.go`,
   `panel_menu_test.go`, `dragdrop_test.go`, `quick_view_panel_test.go`,
   `info_panel_test.go`, `path_hints_test.go`, `console_passthrough_test.go`,
   `user_menu_ui_test.go`, `shell_integration_test.go`, `simple_exec_test.go`,
   `shell_session_test.go`, `cmd_session_test.go`, `apply_command_test.go`,
   `command_prefix_registry_test.go`, `history_hint_test.go`. Score each against
   its final package and convert only those that need it — a test that does not
   touch unexported identifiers converts for free; one that does needs its
   assertions rewritten against the exported surface, and that is a rewrite, so it
   gets its own commit if it is more than mechanical.
8. Add `"internal/panel": 3` to the auditor's layer map and `panel` to the palette
   auditor's file→target-package map — this is the wave that resolves the audit key
   `panel.(*FileSystemPanel).ProcessKey` from Task 2.

### Required Interfaces and Contracts

- `internal/panel` may import `internal/config`, `internal/i18n`,
  `internal/theme`, `internal/keymap`, `internal/sysinfo` (the drive registry),
  `internal/numeric`, `internal/toast`, `internal/history`, `internal/action`,
  `internal/dialog`, `internal/fileops`, `internal/terminal`, `internal/media`,
  `internal/viewer`, `internal/editor`, `internal/plughost`, `vfs`. **Not**
  `internal/cmdline` (step 4) and not `internal/app`.
- Lower layers talk to the panel through interfaces they define themselves. The
  architecture's worked example is `PluginColumns` declared *in* `internal/panel`:
  the panel says what it needs from a plugin host and does not import plughost for
  it.
- `PanelsFrame`, `FileSystemPanel`, `PanelViewSettings` and every other
  Far-derived name is unchanged. Moving a file into a package does not license
  renaming its types.
- `var _ vfs.TextEditorHost = (*PanelsFrame)(nil)` (`text_editor_bridge.go:16`)
  must still compile — it is the assertion that decided the file's home.
- `internal/paneltest` and `internal/testutil` are test scaffolding that ships in
  the module. Their package docs say so; no production file imports them.

### Error Handling and Logging

- Panel operations report through dialogs and the toast channel; error strings are
  user-visible and `ST1005` is disabled for them. Reword nothing.
- `directoryLoadWorkers` is a `sync.WaitGroup` read by tests through the drain.
  Keep its semantics exactly: the workers read `vtui.FrameManager` and
  `config.App` while running, which is why a test replacing either must join them
  first.
- No logging is added.

### Tests

The 48 files Task 43's roster lists for `internal/panel` move — the same-named
neighbours of every source above plus the scenario tests the roster names, among
them `shell_session_test.go`, `shell_integration_test.go`,
`issue863_terminal_test.go`, `uri_navigation_test.go`, `navigation_mode_test.go`,
`panels_frame_pty_test.go`, `managed_execution_test.go`,
`file_associations_dispatch_test.go`, `workspace_routing_test.go`,
`workspace_session_test.go` and what remains of
`frame_manager_test_helpers_test.go`. The conversions in step 7 are the
substantive test work in this plan; budget for them.

```
go test ./internal/panel/... ./internal/paneltest/...
go test -race -shuffle=on -timeout 10m ./internal/panel/...
```

The race run matters most here: the panel owns the directory-load workers that
`swapFrameManager`'s drains exist for.

### Acceptance Criteria

- `grep -rln 'func (.*\(PanelsFrame\|FileSystemPanel\))' cmd/f4/` returns nothing.
- `go list -f '{{join .Imports "\n"}}' ./internal/panel | grep -E 'internal/(cmdline|app)'`
  returns nothing.
- No file in `internal/panel` is named `panel_*.go` — the topic is the subject
  inside the package.
- `internal/paneltest` exports `SetupMockPanelsFrame`, `MockPty`,
  `WaitForDirectoryLoads` and `WaitForLoad`.
- The palette auditor's 42 keys pass with `panel.` qualifiers.


### What the wave actually found

**`hotkeys.go` did move, and phase 5's reason for leaving it is why it could.**
That note said the `conditionRegistry` closures ask the panels frame what is on
screen, so the file belongs with the application. Measured, that names the
*closures*, not the manager: the closures read `pf.cmdLine`, `pf.showPanels`,
`pf.termView`, `pf.shellMode` and `pf.isPtyBusy`, all private members of the
panels frame, and everything else in the file reads only the bindings. The
registry was already fillable from outside — `RegisterCondition` has always been
exported — so the closures moved to `internal/panel` and register themselves
from its `init`, and the manager went to `internal/keymap` beside the Far
key-name codec that came out of it in phase 5. `nativeShortcutOwnedByCurrentContext`
became a seam for the same reason.

That decided the rest of the cluster. `configuredHotkeyAction` reads
`hasExplicitBinding`, a private method, so it went to `keymap` too, and
`macro_dispatch.go` stayed in `cmd/f4` around it: `macroFilter` reaches the
command palette, which is the application's dialog.

**`plugin_hotkeys.go` went to `internal/panel`, not `internal/app`.** Phase 6
sent it up because every binding function takes a `*HotkeyManager` and
`pluginMenuKeyLabels` takes a `*PanelsFrame`. Both are still true; what changed
is that `HotkeyManager` is now at layer 0, so the file follows the frame.

**Two registries went to `internal/plughost`** — the plugin menu items and the
plugin global hotkeys — the split phase 6 made for `plugin_contributions.go`,
for the same reason: `internal/panel` reads them, so they sit below it. They
share one mutex, which is why they travel together, and
`SnapshotPluginRegistries` is what a test uses now that the mutex is not
reachable from `cmd/f4`.

**The panel's API is wide on purpose, and the width was measured.** 111 members
of `PanelsFrame`, `FileSystemPanel` and their neighbours are exported, because
the action table drives the panels and the action table is the application. The
five names that looked accidental — `Pf`, `Free`, `SourcePath`, `Filesystem`,
`Chord` — were un-exported to check, and every one of them broke a caller in
`cmd/f4`. Nothing here is exported that nothing outside uses.

**Tests split by what they need, not by what they name.** A test that inspects
the frame's state is a panel test and moved with the frame; a test that presses
a key and expects an action to run needs the action table, which the panel's
`RunAction` seam cannot supply — the seam is inert in the panel's own `TestMain`
and cannot be otherwise, because the table is layer 4. That is 36 tests, now in
`cmd/f4/panels_frame_app_test.go`, and it is the reason
`internal/panel/press_key_test.go` exists: a key handed straight to a frame
tests a path the user never takes, so the helper goes through `KeyFilter`.

**The build-tag export trap fired again.** `shellPromptReady` is read only from
a Windows-only test, `conInMouseInput` and its two neighbours only from
Windows-only files, and `getActivePTY`/`writePTY` only from
`console_ctrl_handler_windows.go`. None of them are visible to a tool running on
darwin. The ten-target cross-build and `GOOS` vet on four systems are what
found them, and they are the only things that can.

**One string-literal rewrite got through and was caught by the diff.** Stripping
the `panel.` qualifier from a file moved into the package ate it inside five
string literals too: `"panel.activate"`, `"panel.cursor"`, `"panel.open"`,
`"panel.refresh"` and `"panel.toggleSelection"`, the semantic action names the
UI protocol sends. The code compiled and every test passed; only the
before/after literal multiset saw it.

### Verification

- `go test -race -shuffle=on -timeout 10m ./internal/panel/...`
- Expected result: `ok`.
- `go test ./cmd/f4 -run '^TestCommandPalette|^TestArchitecture' -v`
- Expected result: both pass; 42 keys.
- `go test -timeout 25m ./...`
- Expected result: identical to the Task 1 baseline.

---

## Task 35: Extract `internal/cmdline`

### Intent

Forty-one outbound edges, the highest in the tree — which is precisely why it is
last among the subsystems. By this point every one of those targets has its own
package, so the wave is a move rather than an excavation.

### Implementation Steps

1. Move the type home: `command_line.go` (`CommandLine` ×16, 12 methods across two
   files) → `line.go`.
2. Move the zero-score files whole — thirteen of them, none naming a panel
   type. (`commands.go` is no longer among them: it became `internal/appcmd` in
   Task 33, because the editor needed its constants first.)
   `command_quotes.go` → `quotes.go`, `command_quoting.go` → `quoting.go`,
   `remote_command.go` → `remote.go`,
   `resolve_command_other.go` / `resolve_command_windows.go` → `resolve_*.go`
   (by build tag).
3. **`command_prefix_registry.go`, `cmd_session.go`, `simple_exec.go`,
   `apply_command.go` and `remote_command.go` are not here.** An earlier version
   of this step asked for their panel references to be resolved by parameter;
   they cannot be. All five read private members of `PanelsFrame` and
   `FileSystemPanel` — `cmd_session.go` eight of them, `apply_command.go` nine,
   `simple_exec.go` eight plus two methods declared on `*PanelsFrame` — and a
   parameter does not open a private member. They belong to `internal/panel` by
   Go's rule; Task 34 takes them. See `index.md`.

   What that leaves: `internal/cmdline` names no panel type at all, and
   `internal/panel` imports it in one direction.
4. Move the apply-command family: `apply_command.go`, `apply_command_batch.go`,
   `apply_command_output.go`, `apply_command_resources.go`,
   `apply_command_subst.go`, `apply_command_transcript.go`,
   `apply_shortname_other.go` (`!windows`), `apply_shortname_windows.go`
   (`windows`), `apply_shutdown.go` → `apply.go`, `apply_batch.go`,
   `apply_output.go`, `apply_resources.go`, `apply_subst.go`,
   `apply_transcript.go`, `apply_shortname_other.go`,
   `apply_shortname_windows.go`, `apply_shutdown.go`.
5. `cmd_session.go` and `simple_exec.go` go to `internal/panel` (step 3);
   `simple_exec_other.go` and `simple_exec_windows.go` do not exist. There is no
   `history_hint*.go` source either: `history_hint_test.go` drives
   `actionCommandHistory` and is an `internal/app` test.
6. **`command_runner*.go`, `shell_mode.go` and `wine_probe*.go` are explicitly
   NOT here.** They went to `internal/terminal` in Task 30 because their callers are
   `pty_*`. Leaving them in cmdline inverts the layers.
7. Take `semantic.go`'s single `*CommandLine` method as `line_semantic.go`, then
   `git rm cmd/f4/semantic.go` — it is now empty.
8. Add `"internal/cmdline": 3` to the auditor's layer map and `cmdline` to the
   palette auditor's map.

### Required Interfaces and Contracts

- `internal/cmdline` may import every layer 0-2 package plus `internal/panel`,
  `internal/dialog`, `internal/terminal`, `internal/editor`, `internal/viewer`,
  `internal/macro`, `vfs`. Not `internal/app`.
- The apply-command syntax is a user-facing contract: `CompiledApplyCommand`,
  `ApplyCommandSyntaxError`, `ApplyCommandExpansion` and
  `ApplyCommandListFileSpec` keep their names and their error messages, which
  appear in the transcript the user reads.
- Command prefixes are user-visible strings registered through
  `command_prefix_registry.go`; the registration order determines match priority
  and must not change.
- `AppConfig.ApplyCommandParallelism` — now `config.App.ApplyCommandParallelism` —
  keeps its documented meaning: `0` = unlimited, absent config defaults to
  `runtime.NumCPU()`.

### Error Handling and Logging

- Apply-command errors are shown in the output dialog and the transcript. Keep
  every message verbatim.
- `remote_command.go` wraps transport errors with context and compares with
  `errors.Is`. Preserve the sentinels.
- No logging is added.

### Tests

The 11 files Task 43's roster lists for `internal/cmdline` move:
`apply_command_batch_test.go`, `apply_command_resources_test.go`,
`apply_command_subst_test.go`, `apply_command_test.go`, `cmd_session_test.go`,
`command_line_test.go`, `command_prefix_registry_test.go`,
`command_quotes_test.go`, `command_quoting_test.go`,
`resolve_command_windows_test.go`, `simple_exec_test.go`. **Not**
`shell_session_test.go` (a panel test, Task 34) and **not**
`history_hint_test.go` (it references no command-line symbol; it drives
`actionCommandHistory` and goes to `internal/app`) — an earlier draft claimed
both here.
Several were converted to external test packages in Task 34 step 7; verify they
still compile against `internal/cmdline`'s exported surface.

```
go test ./internal/cmdline/...
go test -race -shuffle=on ./internal/cmdline/...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...
```

### Acceptance Criteria

- `ls cmd/f4/semantic.go` fails.
- `grep -rln 'func (.*CommandLine)' cmd/f4/` returns nothing.
- `ls cmd/f4/command_runner.go` fails **and** `ls internal/terminal/runner.go`
  succeeds — the reassignment from Task 30 held.
- `go list -f '{{join .Imports "\n"}}' ./internal/cmdline | grep internal/app`
  returns nothing.

### Verification

- `go test ./internal/cmdline/... ./cmd/f4/... -count=1`
- Expected result: `ok`.
- `go test ./cmd/f4 -run '^TestArchitecture' -v`
- Expected result: `ok`, acyclic, with every layer-3 package present in the map.
- `go test -timeout 25m ./...`
- Expected result: identical to the Task 1 baseline.

---

## Phase Risks and Mitigations

- **Risk:** `internal/panel` and `internal/cmdline` import each other through
  `console_passthrough.go`'s three `CommandLine` references, producing a cycle the
  compiler catches only after both moves.
  **Mitigation:** Task 34 step 4 resolves the three references *before* the panel
  moves, and the direction is fixed: cmdline may import panel, never the reverse.
- **Risk:** the external-test-package conversions in Task 34 step 7 turn into
  assertion rewrites hidden inside a move commit.
  **Mitigation:** step 7 says a non-mechanical conversion gets its own commit —
  "no rewrites inside a move commit" is a ground rule, and 28 test files is enough
  material to hide one in.
- **Risk:** `dragdrop.go` or `translator.go` is split because the graph associated
  them with the GUI and the translator respectively.
  **Mitigation:** both hold methods of both panel types, so they move whole; the
  evidence table records the counts and Task 34 step 2 says so explicitly.
- **Risk:** `semantic.go` is deleted before all six slices have landed, losing the
  15 free functions.
  **Mitigation:** the `git rm` is in Task 35 step 7, after the last slice, and the
  acceptance criterion is the file's absence rather than its emptiness.
- **Risk:** a `panel_*.go` filename survives inside `internal/panel`, reintroducing
  the stutter the naming convention exists to prevent.
  **Mitigation:** it is an explicit acceptance criterion in Task 34.

## Phase Completion Checklist

- Every Task 34-35 satisfies its acceptance criteria.
- `cmd/f4` holds only the composition root, its tests, the module-wide auditors,
  and the two `.syso` files.
- `internal/panel` does not import `internal/cmdline`.
- `cmd/f4/semantic.go` no longer exists.
- `go test -race -shuffle=on ./internal/panel/... ./internal/cmdline/...` is green.
- `go test -timeout 25m ./...` matches the Task 1 baseline.
- `index.md` task checkboxes 34-35 are ticked.
