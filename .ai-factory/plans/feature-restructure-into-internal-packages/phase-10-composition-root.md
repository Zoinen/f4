# Phase 10: The Composition Root

Plan: [index.md](index.md)
Tasks: 36-37
Depends on: Phase 9

## Objective

`internal/app` owns the event loop, the bootstrap and the global application
state the interactive subsystems share. `cmd/f4` is reduced to what the target
layout says it holds: `main.go`, the four module-wide auditors, and the Windows
`.syso` files.

After this phase the compiler, not a convention, keeps every boundary in
`ARCHITECTURE.md`.

## Current-Code Evidence

| Path | Signal | Consequence |
|---|---|---|
| `cmd/f4/action_table.go` | 2554 lines, 173 `RegisterAction`, `PanelsFrame` ×114, `EditorView` ×47 | created in Task 18; this is where it lands |
| `cmd/f4/framework_actions.go` | 25 functions; 18 have **no external callers** — reached only as `Handler:` values and from `main.go:actionScreenDump` | composition-root code, moves whole |
| `cmd/f4/actions.go` | after the earlier waves took 20 functions, the 61 view-bound handlers remain | distributed or moved here with the table |
| `cmd/f4/main.go` | flags, startup mode, wiring | stays in `cmd/f4` |
| `cmd/f4/startup_backend.go` | Task 4 removed `StartupMode` from it | moves to `internal/app` without dragging config up a layer |
| `cmd/f4/runtime_mode.go` | 1 gate reference | moves here |
| `cmd/f4/debug_log.go` | owns `VTUI_DEBUG` | moves here — it is process-wide diagnostics setup |
| `cmd/f4/command_palette_coverage_test.go` | 42 audit keys, module-wide invariant | **stays in `cmd/f4`** |
| `cmd/f4/architecture_test.go` | four rules plus the sysinfo leaf rule | **stays in `cmd/f4`** |
| `cmd/f4/rsrc_windows_amd64.syso`, `rsrc_windows_arm64.syso` | linked only from the built package's directory | **stay in `cmd/f4`** |
| `.github/actions/affected-packages/action.yml:50-54, 64-67, 76-77` | `cmd/f4/*)` treated as one indivisible unit, and the comment that justifies it | the reason for the special case is gone |

## Files to Change

| Path | Action | Required change |
|---|---|---|
| `internal/app/` | create | Event loop, bootstrap, action table, workspaces |
| `cmd/f4/main.go` | modify | Becomes wiring only |
| `.github/actions/affected-packages/action.yml` | modify | Drop the `cmd/f4` special case |
| `cmd/f4/architecture_test.go` | modify | Layer-4 entry; rule 3 now has teeth |

---

## Task 36: Extract `internal/app`

### Intent

What is left after Phase 9 is the application itself: the event loop that
dispatches input to the focused subsystem, the bootstrap, workspace management,
and the 2554-line action registration table whose closures reach into every
interactive package. This is the only place in the tree that is allowed to know
that all of them exist.

### Implementation Steps

1. Move `action_table.go` (Task 18). Its closures call into `internal/panel`,
   `internal/editor`, `internal/viewer`, `internal/dialog`, `internal/cmdline` and
   `internal/terminal` — legal here and nowhere else, because `internal/app` is the
   only package the dependency rules let import every layer 0-3 package.
2. Move `framework_actions.go` whole. The graph shows its 18 dependency-free
   functions have no external callers; they are `Handler:` values, and the seven
   that touch `PanelsFrame`/`QueueFrame` are workspace management, which belongs
   here. This is why Phase 4 declined to split it.
3. Move the remaining `actions.go` handlers. After Tasks 20, 24, 25, 29, 32 and 33
   took their slices, what is left is the 61 view-bound handlers plus
   `hostConsoleLogFallback`, the single `*PanelsFrame` method
   (`actions.go:2194`). Convert that method to a plain function taking the frame,
   per the project decision on cross-package methods — its type now lives in
   `internal/panel` and the method cannot follow it here.
4. Move the bootstrap: `startup_backend.go`, `startup_settings.go` (whatever Task
   25 did not take), `runtime_mode.go`, `debug_log.go`, `hang_dump_unix.go` /
   `hang_dump_windows.go`, `detach_unix.go` / `detach_windows.go`,
   `child_env.go`, `nested_input_other.go` / `nested_input_windows.go`,
   `console_ctrl_handler_other.go` / `console_ctrl_handler_windows.go`,
   `host_input_modes*.go`, `pe_subsystem.go`, `libc_default.go` / `libc_musl.go`
   (`//go:build linux && goffi_musl` and its negation), `title*.go`,
   `arkanoid.go`, `ai_chat_panel.go`, `vtvibe_host.go`, `vtvibe_ap.go`,
   `sheet_actions.go`, `sheet_dialogs.go`, `sheet_frame.go`, `sheet_palette.go`,
   `static_direct_actions.go`, `external_tools.go`, `highlight_files.go`,
   `farmenu_file.go`, `far2l_auth.go` and `async_buffer.go`. Score each with the
   gate first — several will turn out to belong to a package that already exists,
   and moving them there is a better answer than parking them in `app`.

   `main.go` is not exempt. It holds startup and session logic that is not
   wiring — `nestedInputMode`, `shouldTryGui`, `shouldPersistGUIWindowSize`,
   `startupDirs`, `startupDirsFor`, `startupDirArgs`, `rememberStartupDirs`,
   `sudoDispatcherPath`, `sudoStartupMode`, `LoadSession`, `SaveSession`,
   `mergeWorkspaceSessionSave`, `getSessionIniPath`, `formatVersionSHA`,
   `configureUnicodeInput` — and Task 37's single-digit file count is reachable
   only if that logic moves here. Its tests follow (`nested_input_mode_test.go`,
   `should_try_gui_test.go`, `session_test.go`, `startup_dir_test.go`,
   `sudo_dispatcher_args_test.go`, `autosave_settings_test.go`,
   `unicode_input_test.go`): nothing can import `package main`, so a test of
   `main.go` code can live nowhere else.

   **This list is closed.** There is no "and whatever else remains": Task 43
   assigned every `cmd/f4` file to a wave, so anything still sitting here that is
   not on this list is a Task 43 miss to be fixed there, not a file to park in
   `app`. Re-run Task 43 step 1's two `comm` commands before starting; both must
   come back empty.

   Note there is no `attributes.go` in `cmd/f4` — only `attributes_dialog.go` and
   its `_unix`/`_windows` pair, all three already taken by Task 32.
5. `internal/app` owns the process-wide startup calls Phase 1 made explicit:
   `StartQueueWorker()` (Task 5) and `action.Localize = i18n.Msg` (Tasks 21, 24).
   Both move from `cmd/f4/main.go` into the app's `New` / `Run`.
6. Rename to the topic convention: `app.go`, `loop.go`, `bootstrap.go`,
   `bootstrap_unix.go`, `bootstrap_windows.go`, `actions_table.go`,
   `actions_framework.go`, `workspace.go`, `debug.go`, `title.go`,
   `title_unix.go`, `title_windows.go`.
7. Add `"internal/app": 4` to the auditor's layer map. Rule 3 — nothing below
   layer 4 imports `internal/app` — now has something to check, and it must pass
   with **no exemptions**. An exemption here means a wave left an upward edge
   behind, and that is the failure this whole ordering exists to prevent.
8. Move `cmd/f4/action_table_order_test.go` — `TestActionOrderIsStable` and its
   golden slice, split out under that name in Task 21 step 6 — to `internal/app`,
   where the table now lives. The golden slice is unmodified since Task 3.


### What the wave actually found

**It needed no seams and exported nothing.** Every earlier wave spent most of
its effort on the boundary: what the lower package needs from above, declared as
a `var` with an inert default and filled in by the root. Here there is no above.
`internal/app` may import every layer 0-3 package, so each of the 235 files
landed with the calls it already made, and the only renames were the twenty the
topic convention asked for. Six hours of the panel wave went into seams; this
one took the afternoon.

**Rule 3 passes with no exemption**, which is the result the ordering was for. It
says nothing below layer 4 imports `internal/app`, and it had nothing to check
until this package existed. It found nothing, so no wave left an upward edge
behind — the failure the whole sequence was arranged to prevent did not happen
once in eleven phases.

**Five files the palette auditor's map sent elsewhere, and only one went.**
`commandPaletteTargetPackage` forward-declares where each audited `cmd/f4` file
will land, so an audit key survives the move. It predicted `dialog` for
`grabber.go`, `find_file.go`, `hotkeys_ui.go` and `command_palette_ui.go`, and
`panel` for `player_panel.go`. Measured against the graph:

- `player_panel.go` **went to `internal/panel`**. `PlayerPanel` is an alt panel
  beside `InfoPanel` and `QuickViewPanel`, which are already there, it has zero
  references into `app`, and `panel` already imports `media` and `theme`, so the
  move costs no new package edge.
- `find_file.go` calls `actionOpenEditor` and `actionOpenViewer`;
  `hotkeys_ui.go` calls the action table's `GetAction`; `command_palette_ui.go`
  is the palette's own rendering. All three are application code and stay.
- `share_dialog.go` and `grabber.go` have **zero** references into `app`, so
  neither is held here by need. `share_dialog.go` still cannot go to
  `internal/dialog`: it takes a `*panel.PanelsFrame`, and `internal/panel`
  imports `internal/dialog` already, so the edge would close a cycle.
  `grabber.go` could, at the price of a new `dialog -> terminal` edge for one
  file — Task 25 already declined it once, and Task 44 step 4a is where that
  question belongs, not in the wave that is only moving things.

The map has now emptied itself, as its own comment said it would. It is kept,
empty, because a file that moves again needs an entry for exactly one commit or
its key goes stale in a way that reads as a missing surface rather than a move.

**`cmd/f4` has no `TestMain`, and does not need one.** The plan expected a
five-line wrapper around `testutil.Main` for the auditors. The auditors parse
files and read imports; none of them builds a frame or reads the configuration.
The one test there that did — `TestCommandPaletteResolvesEveryActionGeneratedMenuLeafByID`,
which calls `BuildMenuBarItems` and the palette — is not module-wide and moved
to `internal/app` with its subject, leaving the file with the inventory auditor
alone. A `TestMain` here would be scaffolding for a need that left with it.

### Deviation: the `New`/`Run(ctx)` contract is not built

The task's Required Interfaces name `app.New(cfg, fs, host, t, left, right)` and
`(*App).Run(ctx) error`, with `main` reporting the error and exiting non-zero.
That is not what this commit does; `cmd/f4/main.go` calls `app.Main()`, which is
today's `main` verbatim.

**The reason is that the signature would be false.** Counted across `internal/`:

```
grep -rhoE 'vtui\.FrameManager|config\.App|keymap\.GlobalHotkeysMgr|macro\.MacroMgr' \
  --include='*.go' internal | wc -l
```

| global | references |
|---|---|
| `vtui.FrameManager` | 2575 |
| `config.App` | 1575 |
| `keymap.GlobalHotkeysMgr` | 241 |
| `macro.MacroMgr` | 99 |
| | **4490** |

A constructor that takes the configuration, the filesystem, the plugin host, the
terminal and the two panels, in a tree where 4490 sites reach for those things at
package level anyway, does not inject its dependencies — it lists them. The body
still goes to the package globals, and the signature starts asserting something
that is not true of the code beneath it.

That is the same shape this branch has been catching all along: a seam whose
default is inert, a sweep that finds nothing, a race job with an empty package
list — an interface that says a thing is being supplied while the code takes it
from somewhere else. The difference is that here it would not be in a test but
in the contract, where it is read as a description of the design.

The cost argument comes second and is weaker on its own, because expensive
things do get done: `main` exits through `os.Exit` at seven points — the update
helper, the plugin scaffolder, the mount CLI, the sudo dispatcher and three error
paths, thirteen across the package — and returns nothing, so threading an error
out of them is a redesign of the startup path. And the startup path has no test:
`main_test.go` does not exist, and the seven `main.go` tests the plan names all
exercise helpers rather than the entry point. A move whose correctness rests on
"the tests still pass" cannot also rewrite the one function the tests do not
reach. But that is a reason to sequence the work, not a reason the contract is
wrong; the 4490 is.

Two things follow, and both are written where they will be met:

- **Task 41** rewrites `ARCHITECTURE.md` from intent into fact. The line
  describing `app.New` and `(*App).Run(ctx)` describes an API that does not
  exist, and a document handed over as a description of the built tree cannot be
  wrong in its first paragraph about the composition root.
- **Task 44 step 2** carries the proposal, with its price rather than as an open
  question.

### Required Interfaces and Contracts

```go
// internal/app — the only place that knows every subsystem exists.
func New(cfg *config.Config, fs vfs.FileSystem, host *plughost.Host,
    t *term.Terminal, left, right *panel.Panel) *App

func (a *App) Run(ctx context.Context) error
```

- `internal/app` may import every layer 0-3 package. No layer 0-3 package may
  import it. Shared state flows down through constructor arguments, never up
  through an import.
- Registration order is unchanged: `TestActionOrderIsStable` is the proof, and it
  travels with the table.
- No `init()` in `internal/app` has a side effect beyond assignment. The queue
  worker starts from `Run`, not from import.
- Nothing mutates a struct after construction that another goroutine already
  reads. `config.App` is exempt by the stated rule: it is a plain package-level
  value in a layer-0 leaf.

### Error Handling and Logging

- `Run` returns an error; `main` reports it as
  `fmt.Fprintf(os.Stderr, "f4: %v\n", err)` and exits non-zero. That is the
  project's fatal-path convention and it must survive the move verbatim.
- `debug_log.go` keeps `VTUI_DEBUG` as the only diagnostic channel; `--debug` and
  `--log=1` still set it up. Add no logging framework and no stdout writes —
  stdout is the rendered UI.

### Tests

The 52 files Task 43's roster lists for `internal/app` move; after this commit
nothing remains in `cmd/f4` except the four module-wide auditors and their
`TestMain`. Among them: `framework_actions_test.go`, `actions_test.go`,
`arkanoid_test.go`, `ai_chat_panel_test.go`, `sheet_actions_test.go`,
`sheet_frame_test.go`, `vtvibe_host_test.go`, `startup_backend_test.go`, the
seven `main.go` tests named in step 4, the four `cloudfox_real_*_test.go`
end-to-end tests, the five registry-driven tests
(`action_copy_window_title_test.go`, `action_shortcut_conflict_test.go`,
`action_menu_visibility_test.go`, `command_palette_menu_test.go`,
`folder_history_actions_test.go`) and the handler-driven scenario tests the
multi-package table hosts here (`attributes_test.go`, `bom_test.go`,
`delete_trash_test.go`, `editor_binary_open_test.go`,
`command_palette_dynamic_test.go`, `history_hint_test.go`,
`folder_history_navigation_test.go`, `issue821_test.go`,
`updater_issue635_test.go`, `updater_repro_test.go`). **Not**
`action_registry_test.go` (it went to `internal/action` in Task 21), **not**
`macro_test.go` (Task 28), **not** `workspace_session_test.go` and
`workspace_routing_test.go` (panel tests, Task 34).

```
go test ./internal/app/...
go test -race -shuffle=on -timeout 10m ./internal/app/...
```

### Acceptance Criteria

- `go test ./cmd/f4 -run '^TestArchitecture'` passes, with rule 3 active and no
  exemption list.
- `TestActionOrderIsStable` passes in `internal/app` with its golden slice
  unmodified since Task 3.
- No `init()` in `internal/app` starts a goroutine or touches the filesystem.
- `grep -rn 'internal/app' --include='*.go' . | grep -v '^./cmd/f4\|^./internal/app'`
  returns nothing.

### Verification

- `go test ./cmd/f4 -run '^TestArchitecture' -v`
- Expected result: five subtests pass, rule 3 included.
- `go test ./internal/app -run '^TestActionOrderIsStable' -v` and
  `git diff --stat` on the golden file.
- Expected result: `--- PASS`, empty diff.
- `go test -timeout 25m ./...`
- Expected result: identical to the Task 1 baseline.

---

## Task 37: Reduce `cmd/f4` to the entry point

### Intent

`cmd/f4` becomes what the target layout says it is: the Composition Root and
nothing else. The four module-wide auditors stay because a per-package copy of
any of them would see only its own subtree.

### Implementation Steps

1. Confirm what remains and remove anything that does not belong:
   - `main.go` — flags, startup mode selection (terminal, GUI backend,
     `--update`, plugin scaffolding), and the construction of `app.New(...)`.
   - No test of the wiring itself exists today (`main_test.go` does not exist);
     one written later lives here. The `TestMain` that remains is the five-line
     wrapper around `testutil.Main` the auditors need.
   - `command_palette_coverage_test.go` — the module-wide palette auditor. Its 42
     keys are qualified symbols after Task 2 and its file→target-package map now has
     an entry per extracted package. **This is the commit where the map's `cmd/f4
     → main` entry becomes vestigial**; remove it if nothing audited remains in
     `main`.
   - `architecture_test.go` — the module boundary auditor.
   - `frame_manager_capture_test.go` — the third module-wide auditor: it parses
     every production file, through the palette auditor's
     `commandPaletteParseProductionGo`, for background work that captures
     `vtui.FrameManager`. It stays with the parser it shares.
   - `hardcoded_strings_test.go` — the fourth: `hardcode.Scan` over the module
     root against `tools/hardcoded_baseline.txt`, the L10N CI gate. Its
     `moduleRootDir` helper is `testutil.ModuleRootDir` after Task 9.
   - `rsrc_windows_amd64.syso`, `rsrc_windows_arm64.syso` — the toolchain links
     `.syso` only from the directory of the package being built, so they stay even
     though the icon code left in Task 27.
2. `.github/actions/affected-packages/action.yml` lines 50-54 and 76-77 map every
   path under `cmd/f4/` to the single unit `cmd/f4`; the comment at lines 64-67
   ("cmd/f4's other files always do because that package embeds non-Go
   resources") explains the second case. That was true of a 345-file flat package
   and is false of a package holding `main.go`. Remove the special
   case so the action computes affected packages normally. Verify against a
   synthetic diff that touches one `internal/` package and confirm the action
   reports that package alone.
3. Sweep the last references: `grep -rn 'cmd/f4/' . --exclude-dir=.git` should
   return only the build invocations (`go build ./cmd/f4` at `build.yml:170`,
   `174`, `400`, `403`, `579`, `808`, `810`), the `.syso` cache paths
   (`build.yml:96-97`), `tools/icons/main.go:160`, and `README.md:233`/`241`.
   Every one of those is correct and must stay.
4. Update `README.md:237` — "If `cmd/f4/assets/icon/f4.svg` is changed" — to the
   new icon location from Task 27, if Phase 6 did not already.


### What Task 37 actually found

**The `//go:generate` directive travelled with `main.go` and broke silently.**
It generates the icons and the two `rsrc_windows_*.syso` files, and the tool
writes the `.syso` into `cmd/f4` because the toolchain links a resource object
only from the directory of the main package. CI runs `go generate ./cmd/f4`.
Once the directive was in `internal/app`, that command matched no directive and
**exited 0**: the icons would have stopped being regenerated and every job would
have stayed green. Nothing in the tree checks that a generator ran.

It is back in `cmd/f4/main.go`, above the package comment, with the reason
written next to it. The general form is worth keeping: a directive is bound to
the directory a tool is told to look in, and moving the file it sits in moves it
out of that directory without a word from any check. Same family as a sweep that
finds nothing.

**Removing the `cmd/f4` special case changes little today, and that is not the
reason to remove it.** Both branches — the test-impact one mapping every path
under `cmd/f4/` to the single unit `cmd/f4`, and the build-impact one setting
`build=yes` for any of them — now give the same answer the general rules give:
a `.go` file resolves to its own directory, which is `cmd/f4`, and `cmd/f4` is a
main package so it is in the production graph either way. What the special case
actually cost was truth: its comment justified itself with "language files, help
pages, icons and testdata are read by cmd/f4's tests at run time", and none of
those has been there since phase 5. A future non-Go file under `cmd/f4/` would
have been attributed silently instead of forcing the full run the fallback
exists for.

`.syso` needed a branch of its own, since it is neither `.go` nor testdata and
would otherwise reach `all "cannot attribute"`. It resolves to its directory,
for the same reason the linker does.

**The sweep in step 3 cannot pass yet, and not because of this task.** It asks
that `grep -rn 'cmd/f4/'` return only the build invocations, the `.syso` cache
paths, `tools/icons/main.go` and two `README.md` lines. It also returns
`AGENTS.md` and eleven files under `docs/`, all naming files that moved — and
those documents are Tasks 40 and 42's subject, not this one's. What this task
could sweep, it swept: three comments in `internal/` naming `cmd/f4/semantic.go`
and `cmd/f4/editor_view.go` now name where the code is.

### Required Interfaces and Contracts

- `cmd/f4` is `package main`. Nothing can import it and nothing should want to;
  the auditor's rule 2 enforces the second half.
- `go build ./cmd/f4` and `go build -ldflags="-H windowsgui" ./cmd/f4` continue to
  produce the two binaries CI expects at the same paths.
- The `.syso` files are linked by name from this directory. Do not rename them and
  do not move them.
- The palette auditor's 42 keys stay 42. If a key's subject genuinely disappeared,
  that is a finding to report, not a number to adjust.

### Error Handling and Logging

`main` keeps the fatal-path convention exactly:

```go
if err := application.Run(context.Background()); err != nil {
    fmt.Fprintf(os.Stderr, "f4: %v\n", err)
    os.Exit(1)
}
```

Nothing else in `cmd/f4` reports errors.

### Tests

```
go test ./cmd/f4/...
go build ./cmd/f4 && ./f4 --version
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o /dev/null ./cmd/f4
```

The `--version` run is the cheapest end-to-end proof that the composition root
still wires a working binary.

### Acceptance Criteria

- `ls cmd/f4/*.go | grep -v _test | wc -l` is a single-digit number.
- `ls cmd/f4/*.syso | wc -l` returns `2`.
- The Windows build links the icon and version resource — check with
  `GOOS=windows GOARCH=amd64 go build -o /tmp/f4.exe ./cmd/f4` and confirm the
  binary is larger than a `.syso`-less build.
- `affected-packages` reports a single `internal/` package for a single-package
  diff.

### Verification

- `go build ./cmd/f4 && ./f4 --version`
- Expected result: the version string, exit 0.
- `go test -timeout 25m ./...`
- Expected result: identical to the Task 1 baseline.
- `go test ./cmd/f4 -run '^TestArchitecture|^TestCommandPalette' -v`
- Expected result: both pass; 42 palette keys.

---

## Phase Risks and Mitigations

- **Risk:** `internal/app` becomes the new flat package — everything unclaimed by
  a wave is parked there.
  **Mitigation:** Task 43 assigns every `cmd/f4` file to a wave before the waves
  start, so nothing arrives here unclaimed; Task 36 step 4's roster is closed and
  requires scoring each remaining file with the gate. A file that belongs to an
  existing package goes there instead. The measure of success is that
  `internal/app` holds the loop and the table, not 80 files.
- **Risk:** rule 3 fails and is silenced with an exemption list.
  **Mitigation:** Task 36 step 7 forbids exemptions and names what a failure means
  — an upward edge a wave left behind.
- **Risk:** the `.syso` files are moved or renamed with the icon code and the
  Windows binary silently loses its icon and version resource.
  **Mitigation:** Task 37's acceptance criteria check both the file count and the
  linked binary size.
- **Risk:** removing the `affected-packages` special case makes CI skip a package
  that a `cmd/f4` change really does affect.
  **Mitigation:** Task 37 step 2 requires a synthetic-diff check before the change
  is trusted.

## Phase Completion Checklist

- Every Task 36-37 satisfies its acceptance criteria.
- `cmd/f4` holds `main.go`, the four module-wide auditors, their `TestMain` and
  two `.syso` files.
- `go test ./cmd/f4 -run '^TestArchitecture'` passes with rule 3 active and no
  exemptions.
- `./f4 --version` runs from a fresh build.
- `go test -timeout 25m ./...` matches the Task 1 baseline.
- `index.md` task checkboxes 36-37 are ticked.
