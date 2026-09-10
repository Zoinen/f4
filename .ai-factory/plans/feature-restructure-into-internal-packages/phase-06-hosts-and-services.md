# Phase 6: Hosts and Services

Plan: [index.md](index.md)
Tasks: 25-28
Depends on: Phase 5

## Objective

Four packages with seven outbound edges each leave `cmd/f4`: `internal/dialog`,
`internal/plughost`, `internal/gui`, `internal/macro`. After this phase the plugin
transports are behind one host, modal UI has an owner, and the GUI backends are
separated from the terminal code they are often confused with.

## The Extraction Gate

The gate is per **(file, destination)** pair, not per file. Count only references
to types whose own package is extracted **later** than this file's destination:

| Type | Lands in | Extracted at |
|---|---|---|
| `coreAPI` | `internal/plughost` | Task 26 |
| `ViewerView` | `internal/viewer` | Task 29 |
| `TerminalView` | `internal/terminal` | Task 30 |
| `EditorView` | `internal/editor` | Task 33 |
| `PanelsFrame`, `FileSystemPanel`, `pluginPanelInstance` | `internal/panel` | Task 34 |
| `CommandLine` | `internal/cmdline` | Task 35 |

Score a file with:
```
for t in PanelsFrame FileSystemPanel EditorView ViewerView TerminalView CommandLine coreAPI pluginPanelInstance; do
  printf "%-22s %s\n" "$t" "$(grep -c "\b$t\b" cmd/f4/<file>.go)"
done
```
A non-zero count for a *later* type means the file does not move whole: either the
offending function stays behind with its view, or — when its logic belongs here —
it becomes a plain function taking the type, per the project decision on
cross-package methods. Across all 345 non-test files, **235 score zero on every
type** and are a pure `git mv`; 58 score 1-3 and 52 score 4 or more.

**A zero score licenses nothing on its own.** The grep finds view types only; a
file may reference any *other* symbol still in `cmd/f4` and the gate stays silent.
The worked example is in this phase: `plugin_permissions_ui.go` scores `0` on all
eight types, and its entry point is
`func actionPluginPermissions(store *PermissionStore)` — a type declared in
`plugin_permissions.go:76`, which does not leave `cmd/f4` until Task 26. Run the
`codegraph callees` half of wave-procedure step 1 on every file, including the
zero-scoring ones; that is the check that actually binds.

Then: move by `//go:build` line and never by filename (`pty_unix.go` is
`//go:build linux`; `solaris_pty.go` is `//go:build !windows` and holds no PTY
code); take the `_test.go` files Task 43 assigns to this wave; rename to `<topic>.go` /
`<topic>_<aspect>.go` where the topic is the subject inside the package; export
only what has an external caller; add one line to `architecture_test.go`'s layer
map and, for an audited symbol, one line to
`command_palette_coverage_test.go`'s file→target-package map; close every `docs/`,
`README.md` and `AGENTS.md` reference in the same commit.

## Current-Code Evidence

| Path | Gate detail | Consequence |
|---|---|---|
| `cmd/f4/help.go:18` | `//go:embed help/en.hlf`, gate 0 | `help/` moves to `internal/dialog` |
| `cmd/f4/help_search.go`, `themed_table.go`, `dialog_button_layout.go`, `file_dialog.go`, `goto_dialog.go` | gate 0 | move whole |
| `cmd/f4/grabber.go` | 1 | one reference to resolve |
| `cmd/f4/share_dialog.go`, `find_file.go` | 3 each | resolve per reference |
| `cmd/f4/bookmarks_dialog.go` | 4 | resolve per reference |
| `cmd/f4/api.go` | `coreAPI` ×12 | the `coreAPI` type's home — this *is* plughost |
| `cmd/f4/extui_host.go`, `plughost_ffi.go`, `rpc_*.go`, `wasm_plugin.go`, `lua_plugin.go`, `plugin_permissions*.go`, `plugin_scaffold.go`, `sqlite_actions.go` | gate 0 | move whole |
| `cmd/f4/plughost.go` | 2 | resolve |
| `cmd/f4/plugin_hotkeys.go`, `plugins.go` | 1 each | resolve |
| `cmd/f4/plugin_contributions.go` | 4 | resolve |
| `cmd/f4/gui_*.go`, `window_icon_*.go`, `window_position.go`, `winepath_*.go` | gate 0, eighteen files | move whole |
| `cmd/f4/window_icon_darwin.go:18` | `//go:embed assets/icon/generated/f4.icns` | `assets/icon/` moves to `internal/gui` |
| `cmd/f4/dragdrop.go` | `PanelsFrame` ×6, `FileSystemPanel` ×8 | assigned to gui by the call graph, but the types say panel — **goes to `internal/panel` whole** |
| `cmd/f4/macro_export.go` | gate 0 | move whole |
| `cmd/f4/macro.go` | 6 | resolve |
| `cmd/f4/macro_host.go`, `macro_lua_api.go` | 2 each | resolve |
| `cmd/f4/macro_lua.go`, `macro_plugin_calls.go` | 1 each | resolve |
| `.github/workflows/build.yml:198,426,600,832` | `cp -r cmd/f4/lang cmd/f4/help build/` | the **`help` half** belongs to Task 25 |
| `.github/workflows/build.yml:1235` | `-skip '^TestAllDialogs_LayoutValidation$'` applied globally | see Task 25 step 5 |
| `.github/workflows/build.yml:1239-1242` | isolated re-run keyed on `./...` or `cmd/f4` | see Task 25 step 5 |
| `.github/workflows/build.yml:93,95-97,191,195,304,418,422` | icon generation and packaging | Task 27 |
| `tools/icons/main.go:37,38` | `iconDir`, `outDir` | Task 27 |
| `tools/icons/main.go:160` | `cmd.Dir = filepath.Join(root, "cmd", "f4")` | **stays** — `.syso` links only from the built package's dir |

## Files to Change

| Path | Action | Required change |
|---|---|---|
| `internal/dialog/` | create | Modal dialogs, palette, menus, help + `help/` |
| `internal/plughost/` | create | All four transports, `coreAPI` |
| `internal/gui/` | create | Backends, fonts, window, icon + `assets/icon/` |
| `internal/macro/` | create | Macro engine and Lua macro API |
| `.github/workflows/build.yml` | modify | `help` paths, dialog test isolation, icon paths |
| `tools/icons/main.go` | modify | Lines 1-2, 37, 38 — **not** 160 |
| `cmd/f4/architecture_test.go` | modify | Four layer-map entries |

---

## Task 25: Extract `internal/dialog`

### Intent

Modal dialogs, the command palette, generated menus and the help viewer. Seven
outbound edges, 25 inbound — a package with many callers and few dependencies is
an early candidate, which is what leaf-first ranked by *outbound* means.

This wave also collects the eight dialog helpers stranded in `actions.go`
(Phase 4's table): `confirmAndClearHistory`, `confirmAndClearRichHistory`,
`confirmAndPruneMissingFolderHistory`, `showPluginFileDialog`, `elementWidth`,
`checkboxColumnWidth`, `boolToCheckboxState`, `choiceText`.

### Implementation Steps

1. Apply the gate to every candidate. Zero-score files move whole: `help.go`,
   `help_search.go`, `themed_table.go`, `dialog_button_layout.go`,
   `file_dialog.go`, `goto_dialog.go`, `command_palette*.go` (thirteen files —
   score each; `command_palette_direct_panels.go` and
   `command_palette_direct_frames.go` are the likely non-zeros).
2. Resolve the non-zeros individually: `grabber.go` (1), `share_dialog.go` (3),
   `find_file.go` (3), `bookmarks_dialog.go` (4). For each reference, decide
   between "stays with its view" and "becomes a free function taking the type".
3. Take the settings dialogs Phase 5 deferred here: `portable.go`,
   `startup_settings.go`, `codepage_settings.go`, `hotkeys_ui.go`,
   `proxy_settings_ui.go`, `colorer_settings.go`. Each scores 0 or 1 and each is
   `Msg`-heavy UI, not configuration storage.
   Also take `compare_folders_ui.go` (gate 4, `Msg` ×28), which Task 43 assigns
   here — it is the Advanced Compare dialog, not the comparison engine that Task 32
   takes with `compare_folders.go`.
   **`plugin_permissions_ui.go` does not come here.** It scores `0`, but
   `actionPluginPermissions` takes a `*PermissionStore` declared in
   `plugin_permissions.go:76`, which leaves in Task 26. Moving it now gives an
   uncompilable commit; moving it here at all would force
   `internal/dialog` → `internal/plughost`, which this task's own contract forbids.
   It goes to Task 26 with the type it depends on.
4. `git mv cmd/f4/help internal/dialog/help`. The `//go:embed help/en.hlf`
   directive at `help.go:18` is relative to its own directory and needs no edit
   once both move together.
5. **The two CI edits that belong only to this commit:**
   - `build.yml:198`, `:426`, `:600`, `:832` — the `help` half of
     `cp -r cmd/f4/lang cmd/f4/help build/` becomes
     `internal/dialog/help`. (Phase 5 edited the `lang` half of the same lines.)
   - `build.yml:1239-1242`. `dialog_layouts_test.go` holds
     `TestAllDialogs_LayoutValidation` and moves in this commit. Line 1187 skips
     that test **globally** (`-skip '^TestAllDialogs_LayoutValidation$'`) and lines
     1193-1194 re-run it single-threaded only when the target list contains
     `./...` or `github.com/unxed/f4/cmd/f4`. After the move the test matches
     neither: it is skipped everywhere and re-run nowhere. **The build stays green
     and the test silently stops running.** Repoint the isolated re-run at
     `./internal/dialog`, keep the global skip, and confirm the test actually ran.

     **But `dialog_layouts_test.go` cannot move whole in this commit.** It calls
     `NewPanelsFrame()` (`:234`), `NewFileSystemPanel(…)` (`:235-236`) and casts
     `panels.panels[0].(*FileSystemPanel)` (`:242-243`, `:268-269`) — types that do
     not leave `cmd/f4` until Task 34, and which `internal/dialog` may never
     import. Choose one and record it in the commit message:
     - **split** — the layout assertions that need no panel move now; the
       frame-driven cases stay in `cmd/f4` and travel to `internal/panel` at Task
       34 as `package panel_test`; or
     - **defer the whole file** — to Task 36, not Task 34: besides the panel
       constructors it calls `showEditor` and `showViewer` (`actions.go`), which
       leave with `internal/app`. It then lands as `package dialog_test` in
       `internal/dialog` (Task 43's multi-package table), and the CI re-run stays
       pointed at `./cmd/f4` until that commit repoints it at `./internal/dialog`.

     Either way the isolated re-run must name a target where the test actually
     lives at that commit. Pointing it at `./internal/dialog` while the test is
     still elsewhere reproduces exactly the silent-skip failure this step exists to
     prevent.
6. Rename to the topic convention: `help.go`, `help_search.go`, `palette.go`,
   `palette_search.go`, `palette_ui.go`, `table.go`, `buttons.go`, `file.go`,
   `goto.go`, `settings_portable.go`, `settings_codepage.go`,
   `settings_hotkeys.go`, `settings_proxy.go`.
7. Add `"internal/dialog": 3` to the auditor's layer map.

### What the wave actually found

`internal/dialog` came out far smaller than the roster promised, and the reason
is uniform: most of what is called a dialog in this tree is a view over a panel,
an editor or the action registry, and the eight-type gate cannot see that.

Moved: `help.go` (split), `help_search.go`, `help/`, `table.go`, `buttons.go`,
`file.go`, `goto.go`, `settings_proxy.go`, `settings_portable.go`,
`settings_codepage.go`, `path.go`.

Did not move, with the reason each time:

| file | why |
|---|---|
| the thirteen `command_palette*.go` | every one builds `commandPaletteEntry` values whose closures reach `ArkanoidFrame`, `GrabberFrame`, `ImageView`, `QueueFrame`, `MacroMgr` and the action functions. The palette is a view over the whole application. It goes with `internal/app`. |
| `hotkeys_ui.go` | `HotkeyManager`, `GlobalHotkeysMgr`, `FormatKeyForUI`, `configuredHotkeyBinding` and four plugin hotkey helpers |
| `colorer_settings.go` | the colorer engine's scheme and region calls, and `DownloadColorerSchemas`, which takes the panel frame for its progress task |
| `startup_settings.go` | the startup backend tables in `startup_backend.go` |
| `compare_folders_ui.go` | `captureComparePanel` takes a `*FileSystemPanel`; `runCompareFolders` takes the frame |
| `share_dialog.go`, `find_file.go`, `bookmarks_dialog.go` | each keeps a `*PanelsFrame` in a struct field. They belong to Task 34. |
| `grabber.go` | its gate score of 1 is a **comment**; the real blocker is `setClipboardAsync`, which Task 30 takes to `internal/terminal`. Deferred to that wave, where the import becomes legal at 3 to 1. |

Two of the four gate scores this task quoted were false positives — `grabber.go`
(a comment) and `colors.go` in Task 24 (string literals). The gate counts names,
and a name can be a string or a comment.

`dialog_layouts_test.go` took the task's "defer the whole file" option and stays
in `cmd/f4`, so the CI re-run of `TestAllDialogs_LayoutValidation` keeps naming
`./cmd/f4` — repointing it at `./internal/dialog` now is exactly the silent skip
step 5 exists to prevent.

### Required Interfaces and Contracts

- `internal/dialog` may import `internal/config`, `internal/i18n`,
  `internal/theme`, `internal/keymap`, `internal/history`, `internal/toast`,
  `internal/action`, `vfs` and the vtui libraries. It must not import
  `internal/panel`, `internal/editor`, `internal/viewer`, `internal/cmdline`,
  `internal/terminal` or `internal/app`.
- Dialog error strings are user-visible text; `ST1005` is disabled for that
  reason. Do not reword any of them during the move.
- The `help/en.hlf` format and the help topic keys are unchanged.
- The command palette's audited entry points keep their symbol names; only the
  package qualifier in `command_palette_coverage_test.go` changes
  (`main.X` → `dialog.X`), through the one-line map from Task 2.

### Error Handling and Logging

Unchanged. Dialogs report to the user through the dialog itself; nothing here logs
and nothing writes to stdout, which is the rendered UI.

### Tests

Take the 27 files Task 43's roster lists for `internal/dialog`, among them
`command_palette_test.go`, `file_dialog_test.go`, `grabber_mouse_test.go`, the
five `help_keys*_test.go`, `help_lang_test.go` and `envman_help_test.go`.
`help_lang_test.go` reads `lang/*.lng` as well as `help/*.hlf`: point its lang
glob at `internal/i18n/lang` through `testutil.ModuleRootDir` and guard both sets
against coming back empty — it has no guard today. Also repoint the help path of
the four `internal/i18n` tests Task 24 left at `cmd/f4/help` to
`internal/dialog/help`. Three exceptions:
- `command_palette_coverage_test.go` **stays in `cmd/f4`** — it is the module-wide
  auditor.
- `command_palette_dynamic_test.go` drives the palette across nine packages
  (dialog, panel, fileops, macro, cmdline, app, media, plughost, editor by the
  graph). Task 43 hosts it in `internal/app` (Task 36); this wave exports the
  twelve `commandPalette*` entry builders it needs, per the multi-package table.
- `dialog_layouts_test.go` follows the step-5 decision above, not this list.

```
go test ./internal/dialog/...
GOMAXPROCS=1 go test -timeout 15m -run '^TestAllDialogs_LayoutValidation$' ./internal/dialog -v
go test ./cmd/f4 -run '^TestCommandPalette'
```

### Acceptance Criteria

- `TestAllDialogs_LayoutValidation` reports `--- PASS`, not `--- SKIP` and not
  absent, from the CI command in step 5 — run against whichever package step 5's
  decision put it in.
- `find internal/dialog/help -name '*.hlf'` matches the pre-move count.
- `grep -rn 'cmd/f4/help' . --exclude-dir=.git` returns nothing.
- `command_palette_coverage_test.go` still has 42 keys and passes.

### Verification

- `GOMAXPROCS=1 go test -timeout 15m -run '^TestAllDialogs_LayoutValidation$' ./internal/dialog -v`
- Expected result: `--- PASS`. A `no tests to run` here is the silent-drop failure
  this task exists to prevent.
- `go test -timeout 25m ./...`
- Expected result: identical to the Task 1 baseline.

---

## Task 26: Extract `internal/plughost`

### Intent

All four plugin transports behind one host: in-process Go, RPC, Lua via
`internal/luaplug`, and WASM via `wazero`. The rest of the application talks to
plugins through the host, never to a transport directly; the contract a plugin
compiles against is `sdk/`.

Three files the call graph assigns here despite their names: `api.go` (the
`coreAPI` type — 12 references, this is its home), `sqlite_actions.go` and
`plugin_hotkeys.go`.

### Implementation Steps

1. Apply the gate. Zero-score, move whole: `api.go` (its 12 references are to
   `coreAPI` itself, which lands here), `extui_host.go`, `plughost_ffi.go`,
   `rpc_plugin.go`, `rpc_panel.go`, `rpc_vfs.go`, `rpc_commands.go`,
   `wasm_plugin.go`, `lua_plugin.go`, `plugin_permissions.go`,
   `plugin_permissions_ui.go` (declined by Task 25: its
   `actionPluginPermissions(store *PermissionStore)` binds it to
   `plugin_permissions.go`, which moves in this same commit),
   `plugin_scaffold.go`, `sqlite_actions.go`.
   Task 43 also assigns `plugring.go`, `plugring_meta.go` and `plugring_ui.go`
   here — Task 15 edits `plugring.go`'s catalogue URL but never gives it a package.
   Score all three before moving; `plugring_ui.go` is gate 3.
2. Resolve the non-zeros: `plughost.go` (2), `plugins.go` (1),
   `plugin_hotkeys.go` (1), `plugin_contributions.go` (4).
3. `panel_plugins.go` scores `pluginPanelInstance` ×17, `PanelsFrame` ×5,
   `FileSystemPanel` ×4, `coreAPI` ×1. It goes to `internal/panel` (Task 34), not
   here — but **cut its single `coreAPI` method out now**, into this package, as a
   free function taking the panel type it needs. Doing it here rather than in Task
   34 means `internal/panel` never has to reach back into plughost.
4. `extui_host.go` is the heaviest consumer of `internal/numeric` (five of the
   seven `Bounded*` helpers). Confirm the import resolves.
5. Rename to the topic convention: `host.go`, `api.go`, `transport_rpc.go`,
   `transport_wasm.go`, `transport_lua.go`, `permissions.go`, `scaffold.go`,
   `hotkeys.go`, `actions_sqlite.go`.
6. Add `"internal/plughost": 2` to the auditor's layer map.

### What the wave actually found

The interface is **seven methods, not five**, and the count moved twice while it
was being measured — which is why each of the four waves after this one measures
its own instead of copying this number.

- Two of the five in the finding are not needed. `vfs.App` already declares
  `RunProgressTask` and `Menu`, so reaching the current application covers both:
  one `Current() vfs.App` replaced two entries.
- Four were missing. `AskOverwrite` and `AskError` (`file_ops.go`, layer 3) are
  called from `newHostMethods` and the finding did not count them.
  `OpenPanelProvider` is what step 3's cut needs — the provider registry has to
  sit below both `coreAPI` and `internal/panel`, so opening one is the piece
  that stays above. `IsStale` carries the panel-liveness check that
  `executeRegisteredPluginCommand` did inline with a `*PanelsFrame` type
  assertion; translating it as `Current() != app` silently rejected every app
  object that is not a panels frame, which two tests caught.

**Four files the task assigns here do not belong here**, all of them assigned by
name rather than by the call graph:

- `api.go` — the finding already moved it out; `coreAPI` implements `vfs.HostAPI`
  and calls five application functions.
- `plugin_hotkeys.go` — every binding function takes `*HotkeyManager`, and
  `pluginMenuKeyLabels` is called from `panels_frame.go`. It travels to
  `internal/app`, so `command_palette_coverage_test.go`'s map entry becomes
  `app` rather than being deleted.
- `plugring_ui.go` — three `*PanelsFrame` functions; it is the catalogue's UI.
- `sqlite_actions.go` — registers an application action from `init()`. Nothing
  about it is host plumbing.

`plugin_contributions.go` and `panel_plugins.go` **split** rather than move: the
two registries go to the host because `internal/panel` reads them, while
`coreAPI`'s methods, `actionPluginConfiguration` and the whole
`pluginPanelInstance` machinery stay above.

**`transport_wasm.go` is not a legal filename.** Go reads `_wasm` as an implicit
GOARCH constraint, so the file compiles nowhere except `GOARCH=wasm` and the
package still builds — with the WASM transport silently absent. It is
`transport_wazero.go`, with a comment saying why.

**The palette auditor had a latent bug this wave triggered.**
`commandPaletteCountF4Surfaces` derived "which packages are ours" from
`commandPaletteTargetPackage`, a map documented to empty itself as waves land.
Removing the last entry naming `plughost` dropped a live surface from the count.
It now reads `architectureLayers`, so the constant survives the map emptying.

**The plugin registries stayed behind on purpose.** `PluginMenuItems` and
`GlobalHotkeys` share `pluginRegistryMu` and are read by some thirty `cmd/f4`
files; their only plugin-facing entry is `coreAPI`, which stays. Moving them
would have exported a mutex-shared pair for no boundary gain.

### Required Interfaces and Contracts

- `internal/plughost` imports `sdk/`, `vfs`, `internal/luaplug`,
  `internal/numeric`, `internal/action`, `internal/config`, `internal/i18n` and
  `github.com/tetratelabs/wazero`. It must not import `internal/panel`,
  `internal/app`, or any interactive subsystem.
- `coreAPI`'s method set is the plugin-facing surface. It is unexported today and
  stays unexported; the host exposes it through the transports.
- `sdk/` is unchanged by this task. Task 8's auditor rule 1 confirms `sdk/` still
  imports no `internal/`.
- A lower layer that needs something from the host defines its own interface —
  `internal/panel` declares `PluginColumns`, it does not import plughost.

### Error Handling and Logging

Transport failures already wrap with context (`fmt.Errorf("…: %w", err)`) and
compare with `errors.Is`/`errors.As`. Preserve the sentinel errors declared at
package level. Plugin crashes surface to the user as dialog text; keep those
strings verbatim.

### Tests

The 21 files Task 43's roster lists for `internal/plughost` move, including
`plugin_hotkeys_test.go`, `sqlite_actions_test.go`, `extui_test.go`,
`plugin_identity_test.go`, `plugring_policy_test.go`, `plugring_rows_test.go` and
the transport fixtures' tests. The dummy plugins
under `plugins/dummy_internal`, `plugins/dummy_rpc` and `plugins/dummy_lua` are
transport fixtures and stay where they are — check that their tests still find the
host.

```
go test ./internal/plughost/... ./plugins/...
go test -race ./internal/plughost/...
```

### Acceptance Criteria

- `go list -f '{{join .Imports "\n"}}' ./internal/plughost | grep -E 'internal/(panel|editor|viewer|cmdline|app)'`
  returns nothing.
- `panel_plugins.go` no longer declares a `coreAPI` method.
- All four transports' tests pass.

### Verification

- `go test ./internal/plughost/... ./plugins/... -count=1`
- Expected result: `ok`, matching the Task 1 baseline.

---

## Task 27: Extract `internal/gui`

### Intent

GUI backends, font handling, window position and the application icon. Seven
outbound edges. This package is often confused with `internal/terminal`: the
distinction the graph draws is that anything deciding *what the terminal supports*
is term, and anything drawing *a window* is gui.

### Implementation Steps

1. Apply the gate. Zero-score, move whole — eighteen files:
   `gui_backend_capability.go`, `gui_backend_capability_ffi.go`,
   `gui_backend_capability_stub.go`, `gui_font.go`, `gui_font_catalog.go`,
   `gui_font_catalog_unix.go`, `gui_font_catalog_windows.go`, `gui_font_combo.go`,
   `gui_font_notwindows.go`, `gui_font_windows.go`, `gui_unix.go`,
   `gui_windows.go`, `window_icon_darwin.go`, `window_icon_unix.go`,
   `window_icon_windows.go`, `window_position.go`, `winepath_other.go`,
   `winepath_windows.go`. Verify each build tag from the file, not the name.
2. `git mv cmd/f4/assets internal/gui/assets`. `window_icon_darwin.go:18`'s
   `//go:embed assets/icon/generated/f4.icns` is directory-relative and needs no
   edit.
3. **`dragdrop.go` does not come here.** The call graph associates it with the GUI,
   but it declares five `*PanelsFrame` methods and one `*FileSystemPanel` method. Go requires a type's methods in the type's package,
   so it travels **whole** to `internal/panel` as `frame_dragdrop.go` (Task 34). Do
   not split it.
4. **Infrastructure, in this commit:**
   - `build.yml:93` — `go generate ./cmd/f4`. Confirm what the directive
     generates: if the `go:generate` line lives in a file that moved, the target
     moves with it.
   - `build.yml:95-97` — the cached paths `cmd/f4/assets/icon/generated`,
     `cmd/f4/rsrc_windows_amd64.syso`, `cmd/f4/rsrc_windows_arm64.syso`. The icon
     directory moves; **the two `.syso` paths stay**, because the toolchain links
     `.syso` only from the directory of the package being built.
   - `build.yml:209`, `:211`, `:215`, and the packaging copies beside them — packaging copies of the
     generated PNGs, the SVG and `f4.icns`.
   - `tools/icons/main.go` has three path constructions and **only two move**:
     `iconDir` (`:37`) and `outDir` (`:38`) become
     `filepath.Join(root, "internal", "gui", "assets", "icon"…)`;
     `cmd.Dir = filepath.Join(root, "cmd", "f4")` at `:160` **stays** — its own
     comment says why. Rewriting all three is the likely mistake in this commit.
     The doc comment at `:1-2` moves with `iconDir`.
   - `tools/icons/main_test.go:71` is red before this work (Task 1) and for an
     unrelated reason: it reads `../../assets/icon/f4.svg` while the tool reads
     `../../cmd/f4/assets/icon/`. Leave it red; say so in the commit message.
5. Rename to the topic convention: `backend.go`, `backend_ffi.go`,
   `backend_stub.go`, `font.go`, `font_catalog.go`, `font_catalog_unix.go`,
   `font_catalog_windows.go`, `font_combo.go`, `window.go`, `icon_darwin.go`,
   `icon_unix.go`, `icon_windows.go`, `winepath_other.go`, `winepath_windows.go`.
6. Add `"internal/gui": 1` to the auditor's layer map.

### Required Interfaces and Contracts

- Build tags are load-bearing here: `gui_font_notwindows.go` is `!windows`,
  `gui_font_windows.go` is `windows`, `window_icon_darwin.go` is `darwin`,
  `gui_backend_capability_ffi.go` and `_stub.go` split on the `noffi` tag
  expression (`//go:build !noffi && !android && (windows || …)` and its negation).
  Copy each `//go:build` line verbatim.
- `CGO_ENABLED=0` holds: FFI goes through `purego` / `ffibridge`. Do not introduce
  cgo while moving the backend files.
- The embedded `f4.icns` is byte-identical.

### Error Handling and Logging

GUI backend selection already falls back to the terminal path and reports to
stderr with the `f4: ` prefix when a backend cannot start. Preserve the messages.

### Tests

The six files Task 43's roster lists for `internal/gui` move:
`gui_backend_capability_test.go`, `gui_font_catalog_test.go`, `gui_font_test.go`,
`gui_font_windows_test.go`, `gui_unix_test.go`, `window_icon_windows_test.go`.
There is no window-position test.

```
go test ./internal/gui/...
for t in linux/amd64 darwin/arm64 windows/amd64 windows/arm64 freebsd/amd64; do
  extra=(); case $t in freebsd/*|netbsd/*) extra=(-gcflags=github.com/go-webgpu/goffi/internal/fakecgo=-std);; esac
  GOOS=${t%/*} GOARCH=${t#*/} CGO_ENABLED=0 go build "${extra[@]}" ./... || echo "FAIL $t"
done
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -tags noffi ./...
```

The `noffi` build is the one most likely to break: it selects the stub backend.

### Acceptance Criteria

- `ls cmd/f4/assets` fails; `ls internal/gui/assets/icon/f4.svg` succeeds.
- `ls cmd/f4/rsrc_windows_amd64.syso` still succeeds.
- `grep -n 'cmd", "f4"' tools/icons/main.go` returns exactly one line — `:160`.
- `go generate ./...` regenerates the icons into the new location and the `.syso`
  files into `cmd/f4`.

### Verification

- `go -C tools/icons run . && git status --porcelain internal/gui/assets cmd/f4`
- Expected result: regenerated files land in `internal/gui/assets/icon/generated`
  and `cmd/f4/rsrc_windows_*.syso`, nowhere else.
- The cross-compile loop and the `noffi` build print no `FAIL`.

---

### What the wave actually found

**`internal/gui` is layer 2, not the 1 the task text assigns.** Both `RunGui`
variants route the `qt` and `ext:` backends to
`plughost.RunExternalUIWithMapping`, so gui sits beside the host rather than
below it. Rule 6 allows the same-layer edge; a `1` would have failed the
auditor.

**No application interface was needed — one callback was enough.** The package
named three things above it: `SetupUI`, `openDashEFileIfRequested` and
`withGUIRuntime`. The first two are always called together, in order, from the
one place a window is created, so `RunGui` takes a `setupUI func()` and
`main.go` supplies both. The third disappeared: `runningGUI` moved down into
this package as `gui.Running`, because `clipboard.go` (term, Task 30),
`external_editor_process_unix.go` (editor, Task 33) and `session_*.go` all read
it, and each of those waves would otherwise have needed a seam of its own.

**`gui_font_catalog_test.go` split.** Seven of its eight tests are about the
font catalogue; `TestAppearanceSettingsFontComboRemainsEditable` drives the
appearance dialog through a real `PanelsFrame` and stayed behind as
`cmd/f4/gui_font_combo_dialog_test.go`, which is what made
`discoverInstalledGuiFonts` an exported seam.

**Two things the task told the wave to check, both confirmed:** `go generate`
stays as `./cmd/f4`, because the directive is in `main.go`; and of the three
path constructions in `tools/icons/main.go` only `iconDir` and `outDir` move —
`cmd.Dir` stays, along with `build.yml`'s two `.syso` cache paths. Running the
generator to prove the new paths work rewrites every committed icon binary with
different bytes and emits three sizes the repository does not carry, so its
output was reverted; that drift is real but it is not this commit's business.

## Task 28: Extract `internal/macro`

### Intent

The macro engine and the Lua macro API. Seven outbound edges. Macros drive the
application by name, through the action registry that left in Phase 4 — which is
why this wave is possible before the interactive subsystems exist.

### Implementation Steps

1. Apply the gate. `macro_export.go` scores 0 and moves whole. Resolve
   `macro.go` (6), `macro_host.go` (2), `macro_lua_api.go` (2), `macro_lua.go`
   (1), `macro_plugin_calls.go` (1) reference by reference.
2. `macro.go`'s six references are the substantive ones: a macro that drives a
   panel or an editor needs the type. The resolution is the project's stated rule —
   a method whose type lives elsewhere but whose logic belongs here becomes a plain
   function taking the type. Where the logic belongs to the *view*, leave it behind
   and let the view call into `internal/macro`.
3. `macro_lua_api.go` uses `numeric.BoundedRune`; confirm the import resolves.
4. Rename to the topic convention: `engine.go`, `host.go`, `lua.go`,
   `lua_api.go`, `plugin_calls.go`, `export.go`.
5. Add `"internal/macro": 3` to the auditor's layer map.

### Required Interfaces and Contracts

- `internal/macro` may import `internal/action`, `internal/luaplug`,
  `internal/config`, `internal/i18n`, `internal/numeric`, `internal/toast`, `vfs`.
  It must not import `internal/panel`, `internal/editor`, `internal/viewer` or
  `internal/app`.
- Macro command names are the stable `Action.Name` IDs (`"Editor.Save"` shape).
  They are user-facing in `.lua` macro files and must not change.
- The Lua API surface is a compatibility contract with user macros: no function is
  renamed, no argument order changes.

### Error Handling and Logging

Macro errors reach the user as dialog text through `internal/toast` or a modal.
Keep the wrapping (`fmt.Errorf("running macro %q: %w", …)`) and the sentinel
errors. `VTUI_DEBUG` remains the only diagnostic channel; add nothing.

### Tests

The seven files Task 43's roster lists for `internal/macro` move:
`fkeys_hidden_panels_test.go`, `macro_ctrlletter_test.go`, `macro_export_test.go`,
`macro_lua_test.go`, `macro_plugin_calls_test.go`, `macro_reload_test.go`,
`macro_test.go`.

```
go test ./internal/macro/... ./internal/luaplug/...
```

### Acceptance Criteria

- No user-visible macro command name changed: diff the output of the registry dump
  before and after and require it to be empty.
- `go list -f '{{join .Imports "\n"}}' ./internal/macro | grep -E 'internal/(panel|editor|viewer|app)'`
  returns nothing.

### Verification

- `go test ./internal/macro/... ./cmd/f4/... -count=1`
- Expected result: `ok`, matching the Task 1 baseline.
- `go test ./cmd/f4 -run '^TestActionOrderIsStable'`
- Expected result: `--- PASS` — macros register actions, so a reorder shows here.

---

### What the wave actually found

**The interface was already there.** `MacroHost` — eleven methods — was written
before this branch, implemented by `f4MacroHost` and injected through
`NewLuaMacroEngine`. Nothing had to be designed; the wave only had to put the
two halves in different packages. That is what the previous three waves were
converging on, arrived at independently by whoever wrote the Lua engine.

**`macro.go` is not the macro engine.** It holds `MacroManager` — recording,
playback, storage, the assign dialog — and also `Filter`, `LookupHotkey` and
`GetCurrentArea`, which are the application's key router: they consult the
macro engine first and the hotkey manager, the action registry, the plugin
actions and the panels afterwards. Those three, with their five helpers, stayed
behind as free functions in `cmd/f4/macro_dispatch.go`; every field they touch
was already exported, so the split cost nothing. `ToggleRecording` now takes the
area instead of deriving it, because naming the current area means naming the
view types.

**Two methods followed the host they construct.** `LoadLuaMacros` and
`ReloadLuaMacros` built the engine with `f4MacroHost{}` inline; they take a
`MacroHost` now, which is the same injection the constructor already used.

**Three test files split rather than moved**, each keeping the half that tests
what stayed: the router (`macro_test.go`), the action registration
(`macro_reload_action_test.go`) and the palette's macro entries. The palette
test needed a host that records injected keys and got a four-method one
embedding the real host.

**`tools/icons/main_test.go` is fixed rather than left red.** It read
`../../assets/icon/f4.svg` while its own tool read `cmd/f4/assets/icon/` — one
segment above, so it had never passed. It now reads the same directory the tool
does, and the module's tests are green. Task 27 said to leave it; that was right
while the failure looked unrelated, and it is not: the test points at the icons
this phase moved.

**The qualifier hit a local named `macro`.** `macro_export_test.go` and
`macro_lua_test.go` both do `macro := engine.Find(…)`, so a package-qualifier
rewrite silently turned `macro.Description` into `Description`. It failed the
build rather than compiling wrong, but only because the field name is not
otherwise in scope.

## Phase Risks and Mitigations

- **Risk:** `TestAllDialogs_LayoutValidation` stops running and CI stays green.
  **Mitigation:** Task 25 step 5 and its Verification require an explicit
  `--- PASS`, treating `no tests to run` as failure.
- **Risk:** `dragdrop.go` is moved to `internal/gui` because the call graph
  associates it with the GUI, producing 13 unresolved `PanelsFrame` references.
  **Mitigation:** Task 27 step 3 forbids it explicitly and names the destination.
- **Risk:** all three `cmd/f4` paths in `tools/icons/main.go` are rewritten and the
  Windows `.syso` files are generated into the wrong directory, silently dropping
  the application icon and version resource from the Windows build.
  **Mitigation:** Task 27 step 4 names line 160 as the one that stays, and the
  acceptance criteria require exactly one surviving `"cmd", "f4"` occurrence.
- **Risk:** the `noffi` build breaks when the GUI capability files move, and it is
  only exercised on the exotic targets in CI.
  **Mitigation:** the `-tags noffi` build is in Task 27's Tests section.

## Phase Completion Checklist

- Every Task 25-28 satisfies its acceptance criteria.
- `internal/dialog`, `internal/plughost`, `internal/gui` and `internal/macro`
  exist and import no interactive subsystem and no `internal/app`.
- `cmd/f4/help` and `cmd/f4/assets` are gone; `cmd/f4/rsrc_windows_*.syso` remain.
- `go test -timeout 25m ./...` matches the Task 1 baseline.
- `go test ./cmd/f4 -run '^TestArchitecture'` passes with four new layer entries.
- `index.md` task checkboxes 25-28 are ticked.
