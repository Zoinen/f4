# Phase 1: Upstream Sync, Baseline and Barrier Removal

Plan: [index.md](index.md)
Tasks: 0-9 and 43
Depends on: none

## Objective

The branch sits on the newest `upstream/main`, the tree still looks exactly as it
does today — no file has moved — the four structural barriers that would make an
intermediate commit uncompilable are gone, and "green" has a written definition to
compare against.

Every task in this phase is a behaviour-preserving change to `cmd/f4` in place.
None of them creates a package, and none of them may be folded into a later move
commit: a move commit must read as a rename.

## Current-Code Evidence

| Path | Symbols / lines | Why it matters |
|---|---|---|
| `cmd/f4/action_registry.go:24` | `type Action` | `Checked`/`Visible`/`Handler` are `func() bool` — the type is layer-0 clean |
| `cmd/f4/action_registry.go:121` | `RegisterAction` | 173 calls from `init()` here, more from six other files |
| `cmd/f4/action_registry.go:264-2817` | `func init()` | 2554 lines; mentions `PanelsFrame` 114×, `EditorView` 47× |
| `cmd/f4/config.go:352` | `var AppConfig = F4Config{…}` | 132 fields, read from 133 files (55 non-test) |
| `cmd/f4/navigation_mode.go:7` | `type PanelNavigationMode` | config field type declared outside `config.go` |
| `cmd/f4/compare_folders.go:61` | `type compareOptions` | same |
| `cmd/f4/startup_backend.go:11` | `type StartupMode` | same, and in composition-root code that leaves last |
| `cmd/f4/queue_manager.go:312-318` | `func init()` | constructs `GlobalQueueManager` and starts `workerLoop` on import |
| `cmd/f4/panels_frame.go:26-40` | `DriveEntry`, `DriveRegistry`, `RegisterDrive` | drive registry parked on the panel type; needs only `vfs` + `sync` |
| `cmd/f4/gpu_info_linux.go:113` | `Msg("InfoPanel.GPUWSLVirt")` | the one localization call inside the sysinfo family |
| `cmd/f4/command_palette_coverage_test.go` | 42 audit keys | keyed by `cmd/f4/<file>.go:(*Type).Method` |
| `cmd/f4/frame_manager_test_helpers_test.go:129` | `swapFrameManager` | 63 test files; calls `waitForAsyncClipboard`, `waitForDirectoryLoads` |
| `cmd/f4/panels_frame_test.go:794` | `setupMockPanelsFrame` | 28 test files; constructs `PanelsFrame`, `TerminalView`, `CommandLine`, `FileSystemPanel` |
| `cmd/f4/misc.go:12` | `ScreenRow` | five callers, all `_test.go` |
| `cmd/f4/frame_manager_test_helpers_test.go:52` | `appendFrameManagerScreenForTest` | 2 test files; moves with the harness |
| `cmd/f4/frame_manager_test_helpers_test.go:66` | `taskPumpGoroutineProfile` | 2 test files; the harness's own test is built on it |

## Files to Change

| Path | Action | Required change |
|---|---|---|
| backup branch | create | `backup/restructure-into-internal-packages-YYYY-MM-DD` before the rebase |
| `index.md` | modify | Base revision replaced with the post-rebase HEAD |
| `.ai-factory/RESTRUCTURE_BASELINE.md` | create | Written definition of "green before we started", per module |
| `cmd/f4/command_palette_coverage_test.go` | modify | Re-key 42 audit entries to qualified symbols |
| `cmd/f4/action_registry.go` | modify | Explicit registration ordinal + deterministic sort |
| `cmd/f4/action_registry_order_test.go` | create | Golden test over `actionOrder` |
| `cmd/f4/config.go` | modify | Receives three type declarations |
| `cmd/f4/navigation_mode.go` | delete | `PanelNavigationMode`, its constants, `String` and `ParsePanelNavigationMode` all move into `config.go`; nothing remains |
| `cmd/f4/compare_folders.go` | modify | Loses `compareOptions` |
| `cmd/f4/startup_backend.go` | modify | Loses `StartupMode` and its constants |
| `cmd/f4/queue_manager.go` | modify | `init()` no longer starts a goroutine |
| `cmd/f4/main.go` | modify | Starts the queue worker explicitly |
| `cmd/f4/panels_frame.go` | modify | Loses the drive registry |
| `cmd/f4/drive_registry.go` | create | `DriveEntry`, `DriveRegistry`, `RegisterDrive` |
| `cmd/f4/gpu_info_linux.go` | modify | Returns a message key, not a localized string |
| `cmd/f4/info_panel.go` | modify | Localizes the GPU model at render time |
| `cmd/f4/architecture_test.go` | create | Module boundary auditor |
| `internal/testutil/*.go` | create | Frame-manager harness with caller-supplied drains, plus the shared scaffolding Task 43 sends here: `Main`, `PressKey`, `DrainPendingTasks`, `DrainUITasks`, `ModuleRootDir`, `SkipIfNoRelevantChanges`, `RaceEnabled`, the bounded-conversion helpers |
| `internal/paneltest/doc.go` | create | Empty package with its contract documented; filled in Task 34 |
| `cmd/f4/frame_manager_test_helpers_test.go` | modify | Harness moves out; local drains remain |
| 63 + 28 `_test.go` call sites | modify | New harness call shape |

---

## Task 0: Synchronize with `upstream/main`

### Intent

While files still sit in `cmd/f4`, an upstream commit merges into them
automatically. Once two hundred of them have moved to `internal/*` with a changed
`package` clause, git's rename detection starts missing, and the same upstream
commit becomes a manual conflict resolution inside someone else's change — one the
implementer did not write and does not understand.

This is not hypothetical. `c31f9f50` ("test: isolate terminal busy drive menu
frame") touches `cmd/f4/panels_frame_test.go` — the file that holds
`setupMockPanelsFrame` at line 794, which Task 9 refactors and Task 34 moves.
Upstream gained a commit in a first-wave file in the course of one day.

**One sync has already been performed**, onto `upstream/main` = `c31f9f50`; the
branch is `83177611` and `git rev-list --left-right --count upstream/main...HEAD`
currently reports `0 14`. That does not retire this task: time passes between
planning and execution, and the window has to be re-measured rather than assumed.
Run the steps below and expect them to be cheap, not to be no-ops.

Task 0 and Task 1 run as one sitting: a baseline is only meaningful for the
revision it was taken at, and rebasing after recording one invalidates it.

### Implementation Steps

1. `git fetch upstream`
2. **Create the backup branch before rebasing** — this is a standing project
   requirement for any rebase, not a suggestion:
   ```
   git branch backup/restructure-into-internal-packages-YYYY-MM-DD
   ```
3. `git rebase upstream/main`
4. Resolve any conflict. At this point every file is still in `cmd/f4`, so a
   conflict is a normal textual one in a file both sides edited — which is exactly
   why this happens now and not later.
5. Re-verify the plan's load-bearing numbers against the new revision. Upstream may
   have moved them, and a shift here changes Tasks 16-21:
   ```
   ls cmd/f4/*.go | grep -v '_test\.go$' | wc -l            # expect 345
   ls cmd/f4/*_test.go | wc -l                              # expect 346
   grep -c '^func ' cmd/f4/actions.go                       # expect 81
   # Receiver-anchored. The looser '^func (.*\*T)' form counts methods of OTHER
   # types that merely take a *T parameter and inflates every one of these.
   grep -lE '^func \([a-z]+ \*PanelsFrame\)' $(ls cmd/f4/*.go|grep -v _test) | wc -l      # expect 16
   grep -lE '^func \([a-z]+ \*EditorView\)' $(ls cmd/f4/*.go|grep -v _test) | wc -l       # expect 15
   grep -lE '^func \([a-z]+ \*FileSystemPanel\)' $(ls cmd/f4/*.go|grep -v _test) | wc -l  # expect 6
   grep -lE '^func \([a-z]+ \*TerminalView\)' $(ls cmd/f4/*.go|grep -v _test) | wc -l     # expect 5
   # Anchored on the declaration, not on a line number: the block has already
   # moved once and will move again.
   S=$(grep -n '^func init()' cmd/f4/action_registry.go | cut -d: -f1)
   awk -v s="$S" 'NR>s && /^}/ {print NR-s+1; exit}' cmd/f4/action_registry.go  # expect 2554 lines
   ```
   Also check whether upstream touched any file named in Phases 1-4:
   ```
   git diff --name-only <old-base>..upstream/main -- cmd/f4/
   ```
6. Where a number moved, update the affected task in this bundle **before**
   implementing it. A stale count in Task 16's split instruction produces an
   uncompilable commit.
7. Replace the base revision in `index.md`'s header with the new HEAD.
8. **Then do not touch upstream again until the pull request.** If the work runs
   long enough that a sync is genuinely needed, do it **between waves**, on a green
   commit with a consistent tree — never in the middle of one.

### Required Interfaces and Contracts

- The branch is `feature/restructure-into-internal-packages`. Do not recreate it;
  do not rename it.
- The backup branch exists before `git rebase` runs and is not deleted until the
  PR is merged.
- After this task, `git rev-list --left-right --count upstream/main...HEAD` reports
  `0` on the left.
- The eight harness commits already on the branch (`.ai-factory/`, `.claude/`,
  `.mcp.json`, `AGENTS.md`) are preserved. They ship in the PR deliberately.

### Error Handling and Logging

Not applicable — this is a git operation. A rebase conflict is resolved by hand,
not by `--strategy`; the branch's own commits are documentation and harness
configuration, and an automatic resolution can silently drop a hunk.

### Tests

The rebase is verified by the build and by Task 1's baseline run, which follows
immediately:

```
CGO_ENABLED=0 go build ./...
go vet ./...
```

### Acceptance Criteria

- `git rev-list --left-right --count upstream/main...HEAD` reports `0` upstream
  commits missing.
- A backup branch exists.
- Every number in step 5 either matches or has been corrected in this bundle.
- `index.md`'s base revision is the new HEAD.

### Verification

- `git rev-list --left-right --count upstream/main...HEAD`
- Expected result: `0` followed by the branch's own commit count.
- `git branch --list 'backup/*'`
- Expected result: one entry, dated today.

---

## Task 1: Record the pre-restructuring test baseline

### Intent

345 files are about to move. A red test after a move commit must be
distinguishable from a test that was already red, on a matrix of 26 build targets,
without re-investigating from scratch each time. Every later task's Verification
section compares against this file.

### Implementation Steps

1. Create `.ai-factory/RESTRUCTURE_BASELINE.md` with a header recording revision
   (`git rev-parse HEAD` — the post-rebase HEAD from Task 0, not the revision this
   plan was written at), `go version`, and the host platform the run was taken on.
2. Run and record the main module:
   ```
   CGO_ENABLED=0 go build ./...
   go vet ./...
   go test -timeout 25m ./...
   ```
   Record the exit code, the count of `ok` / `no test files` / `FAIL` packages,
   and the total package count from `go list ./... | wc -l`.
3. Run and record each of the other five modules separately. `go test ./...`
   from the repository root sees only the main module's 38 packages, so a failure
   in `tools/` is invisible to it:
   ```
   go -C tools/conptyreconcile test ./...
   go -C tools/icons test ./...
   go -C tools/wine_syscall_probe build ./...
   GOOS=windows GOARCH=amd64 go -C tools/wine_syscall_probe build ./...
   go -C internal/hideconsole build ./...
   ```
4. Record the three known non-green states verbatim, each with the reason, so
   nobody "fixes" them mid-migration:
   - `tools/icons` — `TestRenderScalesStrokesAndGradients` fails at
     `main_test.go:71`: it opens `../../assets/icon/f4.svg` while the tool itself
     (`main.go:37`) reads `../../cmd/f4/assets/icon/`. The test never agreed with
     its subject. **Pre-existing. Do not repoint it at `cmd/f4/assets/icon` as
     part of this work.**
   - `tools/wine_syscall_probe` — does not build on darwin/arm64: `main.go:5`
     declares `func rawGetpid() uint64` with its body in `probe_amd64.s`. Builds
     clean under `GOOS=windows GOARCH=amd64`. **A platform probe, not breakage.**
   - `tools/icons/third_party/oksvg` — vendored third-party, out of scope.
5. Record `cmd/f4/plugring_policy_test.go:40` as green-but-inert: it reads a
   CWD-relative `plugring/index.yaml` from `cmd/f4/`, always reaches `t.Skipf`,
   and asserts nothing. Task 15 decides its fate.
6. Record the tests that cannot travel with their file and must be reworked
   instead: `command_palette_coverage_test.go` (Task 2) and every test on the
   `swapFrameManager` / `setupMockPanelsFrame` harness (Task 9).

### Required Interfaces and Contracts

The file is prose consumed by a human and by the Verification step of every later
task. Structure it as one `##` section per module so a diff against a later run is
readable. Record counts, not full logs.

**The file is immutable.** It is a snapshot of the Task 0 revision and is never
rewritten from a later run. No wave updates it; the only task that touches it
after this one is Task 42, which deletes it. Rewriting it from a fresh run turns a
regression introduced on wave N into "known red" on wave N+1 and loses the signal
permanently — at wave fourteen, when nobody remembers whether that test was red to
begin with. Later tasks *compare against* it; they do not refresh it.

When a wave legitimately changes the test inventory — Task 9 converts three
packages' tests to `package X_test`, Task 2 re-keys the palette auditor's 42 audit
entries — that goes in the commit message, not in this file. The baseline answers
"what was true before all of this", never "what was true yesterday".

### Error Handling and Logging

Not applicable — no product code changes. If a command in step 2 fails, that is a
finding to record, not an error to suppress.

### Tests

No new tests. This task *is* the test record.

### Acceptance Criteria

- `.ai-factory/RESTRUCTURE_BASELINE.md` exists and names the revision it was taken
  at.
- All six modules have a section.
- The three known non-green states are recorded with their reason.

### Verification

- `git rev-parse HEAD` matches the revision recorded in the file.
- Expected result: identical strings.

---

## Task 2: Re-key the command-palette auditor to qualified symbols

### Intent

`command_palette_coverage_test.go` walks the whole module and asserts that every
`ProcessKey` and every `vtui.NewVMenu` is either reachable from the command
palette or listed as a deliberate exception. It is the only module-wide invariant
in the suite and it stays in `cmd/f4` — a per-package copy would see only its own
subtree and would miss a handler added in a third package, which is the thing the
test exists to catch.

Its 42 audit keys are file paths, so every one of the 27 commits in this plan
would rewrite some of them. Re-key once, to something that does not change when a
file moves.

### Implementation Steps

1. Locate the key-producing code in `cmd/f4/command_palette_coverage_test.go` and
   change the emitted key from `<file path>:<qualified symbol>` to
   `<package name>.<qualified symbol>`:
   `cmd/f4/file_panel.go:(*FileSystemPanel).ProcessKey` becomes
   `panel.(*FileSystemPanel).ProcessKey`.
2. Rewrite all 42 entries in the exception/audit list to the new shape, using the
   **target** package name so the list is never touched again.
   The 42 keys come from 35 distinct source files
   (`grep -o 'cmd/f4/[a-z0-9_]*\.go' cmd/f4/command_palette_coverage_test.go | sort -u`),
   and their destinations are **ten** packages, not six:
   - `app` — `actions.go`, `ai_chat_panel.go`, `arkanoid.go`, `sheet_frame.go`
   - `cmdline` — `apply_command_output.go`, `command_line.go`
   - `dialog` — `bookmarks_dialog.go`, `codepage_settings.go`,
     `command_palette_ui.go`, `find_file.go`, `grabber.go`, `hotkeys_ui.go`
   - `editor` — `editor_base64.go`, `editor_find_all.go`, `editor_view.go`
   - `fileops` — `queue_manager.go`
   - `macro` — `macro.go`
   - `media` — `image_view.go`, `player_panel.go`, `video_view.go`
   - `panel` — `drive_bookmarks_ui.go`, `file_associations_editor.go`,
     `fuse_mount_list.go`,
     `file_associations_ui.go`, `file_panel.go`, `info_panel.go`,
     `panel_plugins.go`, `panels_frame.go`, `quick_view_panel.go`,
     `temp_panel.go`, `user_menu_ui.go`, `viewer_editor_history.go`
   - `plughost` — `plugin_hotkeys.go`, `rpc_panel.go`
   - `viewer` — `viewer_view.go`

   `term` owns none of the 42 and must not appear in the list.
3. **The walker must emit the same target name from day one, while every file is
   still in `cmd/f4`** — otherwise the audit list says `panel.X`, the walker says
   `main.X`, and the test is red from this commit until Task 36. A
   file→target-package map cannot do this: at this task there is only one directory.
   Use a **file→target-package** map instead, seeded with all 35 entries above, and
   have the walker fall back to the file's actual directory for anything not in it.
   Each wave then deletes the entries it has satisfied — the map shrinks to nothing
   by Task 37, and the emitted keys never change.
4. Assert in the test that the audit list and the discovered set have the same
   cardinality (42), so a silent drop is a failure rather than a smaller list. Run
   the test at the end of this task and require `--- PASS`, not merely compilation:
   a mismatch here is the failure mode step 3 exists to prevent.

### Required Interfaces and Contracts

- Key format: `<package>.<receiver-qualified symbol>`, e.g.
  `panel.(*FileSystemPanel).ProcessKey`, `editor.(*EditorView).ProcessKey`.
- The file→target-package map is the only place a path appears. Its contract: one
  entry per audited file whose target package differs from its current directory,
  and it is empty once the last wave lands.
- Cardinality invariant: 42 audit keys before and after this task, and after every
  wave. If a key's subject genuinely disappears, that is a finding to report, not a
  number to adjust.

### Error Handling and Logging

The test's failure message must name the key that was not found and the key set it
searched, so a rename shows up as a readable diff and not as "expected 42, got
41". No logging changes — this is a test.

### Tests

This task modifies a test. Its own verification is that the test still passes and
still has 42 keys:

```
go test ./cmd/f4 -run '^TestCommandPalette' -v
```

### Acceptance Criteria

- No key in `command_palette_coverage_test.go` contains a `/` or `.go`.
- The test passes.
- The audit list has exactly 42 entries, unchanged in subject.

### Verification

- `grep -c 'cmd/f4/' cmd/f4/command_palette_coverage_test.go`
- Expected result: `0` inside the key list; the file→target-package map's 35
  entries are keyed by bare basename, not by path.
- `go test ./cmd/f4 -run '^TestCommandPalette' -v`
- Expected result: `--- PASS`. A failure naming a `main.`-prefixed key means step 3
  was implemented as a directory map.

---

## Task 3: Make action registration order explicit

### Intent

Inside one package Go runs `init()` in filename order; across packages it follows
the import graph. Today 173 `RegisterAction` calls sit in
`action_registry.go`'s `init()` and the rest in six more files
(`fuse_mount_action.go` — which has two `init()`s — `fuse_mount_list.go`,
`sheet_actions.go`, `sqlite_actions.go`, `static_direct_actions.go`,
`vtvibe_host.go`). That order is what the menus and the command palette *display*.
Splitting the registry across packages therefore reorders the user's menu silently,
and no test catches it.

### Implementation Steps

1. Add an explicit ordinal to the registry. `RegisterAction`
   (`action_registry.go:121`) currently appends to `actionOrder` on first sight;
   change it to record a monotonically increasing sequence number per action, or
   accept an explicit `Order int` on `Action`. Prefer the sequence number — it
   requires no edit to the 173 call sites.
2. Make every consumer of `actionOrder` sort by that ordinal explicitly rather
   than relying on append order. Find them with
   `npx -y @colbymchenry/codegraph@1.6.0 callers actionOrder`.
3. Because a sequence number assigned at call time still depends on `init()`
   order across packages, pin the order that must survive: capture today's
   `actionOrder` as a golden fixture (Task 3's test below) *before* changing
   anything, then make the sort key the position in that fixture for actions the
   fixture knows, appending unknown actions at the end. This turns the menu order
   from an emergent property of file names into recorded data.
4. Record in a comment on the ordering function that the order is observable
   behaviour and that changing it changes the user's menu.

### Required Interfaces and Contracts

- `RegisterAction(action Action)` keeps its signature. Adding a parameter would
  touch 173 call sites for no gain.
- Ordering is deterministic and independent of file names, package names and
  import order. This is the contract the later waves depend on.
- Duplicate registration keeps today's behaviour: `RegisterAction` already
  ignores a second registration of the same lowercase name and must continue to.

### Error Handling and Logging

No new failure modes and no new logging. A duplicate or missing action is a test
failure, not a runtime log line: this codebase reports user-facing failures to
stderr with the `f4: ` prefix and diagnostics through `VTUI_DEBUG`, and neither
applies to a registry that is fully determined at startup.

### Tests

New `cmd/f4/action_registry_order_test.go`:

- `TestActionOrderIsStable` — asserts the full ordered list of action names
  against a golden slice captured from the pre-change build. This is the test that
  makes a silent menu reorder loud.
- `TestActionOrderCoversRegistry` — every key in `actionRegistry` appears exactly
  once in `actionOrder`, and the lengths match.
- `TestActionOrderIndependentOfRegistrationSequence` — register a small synthetic
  set in two different sequences and assert the resulting order is the same.

Capture the golden slice by running the current binary's registry dump before the
change, not by transcribing `action_registry.go` by hand.

### Acceptance Criteria

- Menus and the command palette present actions in exactly the order they do
  today.
- Ordering does not consult file names, package names, or `init()` sequence.
- The three tests above pass.

### Verification

- `go test ./cmd/f4 -run '^TestActionOrder'`
- Expected result: `ok`, three tests passing.
- `go test ./cmd/f4 -run '^TestCommandPalette'`
- Expected result: `ok` — the palette's own coverage test still agrees.

---

## Task 4: Move `F4Config`'s field types into `config.go`

### Intent

`internal/config` must import no other `internal/*` package — that is the rule
that lets `config.App` stay a package-level global without creating a cycle.
`F4Config` (`config.go:352`) has five named-type fields; three of those types are
declared outside `config.go`. One of them, `StartupMode`, is declared in
composition-root code that leaves `cmd/f4` **last**, which would make layer 0
depend on layer 4.

### Implementation Steps

1. Move `type PanelNavigationMode int` and its constants
   (`NavigationClassic`, `NavigationVim`, `NavigationSearchFirst`) plus the
   `String()` method from `cmd/f4/navigation_mode.go:7-27` into `cmd/f4/config.go`,
   next to `PanelScrollbarMode` (`config.go:125`) and
   `WorkspaceTabNumberingMode` (`config.go:144`). Take `ParsePanelNavigationMode`
   (`:26-35`) with them — `config.go` is its only production caller — and
   `git rm cmd/f4/navigation_mode.go`: without the parser the file would keep ten
   lines that no wave claims. `navigation_mode_test.go` is not the parser's test;
   it drives search-first navigation on the frame, and Task 43 sends it to
   `internal/panel`.
2. Move `type compareOptions struct` from `cmd/f4/compare_folders.go:61` into
   `config.go`. Keep the field comments verbatim: they document the Advanced
   Compare dialog field by field. Its two methods, `normalize` and `hasCriteria`,
   come with it, and so do the three constants they and `defaultCompareOptions`
   read — `compareIgnoreEOL`, `compareIgnoreSpaces` and `compareMaxDepthLimit` —
   plus `defaultCompareOptions` itself, which `config.go:469` calls to seed
   `AppConfig.Compare`.
3. Move `type StartupMode int` and its constants from
   `cmd/f4/startup_backend.go:11` into `config.go`, together with its `String`
   method and `ParseStartupMode`, which `config.go:643` calls.
4. Leave every function and method that *uses* these types where it is. What
   moves is each type's own declaration set: the type, its constants, its
   methods, and whatever `config.go` itself calls.

   A method must travel with its receiver — Go refuses `func (o compareOptions)`
   in a package that does not declare `compareOptions`, so leaving `normalize` in
   `compare_folders.go` stops compiling the moment that file becomes
   `internal/dialog`. `PanelScrollbarMode` and `WorkspaceTabNumberingMode`
   (`config.go:125`, `:144`) already sit in `config.go` in exactly this shape:
   type, constants, `String`, parser.
5. Confirm no fourth type is hiding: re-derive the list with
   ```
   awk '/^type F4Config struct/,/^}/' cmd/f4/config.go |
     grep -E '^\s+[A-Z][A-Za-z0-9_]*\s+[A-Z]'
   ```
   and check each named type's declaration site.

### Required Interfaces and Contracts

- No identifier is renamed. `PanelNavigationMode` stays `PanelNavigationMode`;
  Far-derived names are never renamed during a move.
- `F4Config`'s field set, order and tags are unchanged — `config.go:352`'s literal
  initialiser must still compile untouched.
- After this task, every type named in a `F4Config` field is declared in
  `config.go`.

### Error Handling and Logging

None. This is a declaration move inside one package; the compiler is the only
check that matters.

### Tests

No new tests. The existing config suite must pass unchanged:

```
go test ./cmd/f4 -run '^TestConfig|^TestNavigationMode|^TestCompare'
```

Do not add a test asserting "these types live in config.go" — Task 8's boundary
auditor covers the invariant that actually matters (no upward import) once the
packages exist.

### Acceptance Criteria

- `ls cmd/f4/navigation_mode.go` fails; `grep -n 'func ParsePanelNavigationMode' cmd/f4/config.go` finds it.
- `grep -n '^type StartupMode' cmd/f4/startup_backend.go` returns nothing.
- `grep -n '^type compareOptions' cmd/f4/compare_folders.go` returns nothing.
- `grep -rn 'func (. compareOptions)\|func (. StartupMode)\|func (. PanelNavigationMode)' cmd/f4/`
  names `config.go` and nothing else.
- All five `F4Config` field types are declared in `config.go`.

### Verification

- `CGO_ENABLED=0 go build ./...`
- Expected result: exit 0.
- `go test -timeout 25m ./...`
- Expected result: identical to the Task 1 baseline.

---

## Task 5: Stop starting a goroutine from `init()`

### Intent

`queue_manager.go:308-317` constructs `GlobalQueueManager` and calls
`go GlobalQueueManager.workerLoop()` on import. Once this file lives in
`internal/fileops`, *importing that package* starts a worker — in every binary
that links it and in every test process that touches it, including tests that have
nothing to do with the queue. The composition-root principle bans exactly this:
`init()` with side effects beyond assignment.

### Implementation Steps

1. Keep the assignment half of `init()`. The zero-value manager must stay usable —
   `workerLoop`'s periodic fallback exists so that zero-value test managers work
   (see the comment at `queue_manager.go:138`), and tests rely on it.
2. Extract the `go GlobalQueueManager.workerLoop()` line into a new exported
   `StartQueueWorker()` in the same file, idempotent (guard with a `sync.Once` so a
   double call from a test cannot start two workers).
3. Call `StartQueueWorker()` from the startup path in `cmd/f4/main.go`, at the
   point where other process-wide services are started, before the event loop
   begins. It must run before the first enqueue; find the earliest enqueue with
   `npx -y @colbymchenry/codegraph@1.6.0 callers GlobalQueueManager`.
4. In `cmd/f4/queue_manager_test.go` and any other test that depends on a running
   worker, call `StartQueueWorker()` explicitly in the test setup. Tests that only
   enqueue and inspect state need no worker and must keep working without one.

### Required Interfaces and Contracts

```go
// StartQueueWorker starts the background scheduler. It is idempotent: the
// worker is started at most once per process.
func StartQueueWorker()
```

- `GlobalQueueManager` stays a package-level variable with its current type and
  field initialisation. Only the goroutine launch moves.
- Invariant preserved: enqueueing before the worker starts must still work — the
  wake channel is buffered (`wake: make(chan struct{}, 1)`) and `workerLoop` has a
  periodic fallback, so a task enqueued before start is picked up on the first
  cycle. Do not add a "not started" error path; that would be a behaviour change
  inside what must remain a mechanical refactor.

### Error Handling and Logging

No new failure modes. `StartQueueWorker` cannot fail. Do not add a log line for
"worker started": the project has no logging framework and this path is not a
user-facing failure.

### Tests

- Existing `cmd/f4/queue_manager_test.go` must pass. Add `StartQueueWorker()` to
  the setup of the tests that previously relied on the import-time start.
- Add `TestStartQueueWorkerIsIdempotent` — call it twice, assert exactly one
  worker goroutine, using the same `runtime/pprof` goroutine-profile technique
  `frame_manager_test_helpers_test.go` already uses for
  `TestFrameManagerShutdownStopsTaskPumpAndUnblocksPostTask`. That helper is
  `taskPumpGoroutineProfile` (`:66`); Task 9 moves it out as
  `testutil.TaskPumpGoroutineProfile`, so a test written here against the local
  name needs the import swapped when Task 9 lands. Task 32, which moves
  `queue_manager.go` and its tests into `internal/fileops`, depends on this task.

### Acceptance Criteria

- `grep -n 'go GlobalQueueManager' cmd/f4/queue_manager.go` returns nothing inside
  `init()`.
- No goroutine is running after importing the package in a test that does not call
  `StartQueueWorker`.
- The full suite matches the Task 1 baseline.

### Verification

- `go test ./cmd/f4 -run '^TestQueue|^TestStartQueueWorker' -race`
- Expected result: `ok`.
- `go test -timeout 25m ./...`
- Expected result: identical to the Task 1 baseline.

---

## Task 6: Lift the drive registry out of `panels_frame.go`

### Intent

`drives_unix.go` and `drives_windows.go` build `[]DriveEntry`, but `DriveEntry`,
`DriveRegistry` and `RegisterDrive` are declared at `panels_frame.go:26-40` — on
the panel type's file. The architecture assigns drives to `internal/sysinfo`, a
layer-0 leaf; leaving the type on `panels_frame.go` would mean sysinfo (Task 22,
the *first* wave) depends on panel (Task 34, the second-to-last). The registry
itself has no panel dependency at all: `DriveEntry` is `{Name string; Factory
func() vfs.VFS}` and `RegisterDrive` needs only `sync`.

### Implementation Steps

1. Create `cmd/f4/drive_registry.go` and move `type DriveEntry`
   (`panels_frame.go:26-29`), `var DriveRegistry` (`:31`), the mutex that guards it
   (`pluginRegistryMu`, `:32`) and `RegisterDrive` (`:34`) into it.
2. Check whether `pluginRegistryMu` guards anything besides the drive registry —
   its name suggests a wider role. `grep -n 'pluginRegistryMu' cmd/f4/*.go`. If it
   does, give the drive registry its own mutex rather than moving a shared one,
   and leave `pluginRegistryMu` where it is.
3. Leave `drives_unix.go`, `drives_windows.go` and `drive_menu_options.go`
   untouched — they reference the identifiers, which are still in the same package.
4. Do **not** move `drive_menu_options*.go` or `drive_bookmarks*.go`: those are
   menu UI over the registry and belong to `internal/panel`, which will import
   `internal/sysinfo` (layer 3 → layer 0, allowed).

### Required Interfaces and Contracts

```go
type DriveEntry struct {
    Name    string
    Factory func() vfs.VFS
}

var DriveRegistry []DriveEntry

func RegisterDrive(name string, factory func() vfs.VFS)
```

- Semantics unchanged, including `RegisterDrive`'s replace-in-place behaviour for
  an existing name.
- The only import the new file needs is `sync` and `github.com/unxed/f4/vfs`. If it
  needs anything else, something was moved that should not have been.

### Error Handling and Logging

None. No new failure modes.

### Tests

No new tests. The drive-menu tests must pass unchanged:

```
go test ./cmd/f4 -run '^TestDrive'
```

### Acceptance Criteria

- `cmd/f4/drive_registry.go` imports only `sync` and `vfs`.
- `grep -n 'DriveEntry\|DriveRegistry\|RegisterDrive' cmd/f4/panels_frame.go`
  returns only *uses*, no declarations.
- The suite matches the Task 1 baseline.

### Verification

- `CGO_ENABLED=0 go build ./...` and `GOOS=windows GOARCH=amd64 go build ./...`
- Expected result: exit 0 for both — `drives_windows.go` is the other consumer.

---

## Task 7: Remove sysinfo's last localization call

### Intent

`internal/sysinfo` is named in the dependency rules as a leaf that imports no
other `internal/*` package, and it is the first wave to leave `cmd/f4`. The
sysinfo family is clean apart from a single call: `gpu_info_linux.go:113` builds
`Model: Msg("InfoPanel.GPUWSLVirt")`. Left as is, `internal/sysinfo` would import
`internal/i18n` and stop being a leaf — and, since the config group leaves in Task
24, sysinfo could not go first at all.

### Implementation Steps

1. In `cmd/f4/gpu_info_linux.go:113`, return the message *key*
   (`"InfoPanel.GPUWSLVirt"`) instead of the localized string. Mark it so the
   renderer can tell a key from a vendor-supplied model name — the cleanest way
   with no new type is a sibling field, e.g. `ModelKey string`, left empty for real
   model names.
2. In the GPU rendering site, localize at render time: if `ModelKey` is non-empty,
   display `Msg(ModelKey)`, otherwise display `Model`. Find the site with
   `npx -y @colbymchenry/codegraph@1.6.0 callers gpuInfo` (or the accessor the file
   exposes) — it is expected to be `cmd/f4/info_panel.go`, which lands in
   `internal/panel` and may legally import `internal/i18n`.
3. Re-run the leaf check over the whole family and confirm it comes back empty:
   ```
   grep -lE '\b(Msg|AppConfig|showToast)\b' \
     cmd/f4/cpu_info*.go cmd/f4/mem_info*.go cmd/f4/fs_info*.go \
     cmd/f4/gpu_info*.go cmd/f4/drives_*.go cmd/f4/drive_registry.go
   ```

### Required Interfaces and Contracts

- The GPU info struct gains one field; no existing field changes meaning.
- Invariant: nothing under the sysinfo family calls `Msg`, reads `AppConfig`, or
  calls `showToast`. This is what makes its outbound edge count zero.
- The user-visible string is unchanged — same key, same catalogue, localized one
  layer up.

### Error Handling and Logging

A missing catalogue key already renders as `{Key}` via `Msg`'s existing fallback
(`Action.DisplayLabel` relies on the same behaviour). Do not add a new fallback.

### Tests

- Existing GPU info tests pass unchanged.
- Add one case to the info-panel test asserting that the WSL virtual-GPU row
  renders the localized string, not the raw key — this is the regression the move
  could introduce.

```
go test ./cmd/f4 -run '^TestGPU|^TestInfoPanel'
```

### Acceptance Criteria

- The `grep` in step 3 returns nothing.
- The WSL row renders the same text as before the change.

### Verification

- `GOOS=linux GOARCH=amd64 go build ./...`
- Expected result: exit 0 — `gpu_info_linux.go` only compiles on linux.
- `go test ./cmd/f4 -run '^TestGPU|^TestInfoPanel'`
- Expected result: `ok`.

---

## Task 8: Add the module boundary auditor

### Intent

Four of the dependency rules are mechanically checkable. Written now, the test is
green against today's tree and turns red the first time a wave introduces a
violation. Added after the migration it would only confirm what already happened.

### Implementation Steps

1. Create `cmd/f4/architecture_test.go`, standard library only. Obtain the import
   graph by shelling out to the toolchain — no new dependency, no
   `golang.org/x/tools/go/packages`:
   ```go
   out, err := exec.Command("go", "list", "-f",
       "{{.ImportPath}} {{join .Imports \" \"}}", "./...").Output()
   ```
   Run it with the module root as the working directory (`..`/`..` from
   `cmd/f4`, or resolve via `runtime.Caller` the way `plugring_test.go:40` does).
2. Assert rule 1 — the public contract stays public: no package whose import path
   is under `<module>/sdk/` or `<module>/vfs/` imports anything containing
   `/internal/`.
3. Assert rule 2 — nothing imports the entry point: no package imports
   `<module>/cmd/f4`.
4. Assert rule 3 — no upward imports into the application: no package other than
   `<module>/cmd/f4` imports `<module>/internal/app`.
5. Assert rule 4 — the module's own import graph is acyclic. Depth-first search
   over the graph restricted to `<module>/…` paths; report the cycle as a path,
   not as a boolean.
6. Encode the layer table as one ordered `map[string]int` at the top of the file,
   e.g. `{"internal/config": 0, "internal/sysinfo": 0, … "internal/app": 4}`.
   Rules 3 and any future layer assertion read from it. Each wave updates one line.
   **Seed it with the `internal/*` packages that already exist**, or the map is
   incomplete from the first commit and Task 41's "every package in the layer map
   appears in the document, and vice versa" can never pass — `ARCHITECTURE.md:285-286`
   already places all of them at layer 0:
   ```go
   "internal/netproxy": 0,   // 22 importers today
   "internal/ttyx":     0,   // 9;  internal/terminal imports it (Task 30)
   "internal/wincon":   0,   // 3;  internal/terminal imports it (Task 30)
   ```
   `internal/hideconsole` is **not** in the map: it is a vendored fork with its own
   `go.mod` (`replace` at `go.mod:187`), so `go list ./...` never returns it. Say so
   in a comment beside the map, or the next reader adds it and the test goes red.
7. Skip the whole test when the `go` binary is unavailable
   (`t.Skip("go toolchain not available")`) so a restricted sandbox does not turn
   this into a spurious failure.

### Required Interfaces and Contracts

- Test name prefix `TestArchitecture` so `-run '^TestArchitecture'` selects the
  whole group.
- One subtest per rule, named for the rule, so a failure message names the rule
  that broke.
- Failure output lists every offending edge as `importer -> imported`, not just
  the first.
- The layer map is the single place a package name appears. Adding a package to
  the map is the only edit a wave makes to this file.

### Error Handling and Logging

`go list` failing is a test failure with the command's stderr attached, not a
skip — a silent skip would disable the auditor for the rest of the migration. The
only skip is the missing-toolchain case in step 7.

### Tests

This task *is* a test. It must be green on today's tree — that is its acceptance
criterion. Verify it can actually fail by temporarily adding an import of
`internal/wincon` to a `vfs` file, confirming rule 1 goes red, then reverting.

### Acceptance Criteria

- `go test ./cmd/f4 -run '^TestArchitecture'` passes on the current tree.
- Deliberately introducing each of the four violations makes the matching subtest
  fail with a message naming the offending edge.
- The file adds no module dependency: `go.mod` is unchanged.

### Verification

- `go test ./cmd/f4 -run '^TestArchitecture' -v`
- Expected result: four subtests, all pass.
- `git diff --stat go.mod go.sum`
- Expected result: empty.

---

## Task 9: Give the shared frame harness a home

### Intent

`swapFrameManager` is used by 63 test files and `setupMockPanelsFrame` by 28. Left
in `cmd/f4`, every wave would strand its own tests. But they cannot share one
package: `setupMockPanelsFrame` (`panels_frame_test.go:794`) calls
`NewTerminalView` (term), `NewCommandLine` (cmdline), `NewFileSystemPanel` and
`PanelsFrame.initPTY` (panel), and constructs `PanelsFrame` — so a package holding
it imports three layer-3 packages, and the in-package tests of those same three
packages cannot import it back without an import cycle.

Split by dependency depth, and do it now, while everything is still one package:
the resulting diff touches only `_test.go` files and can be reviewed as one change
of call shape.

### Implementation Steps

1. Create `internal/testutil`. Move from
   `cmd/f4/frame_manager_test_helpers_test.go` into it, exported:
   - `SwapFrameManager(t *testing.T, drains ...func(*testing.T)) func()` — was
     `swapFrameManager` (`:129`)
   - `SetFrameManagerScreens` — was `setFrameManagerScreensForTest` (`:36`)
   - `AppendFrameManagerScreen` — was `appendFrameManagerScreenForTest` (`:52`)
   - `CloseFrameManagerFrames`, `CloseFrameManagerScreens` — were
     `closeFrameManagerFrames` (`:27`), `closeFrameManagerScreens` (`:14`)
   - `TaskPumpGoroutineProfile` — was `taskPumpGoroutineProfile` (`:66`). Not
     optional: `TestFrameManagerShutdownStopsTaskPumpAndUnblocksPostTask` is built
     on it and moves to `internal/testutil` in this same task, and Task 5's
     idempotence test uses the same technique.
   - `PumpUntilToastActive`, `WaitForToastExpiry`
   - `ScreenRow` — was `misc.go:12`; its five callers are all `_test.go`
     (`image_gallery_test.go`, `file_panel_test.go`, `image_view_overlay_test.go`,
     `editor_find_all_test.go`, `ai_chat_panel_test.go`), so it is test scaffolding
     that happens to live in production code today.

   After these ten helpers leave, `cmd/f4/frame_manager_test_helpers_test.go`
   retains only `waitForDirectoryLoads` (`:83`), which stays until Task 34.

   Task 43's helper table sends five more files' worth of scaffolding here in this
   same task, because their users scatter across seven packages: `pressKey` and
   the process-wide `TestMain` from `test_main_test.go` (as `PressKey` and
   `Main(m *testing.M, before, after func())` — the silent screen, the temporary
   config directory and the task-pump leak check are Main's own; `before` sets
   the calling package's seams, `after` runs its teardown, so `cmd/f4` keeps a
   five-line `TestMain` of its own), the nine bounded-conversion helpers of
   `numeric_conversions_test.go` (it holds no test), `skipIfNoRelevantChanges`
   from `test_cache_helper_test.go`, `moduleRootDir` from `module_root_test.go`,
   the `raceEnabled` pair `race_enabled_test.go` / `race_disabled_test.go` (build
   tags verbatim), plus `drainPendingTasks` (`editor_target_line_test.go`) and
   `drainUITasks` (`managed_execution_test.go`). `preserveActionRegistry` does
   **not** come here: it copies the registry maps and becomes `action.Snapshot()`
   in Task 21.
2. Break the two production drains out of `SwapFrameManager` and make them
   parameters. Today it calls `waitForAsyncClipboard` (`clipboard_async.go:27`,
   production, lands in `internal/terminal`) and `waitForDirectoryLoads`
   (`frame_manager_test_helpers_test.go:83`, which waits on the
   `directoryLoadWorkers` production global that lands in `internal/panel`).
   Reaching down into two layer-1/3 packages is exactly the cycle this task
   exists to avoid. New shape:
   ```go
   // SwapFrameManager replaces the global vtui.FrameManager with a fresh
   // instance and returns a restore function. Each drain runs before the swap
   // and before the restore: a caller passes the waits for whichever background
   // workers its own package leaves running.
   func SwapFrameManager(t *testing.T, drains ...func(*testing.T)) func()
   ```
3. `internal/testutil` must import only `testing`, `time`, `runtime/pprof`,
   `bytes`, `strings`, `github.com/unxed/vtui` and
   `github.com/unxed/vtui/vreactive`. If it needs an `internal/*` import,
   something was moved that should have stayed a drain.
4. Bind the drains once, in a package-level wrapper, and leave the 159
   `swapFrameManager` call sites alone:

   ```go
   func swapFrameManager(t *testing.T) func() {
       return testutil.SwapFrameManager(t, drainAsyncClipboard, waitForDirectoryLoads)
   }
   ```

   The drains belong to the package, not to the individual test — that is what
   "a caller passes the waits for whichever background workers its own package
   leaves running" means, and a wrapper is where a package states it once.
   Spelling the same two drains out 159 times would also bury the extraction in
   a diff no reviewer can read as a move. Every later wave does the same: its
   own wrapper, its own drains. `pressKey` gets the same treatment for the same
   reason — it needs `MacroMgr.Filter`, which `internal/testutil` cannot reach,
   so `testutil.PressKey` takes the filter and `cmd/f4` supplies it in a wrapper
   over its 178 call sites.

   Keep `waitForDirectoryLoads` in
   `cmd/f4/frame_manager_test_helpers_test.go` for now; it travels to
   `internal/panel`'s test files in Task 34. Two files mention `swapFrameManager`
   in prose only (`config.go:1204`, `queue_manager.go:138`); update the comments,
   they are not call sites.
5. Create `internal/paneltest` with a `doc.go` only. Document its contract: it
   will hold `SetupMockPanelsFrame`, it may import `internal/panel`,
   `internal/cmdline` and `internal/terminal`, and any test *inside* those three
   packages that uses it must be an external test package (`package panel_test`).
   It is filled in Task 34, when those packages exist.
6. Leave `setupMockPanelsFrame` in `cmd/f4/panels_frame_test.go` unchanged for
   now. Moving it before `panel` exists would only move the problem.

### Required Interfaces and Contracts

- Everything moved is exported; `internal/testutil` is a normal package, and Go
  will not let a `_test.go`-only package be imported.
- `SwapFrameManager` semantics are unchanged apart from the drains: it must still
  replace `vtui.FrameManager`, swap `vreactive.GlobalUpdateQueue` and
  `vreactive.GlobalAnimationManager`, and its restore function must close the
  fresh manager's frames, call `Shutdown`, and restore all three globals.
- Drain ordering is part of the contract: drains run *before* the swap and *before*
  the restore, in the order given. `swapFrameManager`'s current comments explain
  why (a directory-load worker still reading the old manager is what the race
  detector reports against whichever test does the replacing) — carry those
  comments across verbatim.
- `internal/testutil` and `internal/paneltest` are test scaffolding that ships in
  the module. That is the price of 91 cross-package call sites; note it in the
  package doc so nobody imports them from production code. Task 8's auditor can
  grow a fifth rule for this later, but not in this task.

### Error Handling and Logging

Failure modes are unchanged: the 30-second timeout in `waitForDirectoryLoads` is
still a `t.Fatal`, and toast pumps still `t.Fatal` on timeout. Keep the existing
messages verbatim — they are what a failing CI job shows.

### Tests

The moved helpers are themselves exercised by the 63 + 28 tests that use them, so
the acceptance test is the suite. In addition, keep
`TestFrameManagerShutdownStopsTaskPumpAndUnblocksPostTask` — it asserts the
harness does not leak task pumps and is the one test that tests the harness
itself. It moves with the helpers, into `internal/testutil`.

### Acceptance Criteria

- `internal/testutil` imports no `internal/*` package.
- `grep -rn 'func swapFrameManager\|func setFrameManagerScreensForTest\|func appendFrameManagerScreenForTest\|func taskPumpGoroutineProfile' cmd/f4/`
  returns nothing.
- `cmd/f4/frame_manager_test_helpers_test.go` declares only what binds this
  package to the shared harness: `waitForDirectoryLoads`, the
  `drainAsyncClipboard` adapter, and the `swapFrameManager` wrapper. No
  mechanism remains — every line of it is now in `internal/testutil`.
- `ls cmd/f4/numeric_conversions_test.go cmd/f4/test_cache_helper_test.go cmd/f4/module_root_test.go cmd/f4/race_enabled_test.go cmd/f4/race_disabled_test.go`
  all fail; `cmd/f4/test_main_test.go` declares `TestMain` (a wrapper around
  `testutil.Main`), the `before`/`after` pair it hands to it, the `pressKey`
  wrapper that supplies `MacroMgr.Filter`, and `preserveActionRegistry`, which
  waits for Task 21. Nothing else: everything there is this package's own seams,
  which is exactly what `testutil.Main` takes as arguments rather than knowing.
- `internal/paneltest` exists with `doc.go` and no other file.
- The full suite matches the Task 1 baseline.

### Verification

- `go list -f '{{join .Imports "\n"}}' ./internal/testutil | grep internal/`
- Expected result: no output.
- `go test -timeout 25m ./...`
- Expected result: identical to the Task 1 baseline.
- `go test -race -shuffle=on -timeout 5m ./cmd/f4 -run '^TestFrameManager'`
- Expected result: `ok` — the harness is the thing most likely to regress under
  the detector.

---

## Task 43: Assign every `cmd/f4` file to a wave

### Intent

The wave tasks name the files they own. Measured on the current tree with the
CodeGraph index, three sets of files are named by nobody, and all three fall
through to `internal/app` — which is how it becomes the flat package Phase 10
lists as its own top risk.

- **23 non-test sources** no wave claims: 21 that appear in no task at all, plus
  `process_environment_shell.go` and `plugring.go`, which are mentioned (a
  `showToast` call site in Task 20, a URL edit in Task 15) but never given a
  package. Two more are named only where nothing is assigned:
  `navigation_mode.go` (Task 4 empties it) and `terminal_redraw.go` (an evidence
  table).
- **154 of the 346 `_test.go` files** (152 of 343 once Phase 1 has run: five
  helper-only files are deleted and three tests are created) have no same-named
  source — they are named
  for the scenario they exercise. Wave-procedure step 3 says "take every
  `_test.go` neighbour", which is a filename rule, so it strands 45% of the suite:
  those tests stay in `cmd/f4` while their subjects leave, and either stop
  compiling or — worse — keep passing against nothing.
- **48 test helpers and `TestMain`** are called from test files that land in
  other packages. Task 9 moves the `swapFrameManager` family and Task 34 the mock
  frame; the other 46 had no plan, and `test_main_test.go`,
  `numeric_conversions_test.go`, `test_cache_helper_test.go` and
  `module_root_test.go` are helper-only files no wave named.

This task records the assignment. It writes no Go code and produces no commit of
its own; its output is the tables below, which the waves execute. Every number
comes from the graph's edges out of each test file, not from filenames.

### Implementation Steps

1. Re-derive the inputs on the current revision, so the task stays self-checking:
   ```
   # sources named by no task — must return nothing (the six glob families are
   # listed by name in step 2)
   comm -23 <(ls cmd/f4/*.go | grep -v '_test\.go$' | sed 's|cmd/f4/||' | sort) \
            <(grep -rhoE '[a-z0-9_]+\.go' .ai-factory/plans/feature-restructure-into-internal-packages/*.md | sort -u)
   # tests named nowhere in the bundle — must return nothing
   comm -23 <(ls cmd/f4/*_test.go | sed 's|cmd/f4/||' | sort) \
            <(grep -rhoE '[a-z0-9_]+_test\.go' .ai-factory/plans/feature-restructure-into-internal-packages/*.md | sort -u)
   # the size of the table in step 3 — a property of the tree, not a check
   comm -23 <(ls cmd/f4/*_test.go | sed 's|cmd/f4/||;s|_test\.go$||' | sort) \
            <(ls cmd/f4/*.go | grep -v '_test\.go$' | sed 's|cmd/f4/||;s|\.go$||' | sort) | wc -l   # 154 before Phase 1, 152 after
   ```
   The third command compares the tree with itself and never reads the bundle;
   it reports 154 whatever the tables say. Only the first two are checks.

   **Both checks stop returning nothing once the waves start, and that is not a
   miss.** A wave creates files the bundle could not have named — the seam files
   the composition root fills in, and the tests written for them. Run before
   Task 36 the first check returned ten (`keymap_suspend.go`, `media_app.go`,
   `panels_app_commands.go`, `plughost_app.go`, `process_environment_host.go`,
   `settings_save.go`, `term_app.go` and its two build-tagged halves,
   `viewer_app.go`) and the second twenty-four, and every one of the
   thirty-four is absent from the branch point. So the question the check
   answers is not "is this list empty" but "is anything on it older than the
   branch":

   ```
   base=$(git merge-base HEAD upstream/main)
   for f in <the names the two commands printed>; do
       git cat-file -e "$base:cmd/f4/$f" 2>/dev/null && echo "predates the branch: $f"
   done
   ```

   Nothing printed means no wave missed a file. Something printed is a Task 43
   miss and belongs in the wave that should have named it.

2. Sources. Each row still needs its gate score confirmed by the wave that moves
   it:

   | Files | Wave |
   |---|---|
   | `ttyx_probe.go`, `ttyx_probe_parse.go`, `ttyx_probe_unix.go`, `ttyx_probe_windows.go`, `ttyx_session.go` | Task 30 — `internal/terminal`; they decide what the terminal supports |
   | `terminal_log_console_other.go`, `terminal_log_console_windows.go`, `terminal_log_vfs.go` | Task 30 — `internal/terminal` |
   | `console_host_windows.go`, `console_overlay_other.go`, `console_overlay_windows.go` | Task 30 — `internal/terminal`; the overlays score only on `TerminalView`, the wave's own type |
   | `process_environment.go` (gate 4), `process_environment_shell.go`, `process_environment_runtime_unix.go`, `process_environment_runtime_windows.go` | Task 30 — `internal/terminal`. `pty_interface.go` calls into `process_environment_shell.go` five times and `panels_frame.go` twenty-five; term is the lowest package that can hold them without inverting a layer. The four `PanelsFrame` references in `process_environment.go` stay with the panel per the gate rule |
   | `terminal_redraw.go` | Task 30 — `internal/terminal`; gate 0, called only from `panels_frame.go`, a legal panel → term edge |
   | `plugring.go`, `plugring_meta.go`, `plugring_ui.go` | Task 26 — `internal/plughost`. Task 15 only edits `plugring.go`'s catalogue URL; it never assigns it a package |
   | `compare_folders_ui.go` (gate 4, `Msg` ×28) | Task 25 — `internal/dialog`, beside the other settings dialogs |
   | `colorer_downloader.go` | Task 33 — `internal/editor`, with `colorer_plugin.go` |
   | `action_menu.go`, `external_ui.go` | Task 25 — `internal/dialog`: `BuildMenuBarItems` and the external-UI command runner are dialog code |
   | `info_usage.go` | Task 34 — `internal/panel` |
   | `navigation_mode.go` | deleted in Task 4: its type, constants and `ParsePanelNavigationMode` all move into `config.go` |
| `action_order.go` | Task 21 — `internal/action`; created by Task 3, it is the presentation order the registry mechanism reads, so it travels with the mechanism and not with the table |
   | `command_palette.go`, `command_palette_drives.go`, `command_palette_frames.go`, `command_palette_help.go`, `command_palette_macros.go`, `command_palette_modal.go`, `command_palette_panels.go`, `command_palette_prefixes.go`, `command_palette_search.go`, `command_palette_workspace.go` | Task 25 — `internal/dialog`; the ten `command_palette*.go` files named nowhere else (the other four are in Tasks 24 and 25) |
   | `drive_bookmarks.go`, `drive_menu_options_unix.go`, `drive_menu_options_windows.go`, `file_associations.go`, `user_menu.go`, `user_menu_ini.go`, `user_menu_script.go`, `user_menu_subst.go` | Task 34 — `internal/panel`; the members of four glob families named nowhere else |
   | `host_input_modes.go`, `host_input_modes_other.go`, `host_input_modes_windows.go` | Task 36 — `internal/app` |

3. Tests without a same-named source, classified by the symbols they reference
   (graph edges into `cmd/f4` sources; methods whose name is not unique in
   `cmd/f4` dropped, because the index resolves those by name). The evidence
   column lists the packages referenced and how many distinct symbols each
   contributes. A test that drives an `actions.go` handler on a mock frame is
   hosted by `internal/app`: it owns the handler and is the only package allowed
   to import everything such a test touches. A test that looks actions up by
   name needs the registry the app table fills, so it is hosted there too.

   | Test | Package | Wave | Evidence / note |
   |---|---|---|---|
   | `action_copy_window_title_test.go` | `app` | Task 36 | action 3; GetAction/RunAction on the registry the app table fills: needs app linked |
   | `action_copyname_parent_test.go` | `panel` | Task 34 | panel 15, action 6, editor 2 |
   | `action_marked_clipboard_test.go` | `panel` | Task 34 | panel 9, action 6, keymap 3 |
   | `action_menu_visibility_test.go` | `app` | Task 36 | action 4, dialog 2; RegisterAction plus BuildMenuBarItems over the filled registry |
   | `action_restore_selection_test.go` | `panel` | Task 34 | panel 27, keymap 3, action 2 |
   | `action_shortcut_conflict_test.go` | `app` | Task 36 | action 1; GetOrderedActions over the registry the app table fills |
   | `ansi_parser_sync_test.go` | `term` | Task 30 | term 11 |
   | `appearance_settings_test.go` | `config` | Task 24 | config 4 |
   | `attributes_test.go` | `app` | Task 36 | panel 41, fileops 20, app 3; drives actionFileAttributes on the frame; fileops exports showAttributes*, panel exports getActivePanel; split candidate: 41 panel references |
   | `autosave_settings_test.go` | `app` | Task 36 | main 4, panel 4, config 3; mergeWorkspaceSessionSave and SaveSession live in main.go and move to app in Task 36 |
   | `background_jobs_session_test.go` | `panel` | Task 34 | term 10, panel 5; term exports FinishWith/StartOn if still unexported at Task 30; reconnect.go helpers are panel's own |
   | `bom_test.go` | `app` | Task 36 | panel 4, app 2, editor 2; drives showEditor/findOpenedEditor; editor exports cancelIndexing, panel exports loadDefaultQuickView |
   | `child_env_universal_linux_test.go` | `app` | Task 36 | no cmd/f4 symbol references; //go:build linux && (amd64 || arm64); subject child_env.go |
   | `cloudfox_real_archive_test.go` | `app` | Task 36 | app 2, theme 1, term 1; end-to-end over the whole application, drives actions.go handlers; skipped without credentials |
   | `cloudfox_real_cross_cloud_test.go` | `app` | Task 36 | theme 1, term 1, app 1, panel 1; end-to-end, as above |
   | `cloudfox_real_large_f5_test.go` | `app` | Task 36 | theme 1, term 1, app 1; end-to-end, as above |
   | `cloudfox_real_ui_test.go` | `app` | Task 36 | app 8, panel 7, editor 3, theme 2, term 1, viewer 1; end-to-end; one of the five multi-package tests, hosted whole as package app_test |
   | `codepage_issue875_sticky_test.go` | `viewer` | Task 29 | viewer 4 |
   | `codepage_issue875_test.go` | `app` | Task 36 | viewer 3, app 2, panel 1, editor 1; one of the five; foreign symbols are exported types only, hosted whole as package app_test |
   | `command_palette_coverage_test.go` | `cmd/f4` | stays | action 6, dialog 2; module-wide auditor, stays |
   | `command_palette_direct_panels_test.go` | `dialog` | Task 25 | dialog 19, panel 8, cmdline 1, term 1, action 1, app 1, i18n 1 |
   | `command_palette_dynamic_test.go` | `app` | Task 36 | dialog 44, panel 31, macro 12, cmdline 6, keymap 6, fileops 5, action 3, i18n 3,; one of the five; drives the palette end to end across nine packages; hosted in app, the last of them; see the multi-package table |
   | `command_palette_menu_test.go` | `app` | Task 36 | action 2; RunAction over the registry the app table fills |
   | `delete_trash_test.go` | `app` | Task 36 | fileops 14, app 3, panel 2, theme 1; drives actionDelete/actionDeletePermanent; fileops exports calculateDeleteStats, deletePathWithDisposition; split candidate: 14 fileops references |
   | `dialog_layouts_test.go` | `dialog` | Task 25 | panel 3, action 2, app 2, term 2, keymap 1, macro 1, i18n 1, fileops 1, viewer 1; Task 25 step 5 decision (split or defer); also see the multi-package table |
   | `dialog_reporter_test.go` | `fileops` | Task 32 | fileops 5 |
   | `editor_binary_open_test.go` | `app` | Task 36 | editor 16, app 5, panel 4; one of the five; drives showEditor/findOpenedEditor; editor exports cancelIndexing, indexIsComplete, awaitOffsetAsync, newEditorView; split candidate: 16 editor references |
   | `editor_codepage_test.go` | `editor` | Task 33 | editor 5, panel 2 |
   | `editor_delta_test.go` | `editor` | Task 33 | editor 1 |
   | `editor_duplicate_line_test.go` | `editor` | Task 33 | editor 10, action 1 |
   | `editor_features_test.go` | `editor` | Task 33 | editor 9 |
   | `editor_highlight_budget_test.go` | `editor` | Task 33 | editor 31 |
   | `editor_mmap_test.go` | `editor` | Task 33 | editor 22 |
   | `editor_move_line_test.go` | `editor` | Task 33 | editor 11, action 1 |
   | `editor_multicursor_edit_test.go` | `editor` | Task 33 | editor 19 |
   | `editor_multicursor_move_test.go` | `editor` | Task 33 | editor 14 |
   | `editor_multicursor_occurrence_test.go` | `editor` | Task 33 | editor 22, action 2 |
   | `editor_multicursor_select_test.go` | `editor` | Task 33 | editor 15 |
   | `editor_occurrence_test.go` | `editor` | Task 33 | editor 15, theme 2 |
   | `editor_restore_keys_test.go` | `editor` | Task 33 | editor 3 |
   | `editor_save_inplace_test.go` | `editor` | Task 33 | editor 9, app 2; app's async_buffer.prewarm is the single foreign symbol; the case using it splits out to app (Task 36) |
   | `editor_search_lazy_test.go` | `editor` | Task 33 | editor 8, app 4 |
   | `editor_search_zerocopy_test.go` | `editor` | Task 33 | editor 9 |
   | `editor_shiftdel_test.go` | `editor` | Task 33 | keymap 4, editor 2 |
   | `editor_target_line_test.go` | `editor` | Task 33 | editor 4, app 2 |
   | `editor_veto_test.go` | `editor` | Task 33 | editor 2 |
   | `editor_view_ads_test.go` | `editor` | Task 33 | editor 2 |
   | `editor_wrap_memory_test.go` | `editor` | Task 33 | editor 10, fileops 9 |
   | `envman_help_test.go` | `dialog` | Task 25 | dialog 1; reads help/ from disk |
   | `external_editor_test.go` | `editor` | Task 33 | app 5, config 3; configuredExternalEditorCommand moves to editor in Task 33 step 4 |
   | `extui_test.go` | `plughost` | Task 26 | plughost 15 |
   | `farcolor_test.go` | `theme` | Task 24 | theme 5 |
   | `fast_find_overlay_test.go` | `panel` | Task 34 | panel 4, theme 1, action 1 |
   | `file_associations_dispatch_test.go` | `panel` | Task 34 | panel 17, editor 3, theme 1, i18n 1 |
   | `file_mask_far2l_test.go` | `fileops` | Task 32 | fileops 9 |
   | `file_ops_coverage_test.go` | `fileops` | Task 32 | fileops 13 |
   | `file_ops_safety_test.go` | `fileops` | Task 32 | fileops 39 |
   | `file_ops_transfer_name_test.go` | `fileops` | Task 32 | fileops 13 |
   | `file_panel_sorting_regression_test.go` | `panel` | Task 34 | panel 5 |
   | `file_state_key_test.go` | `fileops` | Task 32 | fileops 15 |
   | `fkeys_hidden_panels_test.go` | `macro` | Task 28 | keymap 7, theme 1, macro 1 |
   | `folder_history_actions_test.go` | `app` | Task 36 | keymap 3, dialog 1; BuildMenuBarItems over the filled registry, NewHotkeyManager |
   | `folder_history_navigation_test.go` | `app` | Task 36 | panel 22, app 1, history 1; drives actionFoldersHistory; panel exports the folder-history suppression helpers; split candidate: 22 panel references |
   | `folder_history_panel_test.go` | `panel` | Task 34 | panel 4 |
   | `frame_manager_capture_test.go` | `cmd/f4` | stays | no cmd/f4 symbol references; module-wide auditor: walks every production file via commandPaletteParseProductionGo; stays |
   | `frame_manager_test_helpers_test.go` | `panel` | Task 34 | term 2; after Task 9 holds only waitForDirectoryLoads, which Task 34 moves into internal/paneltest |
   | `goto_test.go` | `editor` | Task 33 | editor 4, dialog 3; dialog exports parseGotoOffset, showGotoOffsetDialog |
   | `grabber_mouse_test.go` | `dialog` | Task 25 | dialog 5 |
   | `hardcoded_strings_test.go` | `cmd/f4` | stays | no cmd/f4 symbol references; module-wide auditor: hardcode.Scan(root) over the whole module, L10N CI gate; stays |
   | `help_keys_ar_test.go` | `dialog` | Task 25 | i18n 4, dialog 4, keymap 2 |
   | `help_keys_he_test.go` | `dialog` | Task 25 | i18n 4, dialog 4, keymap 2 |
   | `help_keys_ru_test.go` | `dialog` | Task 25 | i18n 6, dialog 5, keymap 3 |
   | `help_keys_test.go` | `dialog` | Task 25 | keymap 5, dialog 4; keymap exports initDefaults, Bind is exported already |
   | `help_keys_tr_test.go` | `dialog` | Task 25 | i18n 4, dialog 4, keymap 2 |
   | `help_lang_test.go` | `dialog` | Task 25 | config 1, dialog 1; reads help/*.hlf AND lang/*.lng from disk; no empty-set guard |
   | `history_hint_test.go` | `app` | Task 36 | panel 17, app 15, history 8, theme 5, i18n 3, dialog 1; Task 35 claimed it for cmdline: measurement error; zero cmdline symbols, drives actionCommandHistory/actionFoldersHistory |
   | `image_formats_test.go` | `media` | Task 31 | media 11 |
   | `image_view_orient_test.go` | `media` | Task 31 | media 32 |
   | `image_view_overlay_test.go` | `media` | Task 31 | media 6, numeric 2 |
   | `issue149_test.go` | `fileops` | Task 32 | fileops 15 |
   | `issue54_test.go` | `panel` | Task 34 | panel 2, theme 1 |
   | `issue561_test.go` | `app` | Task 36 | app 1, panel 1; drives actionPanelSettings on a NewPanelsFrame |
   | `issue631_test.go` | `app` | Task 36 | app 2, theme 1, i18n 1, panel 1; drives actionConfirmationsSettings and actionPanelSettings |
   | `issue815_test.go` | `fileops` | Task 32 | fileops 3 |
   | `issue821_test.go` | `app` | Task 36 | theme 2, app 1, panel 1, history 1; drives actionCommandHistory; history exports newHistorySearch |
   | `issue856_mouse_capture_test.go` | `panel` | Task 34 | panel 1 |
   | `issue863_terminal_test.go` | `panel` | Task 34 | panel 14, term 4, editor 2; Task 30 claimed it for term: measurement error; panels_frame.go buildPrompt/consumeLocalOutput |
   | `issue95_followup_test.go` | `panel` | Task 34 | panel 3, theme 1, editor 1 |
   | `keybar_injected_test.go` | `panel` | Task 34 | keymap 5, action 2, macro 1, panel 1 |
   | `kitty_metrics_test.go` | `term` | Task 30 | term 6 |
   | `lang_bidi_test.go` | `i18n` | Task 24 | no cmd/f4 symbol references; reads lang/*.lng AND help/*.hlf from disk |
   | `lang_consistency_test.go` | `i18n` | Task 24 | config 4, i18n 2; reads lang/*.lng and lang/coverage_baseline.txt from disk; no empty-set guard |
   | `lang_contamination_test.go` | `i18n` | Task 24 | no cmd/f4 symbol references; reads lang/*.lng AND help/*.hlf from disk |
   | `lang_fallback_priority_test.go` | `i18n` | Task 24 | i18n 3 |
   | `lang_homoglyphs_test.go` | `i18n` | Task 24 | no cmd/f4 symbol references; reads lang/*.lng, lang/homoglyph_baseline.txt AND help/*.hlf from disk |
   | `lang_scripts_test.go` | `i18n` | Task 24 | no cmd/f4 symbol references; reads lang/*.lng AND help/*.hlf from disk; no empty-set guard |
   | `language_list_test.go` | `i18n` | Task 24 | app 1 |
   | `macro_ctrlletter_test.go` | `macro` | Task 28 | macro 1 |
   | `macro_reload_test.go` | `macro` | Task 28 | macro 3, keymap 2, action 1 |
   | `managed_execution_test.go` | `panel` | Task 34 | panel 9 |
   | `manual_uac_validation_windows_test.go` | `update` | Task 23 | update 1 |
   | `module_root_test.go` | `testutil` | Task 9 | no cmd/f4 symbol references; helper only: moduleRootDir → testutil.ModuleRootDir |
   | `nested_input_mode_test.go` | `app` | Task 36 | main 1; nestedInputMode lives in main.go and is startup logic; Task 36 moves it to app, the test follows |
   | `numeric_conversions_test.go` | `testutil` | Task 9 | no cmd/f4 symbol references; helpers only (testRune, testInt16, …), zero tests; used by 16 files in 7 packages |
   | `panel_menu_test.go` | `panel` | Task 34 | panel 6, i18n 4, keymap 2 |
   | `panels_frame_drivecursor_windows_test.go` | `panel` | Task 34 | panel 2, theme 1 |
   | `panels_frame_pty_test.go` | `panel` | Task 34 | panel 8 |
   | `plugin_identity_test.go` | `plughost` | Task 26 | plughost 13 |
   | `plugring_policy_test.go` | `plughost` | Task 26 | plughost 10 |
   | `plugring_rows_test.go` | `plughost` | Task 26 | plughost 6 |
   | `portable_paths_test.go` | `config` | Task 24 | config 4 |
   | `proxy_settings_test.go` | `config` | Task 24 | config 3 |
   | `pty_cloexec_test.go` | `term` | Task 30 | term 2 |
   | `pty_pollable_test.go` | `term` | Task 30 | term 1 |
   | `pty_test.go` | `term` | Task 30 | term 1 |
   | `quick_view_provider_test.go` | `panel` | Task 34 | panel 5 |
   | `race_disabled_test.go` | `testutil` | Task 9 | no cmd/f4 symbol references; //go:build !race pair; with race_enabled_test.go |
   | `race_enabled_test.go` | `testutil` | Task 9 | no cmd/f4 symbol references; //go:build race pair; raceEnabled → testutil.RaceEnabled |
   | `rpc_lua_test.go` | `plughost` | Task 26 | plughost 2 |
   | `session_attach_payload_test.go` | `term` | Task 30 | term 3 |
   | `session_daemon_test.go` | `term` | Task 30 | term 6 |
   | `session_test.go` | `app` | Task 36 | main 2; shouldPersistGUIWindowSize, main.go startup logic; moves to app in Task 36 |
   | `shell_integration_test.go` | `panel` | Task 34 | panel 7, editor 3, theme 1; Task 30 claimed it for term: measurement error; PanelsFrame ×10, TerminalView 0 |
   | `shell_session_test.go` | `panel` | Task 34 | panel 9, editor 2, i18n 1, term 1; Task 30 and Task 35 both claimed it: measurement error; setupMockPanelsFrame ×3, panels_frame.go initPTY/isPtyBusy/beginPromptDrivenExecution |
   | `should_try_gui_test.go` | `app` | Task 36 | main 3; shouldTryGui, main.go startup logic; moves to app in Task 36 |
   | `sixel_layers_test.go` | `media` | Task 31 | media 2 |
   | `solaris_pty_alloc_test.go` | `term` | Task 30 | term 3 |
   | `solaris_pty_backend_test.go` | `term` | Task 30 | term 3 |
   | `solaris_streams_mock_linux_test.go` | `term` | Task 30 | no cmd/f4 symbol references; with solaris_streams_mock_test.go, by build tag |
   | `solaris_streams_mock_other_test.go` | `term` | Task 30 | no cmd/f4 symbol references; with solaris_streams_mock_test.go, by build tag |
   | `solaris_streams_mock_test.go` | `term` | Task 30 | no cmd/f4 symbol references; NewMockSolarisStreams fixture for solaris_streams.go |
   | `startup_dir_test.go` | `app` | Task 36 | main 6; startupDirs*, rememberStartupDirs, main.go startup logic; moves to app in Task 36 |
   | `style_combo_colors_test.go` | `theme` | Task 24 | theme 6 |
   | `style_completeness_test.go` | `theme` | Task 24 | theme 3, config 3 |
   | `style_custom_test.go` | `theme` | Task 24 | theme 7; getUserStylesDir seam |
   | `style_default_dark_test.go` | `theme` | Task 24 | theme 5 |
   | `style_overrides_test.go` | `theme` | Task 24 | theme 6 |
   | `sudo_dispatcher_args_test.go` | `app` | Task 36 | main 3; sudoDispatcherPath, sudoStartupMode, main.go startup logic; moves to app in Task 36 |
   | `terminal_mouse_offset_test.go` | `keymap` | Task 24 | keymap 5 |
   | `terminal_selection_test.go` | `term` | Task 30 | term 58, panel 3, theme 2; one of the five; panel references are exported types, hosted whole as package term_test |
   | `test_cache_helper_test.go` | `testutil` | Task 9 | no cmd/f4 symbol references; skipIfNoRelevantChanges → testutil.SkipIfNoRelevantChanges; glob patterns are CWD-relative and are rewritten per package |
   | `test_fallback_lang_test.go` | `i18n` | Task 24 | config 2 |
   | `test_main_test.go` | `testutil` | Task 9 | macro 2, keymap 1, config 1; TestMain + pressKey + preserveActionRegistry; see the helper table |
   | `unicode_input_test.go` | `app` | Task 36 | editor 5, main 4, cmdline 1; configureUnicodeInput lives in main.go and moves to app in Task 36; NewCommandLine and NewEditorView are exported constructors |
   | `updater_issue635_test.go` | `app` | Task 36 | update 2, media 1, panel 1; calls performUpdate(pf, …) with NewPanelsFrame(); Task 23's claim was a measurement error, performUpdate stays until Task 36 |
   | `updater_libc_test.go` | `update` | Task 23 | update 6 |
   | `updater_repro_lock_other_test.go` | `update` | Task 23 | no cmd/f4 symbol references; by build tag |
   | `updater_repro_lock_windows_test.go` | `update` | Task 23 | no cmd/f4 symbol references; by build tag |
   | `updater_repro_test.go` | `app` | Task 36 | update 2, panel 2; calls performUpdate and NewPanelsFrame, which stay in cmd/f4 until Task 36 (Task 23 step 1) |
   | `uri_navigation_test.go` | `panel` | Task 34 | panel 23, history 1; Task 29 claimed it for viewer: measurement error; 23 panel references, no viewer symbol |
   | `viewer_tail_test.go` | `viewer` | Task 29 | viewer 4 |
   | `win32_backend_test.go` | `app` | Task 36 | no cmd/f4 symbol references; DefaultConsoleBackend / IsWine, startup backend selection |
   | `workspace_routing_test.go` | `panel` | Task 34 | panel 9, editor 5, app 1 |
   | `zzz_pty_leak_check_test.go` | `term` | Task 30 | term 1 |

4. Roster of all 346 test files by wave. A test with a same-named source follows
   it; the rest come from step 3. This is the list every wave's Tests section
   refers to — `sysinfo`, `numeric`, `toast` and `colorer` receive no test file.

   - **`testutil`** (Task 9, 6 files): `module_root_test.go`, `numeric_conversions_test.go`, `race_disabled_test.go`, `race_enabled_test.go`, `test_cache_helper_test.go`, `test_main_test.go`
   - **`history`** (Task 20, 5 files): `command_history_paths_test.go`, `history_dialog_test.go`, `history_provider_test.go`, `menu_history_test.go`, `search_history_test.go`
   - **`action`** (Task 21, 1 files): `action_registry_test.go`
   - **`update`** (Task 23, 8 files): `manual_uac_validation_windows_test.go`, `self_exec_linux_test.go`, `self_exec_test.go`, `update_cli_test.go`, `updater_libc_test.go`, `updater_repro_lock_other_test.go`, `updater_repro_lock_windows_test.go`, `updater_test.go`
   - **`config`** (Task 24, 6 files): `appearance_settings_test.go`, `config_overlay_test.go`, `config_test.go`, `ini_test.go`, `portable_paths_test.go`, `proxy_settings_test.go`
   - **`i18n`** (Task 24, 11 files): `command_palette_i18n_test.go`, `lang_bidi_test.go`, `lang_consistency_test.go`, `lang_contamination_test.go`, `lang_fallback_priority_test.go`, `lang_homoglyphs_test.go`, `lang_packs_test.go`, `lang_scripts_test.go`, `lang_test.go`, `language_list_test.go`, `test_fallback_lang_test.go`
   - **`keymap`** (Task 24, 7 files): `hotkeys_test.go`, `input_translation_test.go`, `keymap_test.go`, `mackeys_test.go`, `terminal_mouse_offset_test.go`, `translate_kitty_test.go`, `ttyx_keys_test.go`
   - **`theme`** (Task 24, 9 files): `colors_test.go`, `colorspace_test.go`, `farcolor_test.go`, `style_combo_colors_test.go`, `style_completeness_test.go`, `style_custom_test.go`, `style_default_dark_test.go`, `style_overrides_test.go`, `style_test.go`
   - **`dialog`** (Task 25, 27 files): `action_menu_test.go`, `bookmarks_dialog_test.go`, `colorer_settings_test.go`, `command_palette_direct_frames_test.go`, `command_palette_direct_panels_test.go`, `command_palette_drives_test.go`, `command_palette_help_test.go`, `command_palette_search_test.go`, `command_palette_test.go`, `command_palette_ui_test.go`, `dialog_layouts_test.go`, `envman_help_test.go`, `file_dialog_test.go`, `find_file_test.go`, `grabber_mouse_test.go`, `grabber_test.go`, `help_keys_ar_test.go`, `help_keys_he_test.go`, `help_keys_ru_test.go`, `help_keys_test.go`, `help_keys_tr_test.go`, `help_lang_test.go`, `help_search_test.go`, `help_test.go`, `hotkeys_ui_test.go`, `portable_test.go`, `share_dialog_test.go`
   - **`plughost`** (Task 26, 21 files): `api_test.go`, `extui_test.go`, `lua_plugin_test.go`, `plughost_ffi_test.go`, `plugin_contributions_test.go`, `plugin_hotkeys_test.go`, `plugin_identity_test.go`, `plugin_permissions_test.go`, `plugin_permissions_ui_test.go`, `plugin_scaffold_test.go`, `plugring_meta_test.go`, `plugring_policy_test.go`, `plugring_rows_test.go`, `plugring_test.go`, `plugring_ui_test.go`, `rpc_commands_test.go`, `rpc_lua_test.go`, `rpc_plugin_test.go`, `rpc_vfs_test.go`, `sqlite_actions_test.go`, `wasm_plugin_test.go`
   - **`gui`** (Task 27, 6 files): `gui_backend_capability_test.go`, `gui_font_catalog_test.go`, `gui_font_test.go`, `gui_font_windows_test.go`, `gui_unix_test.go`, `window_icon_windows_test.go`
   - **`macro`** (Task 28, 7 files): `fkeys_hidden_panels_test.go`, `macro_ctrlletter_test.go`, `macro_export_test.go`, `macro_lua_test.go`, `macro_plugin_calls_test.go`, `macro_reload_test.go`, `macro_test.go`
   - **`viewer`** (Task 29, 9 files): `codepage_issue875_sticky_test.go`, `disasm_test.go`, `top_bar_test.go`, `url_links_test.go`, `viewer_backend_test.go`, `viewer_tail_test.go`, `viewer_text_test.go`, `viewer_view_test.go`, `word_nav_test.go`
   - **`term`** (Task 30, 37 files): `ansi_parser_sync_test.go`, `ansi_parser_test.go`, `background_jobs_test.go`, `clipboard_test.go`, `command_runner_test.go`, `command_runner_unix_test.go`, `command_runner_windows_test.go`, `far2l_image_test.go`, `graphics_compat_test.go`, `graphics_probe_decision_test.go`, `kitty_graphics_test.go`, `kitty_metrics_test.go`, `kitty_placements_test.go`, `process_environment_test.go`, `pty_bsd_test.go`, `pty_cloexec_test.go`, `pty_pollable_test.go`, `pty_test.go`, `pty_windows_test.go`, `session_attach_payload_test.go`, `session_daemon_test.go`, `session_unix_test.go`, `shell_mode_test.go`, `sixel_decode_test.go`, `sixel_terminal_test.go`, `solaris_pty_alloc_test.go`, `solaris_pty_backend_test.go`, `solaris_streams_mock_linux_test.go`, `solaris_streams_mock_other_test.go`, `solaris_streams_mock_test.go`, `solaris_streams_test.go`, `terminal_log_vfs_test.go`, `terminal_selection_test.go`, `terminal_view_test.go`, `terminal_workspace_test.go`, `ttyx_probe_test.go`, `zzz_pty_leak_check_test.go`
   - **`media`** (Task 31, 18 files): `audio_decode_test.go`, `image_console_stats_test.go`, `image_decode_test.go`, `image_external_test.go`, `image_formats_test.go`, `image_gallery_test.go`, `image_native_darwin_test.go`, `image_pipeline_test.go`, `image_preview_test.go`, `image_slideshow_test.go`, `image_transform_test.go`, `image_view_orient_test.go`, `image_view_overlay_test.go`, `image_view_test.go`, `image_x11_overlay_test.go`, `player_panel_test.go`, `sixel_layers_test.go`, `video_player_test.go`
   - **`fileops`** (Task 32, 19 files): `archive_index_test.go`, `atomic_file_test.go`, `attributes_dialog_windows_test.go`, `compare_folders_test.go`, `dialog_reporter_test.go`, `file_mask_far2l_test.go`, `file_mask_test.go`, `file_op_dialog_test.go`, `file_op_tracker_test.go`, `file_ops_coverage_test.go`, `file_ops_safety_test.go`, `file_ops_test.go`, `file_ops_transfer_name_test.go`, `file_state_key_test.go`, `file_state_test.go`, `issue149_test.go`, `issue815_test.go`, `path_identity_test.go`, `queue_manager_test.go`
   - **`editor`** (Task 33, 35 files): `colorer_plugin_test.go`, `editor_base64_test.go`, `editor_codepage_test.go`, `editor_delta_test.go`, `editor_duplicate_line_test.go`, `editor_fade_test.go`, `editor_features_test.go`, `editor_find_all_test.go`, `editor_highlight_budget_test.go`, `editor_index_status_test.go`, `editor_mmap_test.go`, `editor_move_line_test.go`, `editor_multicursor_edit_test.go`, `editor_multicursor_move_test.go`, `editor_multicursor_occurrence_test.go`, `editor_multicursor_select_test.go`, `editor_multicursor_test.go`, `editor_occurrence_test.go`, `editor_restore_keys_test.go`, `editor_save_as_test.go`, `editor_save_inplace_test.go`, `editor_search_lazy_test.go`, `editor_search_remote_test.go`, `editor_search_zerocopy_test.go`, `editor_shiftdel_test.go`, `editor_target_line_test.go`, `editor_veto_test.go`, `editor_view_ads_test.go`, `editor_view_test.go`, `editor_wrap_memory_test.go`, `editor_wrap_safety_test.go`, `external_editor_process_unix_test.go`, `external_editor_test.go`, `goto_test.go`, `mapped_file_test.go`
   - **`panel`** (Task 34, 48 files): `action_copyname_parent_test.go`, `action_marked_clipboard_test.go`, `action_restore_selection_test.go`, `background_jobs_session_test.go`, `bookmarks_test.go`, `console_passthrough_test.go`, `dragdrop_test.go`, `drive_bookmarks_test.go`, `drive_menu_options_test.go`, `fast_find_overlay_test.go`, `file_associations_dispatch_test.go`, `file_associations_test.go`, `file_panel_sorting_regression_test.go`, `file_panel_test.go`, `folder_history_panel_test.go`, `frame_manager_test_helpers_test.go`, `info_panel_test.go`, `issue54_test.go`, `issue856_mouse_capture_test.go`, `issue863_terminal_test.go`, `issue95_followup_test.go`, `keybar_injected_test.go`, `managed_execution_test.go`, `navigation_mode_test.go`, `panel_actions_test.go`, `panel_menu_test.go`, `panel_plugins_test.go`, `panels_frame_drivecursor_windows_test.go`, `panels_frame_pty_test.go`, `panels_frame_test.go`, `path_hints_test.go`, `quick_view_panel_test.go`, `quick_view_provider_test.go`, `reconnect_test.go`, `semantic_test.go`, `shell_integration_test.go`, `shell_session_test.go`, `sort_groups_test.go`, `temp_panel_test.go`, `text_editor_bridge_test.go`, `translator_test.go`, `uri_navigation_test.go`, `user_menu_ini_test.go`, `user_menu_subst_test.go`, `user_menu_ui_test.go`, `viewer_editor_history_test.go`, `workspace_routing_test.go`, `workspace_session_test.go`
   - **`cmdline`** (Task 35, 11 files): `apply_command_batch_test.go`, `apply_command_resources_test.go`, `apply_command_subst_test.go`, `apply_command_test.go`, `cmd_session_test.go`, `command_line_test.go`, `command_prefix_registry_test.go`, `command_quotes_test.go`, `command_quoting_test.go`, `resolve_command_windows_test.go`, `simple_exec_test.go`
   - **`app`** (Task 36, 52 files): `action_copy_window_title_test.go`, `action_menu_visibility_test.go`, `action_shortcut_conflict_test.go`, `actions_test.go`, `ai_chat_panel_test.go`, `arkanoid_test.go`, `async_buffer_test.go`, `attributes_test.go`, `autosave_settings_test.go`, `bom_test.go`, `child_env_test.go`, `child_env_universal_linux_test.go`, `cloudfox_real_archive_test.go`, `cloudfox_real_cross_cloud_test.go`, `cloudfox_real_large_f5_test.go`, `cloudfox_real_ui_test.go`, `codepage_issue875_test.go`, `command_palette_dynamic_test.go`, `command_palette_menu_test.go`, `debug_log_test.go`, `delete_trash_test.go`, `editor_binary_open_test.go`, `external_tools_test.go`, `farmenu_file_test.go`, `folder_history_actions_test.go`, `folder_history_navigation_test.go`, `framework_actions_test.go`, `highlight_files_test.go`, `history_hint_test.go`, `host_input_modes_test.go`, `issue561_test.go`, `issue631_test.go`, `issue821_test.go`, `libc_default_test.go`, `libc_musl_test.go`, `nested_input_mode_test.go`, `nested_input_windows_test.go`, `pe_subsystem_test.go`, `session_test.go`, `sheet_actions_test.go`, `sheet_frame_test.go`, `should_try_gui_test.go`, `startup_backend_test.go`, `startup_dir_test.go`, `static_direct_actions_test.go`, `sudo_dispatcher_args_test.go`, `title_test.go`, `unicode_input_test.go`, `updater_issue635_test.go`, `updater_repro_test.go`, `vtvibe_host_test.go`, `win32_backend_test.go`
   - **`cmd/f4`** (stays in cmd/f4, 3 files): `command_palette_coverage_test.go`, `frame_manager_capture_test.go`, `hardcoded_strings_test.go`

5. Tests whose **unexported** references span more than one future package.
   Such a test cannot live in any single package as it stands: an in-package test
   reaches its own package's unexported symbols but cannot import a package that
   imports it, and an external test package (`package X_test`) can import
   anything but sees only exported names. The rule applied: the host is the
   package whose wave comes last among the referenced ones; every unexported
   symbol it needs from a lower wave is exported by that wave when it moves; a
   symbol from a *later* wave means the cases using it split out to that
   package's tests. "Unexported" is Go's rule, a lower-case initial. 61 tests —
   16 from step 3 and 45 with a same-named source:

   | Test | Host (wave) | Exported first by a lower wave | Cases that split out to a later wave | Form in the host |
   |---|---|---|---|---|
   | `action_marked_clipboard_test.go` | `panel` (Task 34) | keymap (Task 24): initDefaults |  | in-package |
   | `action_restore_selection_test.go` | `panel` (Task 34) | keymap (Task 24): initDefaults |  | in-package |
   | `attributes_test.go` | `app` (Task 36) | fileops (Task 32): showAttributesUnix, showAttributesWindows, showAttributesWindowsForTargets, showAttributesWindowsWithProperties; panel (Task 34): getActivePanel |  | in-package |
   | `autosave_settings_test.go` | `app` (Task 36) | panel (Task 34): panelSessionState; main.go → app (Task 36): mergeWorkspaceSessionSave |  | `package app_test` |
   | `bom_test.go` | `app` (Task 36) | editor (Task 33): cancelIndexing; panel (Task 34): loadDefaultQuickView |  | in-package |
   | `command_palette_dynamic_test.go` | `app` (Task 36) | sysinfo (Task 22): getPlatformDrives; dialog (Task 25): commandPaletteAIChatFocusedEntries, commandPaletteBookmarkEntries, commandPaletteDriveEntries, commandPaletteEntry, commandPaletteImageEntries, commandPaletteLuaMacroEntries, commandPaletteMacroEntries, commandPalettePanelsContextEntries, commandPalettePrefixEntries, commandPaletteQueueEntries, executeCommandPaletteEntry, rankCommandPaletteEntries; plughost (Task 26): coreAPI; macro (Task 28): waitIdle; media (Task 31): imageGallery |  | in-package |
   | `delete_trash_test.go` | `app` (Task 36) | fileops (Task 32): calculateDeleteStats, deletePathWithDisposition |  | in-package |
   | `dialog_layouts_test.go` | `dialog` (Task 25) | term (Task 30, later, the file moves then): waitForAsyncClipboard; fileops (Task 32, later, the file moves then): reportMount; app (Task 36, later, the file moves then): showEditor, showViewer |  | `package dialog_test` |
   | `editor_binary_open_test.go` | `app` (Task 36) | editor (Task 33): awaitOffsetAsync, cancelColorer, indexIsComplete, newEditorView; panel (Task 34): stopLoadingAnimation |  | in-package |
   | `editor_save_inplace_test.go` | `editor` (Task 33) |  | app (Task 36): prewarm | in-package |
   | `folder_history_navigation_test.go` | `app` (Task 36) | panel (Task 34): consumeFolderHistorySuppression, folderHistoryStep, folderHistorySuppression, moveFolderHistory, sameFolderHistoryPath, suppressNextFolderHistory |  | in-package |
   | `goto_test.go` | `editor` (Task 33) | dialog (Task 25): parseGotoOffset, showGotoOffsetDialog |  | in-package |
   | `history_hint_test.go` | `app` (Task 36) | history (Task 20): loadFolderHistoryRecords, rememberCommandHistoryPath, selectedSecondary; panel (Task 34): getActivePanel |  | in-package |
   | `issue821_test.go` | `app` (Task 36) | history (Task 20): newHistorySearch |  | in-package |
   | `issue863_terminal_test.go` | `panel` (Task 34) | term (Task 30): cellsText |  | in-package |
   | `shell_session_test.go` | `panel` (Task 34) | term (Task 30): cellsText |  | in-package |
   | `action_menu_test.go` | `dialog` (Task 25) | action (Task 21): plainLabel; plughost (Task 26, later, the file moves then): coreAPI |  | `package dialog_test` |
   | `actions_test.go` | `app` (Task 36) | panel (Task 34): cacheKey, dirCacheEntry; main.go → app (Task 36): getSessionIniPath |  | in-package |
   | `apply_command_test.go` | `cmdline` (Task 35) | plughost (Task 26): findPanelsFrame; panel (Task 34): captureSelectionToken, clearSelectionIfUnchanged, getActivePanel |  | in-package |
   | `colors_test.go` | `theme` (Task 24) |  | dialog (Task 25): memoryHelpVFS | in-package |
   | `command_palette_direct_frames_test.go` | `dialog` (Task 25) |  | media (Task 31): imageGallery | in-package |
   | `command_palette_help_test.go` | `dialog` (Task 25) |  | app (Task 36): actionContextHelp | in-package |
   | `command_palette_i18n_test.go` | `i18n` (Task 24) | action (Task 21): plainLabel | dialog (Task 25): commandPaletteActionEntries, commandPaletteEntry, normalizeCommandPaletteText, rankCommandPaletteEntries | in-package |
   | `command_palette_test.go` | `dialog` (Task 25) |  | plughost (Task 26): coreAPI | in-package |
   | `command_prefix_registry_test.go` | `cmdline` (Task 35) | plughost (Task 26): coreAPI |  | in-package |
   | `compare_folders_test.go` | `fileops` (Task 32) | dialog (Task 25): loadCompareOptions |  | in-package |
   | `config_test.go` | `config` (Task 24) |  | media (Task 31): imageDecoderPriorityOf | in-package |
   | `console_passthrough_test.go` | `panel` (Task 34) |  | app (Task 36): terminalChildEnv | in-package |
   | `disasm_test.go` | `viewer` (Task 29) |  | editor (Task 33): editorStatusText | in-package |
   | `drive_bookmarks_test.go` | `panel` (Task 34) | fileops (Task 32): writeFileAtomically |  | in-package |
   | `editor_view_test.go` | `editor` (Task 33) |  | app (Task 36): actionWorkspaceClose, actionWorkspaceCloseNumber | in-package |
   | `file_dialog_test.go` | `dialog` (Task 25) |  | app (Task 36): actionCopyMove | in-package |
   | `file_panel_test.go` | `panel` (Task 34) | sysinfo (Task 22): fsInfo; dialog (Task 25): rowText |  | in-package |
   | `framework_actions_test.go` | `app` (Task 36) | dialog (Task 25): commandPaletteWorkspaceEntries, executeCommandPaletteEntry, generateKeysHelpTopic, rankCommandPaletteEntries |  | in-package |
   | `gui_font_catalog_test.go` | `gui` (Task 27) |  | app (Task 36): actionAppearanceSettings | in-package |
   | `highlight_files_test.go` | `app` (Task 36) | theme (Task 24): deltaE2000, loadStylesFromFS, rgbToLAB, toRGBF; panel (Task 34): fileEntry |  | in-package |
   | `host_input_modes_test.go` | `app` (Task 36) | panel (Task 34): enterHostConsole, leaveHostConsole |  | in-package |
   | `image_x11_overlay_test.go` | `media` (Task 31) | term (Task 30): answerComplete, hostGridRect, hostPixelsFromIoctl, hostScale, hostTextSize, parseXTWinOps |  | in-package |
   | `info_panel_test.go` | `panel` (Task 34) | sysinfo (Task 22): fsInfo, memInfo |  | in-package |
   | `lang_packs_test.go` | `i18n` (Task 24) | action (Task 21): plainLabel | dialog (Task 25): dialogButtonRows; panel (Task 34): assocEditorState, editAt; app (Task 36): checkboxColumnWidth, elementWidth | in-package |
   | `lang_test.go` | `i18n` (Task 24) | main.go → app (Task 36): formatVersionSHA |  | in-package |
   | `macro_plugin_calls_test.go` | `macro` (Task 28) | plughost (Task 26): coreAPI |  | in-package |
   | `macro_test.go` | `macro` (Task 28) | keymap (Task 24): initDefaults |  | in-package |
   | `panel_plugins_test.go` | `panel` (Task 34) | plughost (Task 26): coreAPI, pluginSessionRegistrations, registerRPCPluginPanels |  | in-package |
   | `panels_frame_test.go` | `panel` (Task 34) | dialog (Task 25): actionViewerSettings, associatedFileCommand, systemFileManagerCommand; term (Task 30): newTerminalRedrawScheduler; fileops (Task 32): globalAwareReporter | app (Task 36): actionAppearanceSettings, actionCopyMove, actionDelete, actionEditorSettings, actionFindFile, actionMkDir, actionPanelSettings | in-package |
   | `path_identity_test.go` | `fileops` (Task 32) |  | panel (Task 34): sameFolderHistoryPath | in-package |
   | `plugin_hotkeys_test.go` | `plughost` (Task 26) | dialog (Task 25): buildHotkeyRows |  | in-package |
   | `plugring_test.go` | `plughost` (Task 26) | config (Task 24): resetConfigDirForTest |  | in-package |
   | `portable_test.go` | `dialog` (Task 25) | config (Task 24): resetConfigDirForTest, resolveProfileDir |  | in-package |
   | `pty_windows_test.go` | `term` (Task 30) |  | panel (Task 34): getActivePTY, isPtyBusy; app (Task 36): actionExecute | in-package |
   | `queue_manager_test.go` | `fileops` (Task 32) | dialog (Task 25): themedForeground | cmdline (Task 35): cancelOperationsForShutdown | in-package |
   | `search_history_test.go` | `history` (Task 20) |  | editor (Task 33): showReplaceDialog; panel (Task 34): applyPathHintSettings; app (Task 36): actionFindFile, actionMkDir, actionViewerSearchDirection | in-package |
   | `semantic_test.go` | `panel` (Task 34) | editor (Task 33): getLineLength |  | in-package |
   | `sheet_actions_test.go` | `app` (Task 36) | action (Task 21): plainLabel |  | in-package |
   | `sqlite_actions_test.go` | `plughost` (Task 26) | action (Task 21): plainLabel |  | in-package |
   | `static_direct_actions_test.go` | `app` (Task 36) | action (Task 21): plainLabel; dialog (Task 25): commandPaletteActionEntries; panel (Task 34): buildMenuItems, leftMenu, rightMenu, updateMenuCheckmarks |  | `package app_test` |
   | `text_editor_bridge_test.go` | `panel` (Task 34) | editor (Task 33): saveUndo |  | in-package |
   | `title_test.go` | `app` (Task 36) | update (Task 23): getCurrentVersion |  | in-package |
   | `updater_test.go` | `update` (Task 23) |  | plughost (Task 26): coreAPI | in-package |
   | `url_links_test.go` | `viewer` (Task 29) |  | editor (Task 33): fillCellsWithLinks | in-package |
   | `viewer_editor_history_test.go` | `panel` (Task 34) | history (Task 20): displayText, processKey, selectedSecondary |  | in-package |

   The exports this implies, per wave. The wave that moves a symbol exports it
   in that commit, so every test still compiles in `cmd/f4` until its own wave:

   - `history` (Task 20): `displayText`, `loadFolderHistoryRecords`, `newHistorySearch`, `processKey`, `rememberCommandHistoryPath`, `selectedSecondary`
   - `action` (Task 21): `plainLabel`
   - `sysinfo` (Task 22): `fsInfo`, `getPlatformDrives`, `memInfo`
   - `update` (Task 23): `getCurrentVersion`
   - `keymap` (Task 24): `initDefaults`
   - `theme` (Task 24): `deltaE2000`, `loadStylesFromFS`, `rgbToLAB`, `toRGBF`
   - `config` (Task 24): `resetConfigDirForTest`, `resolveProfileDir`
   - `dialog` (Task 25): `actionViewerSettings`, `associatedFileCommand`, `buildHotkeyRows`, `commandPaletteAIChatFocusedEntries`, `commandPaletteActionEntries`, `commandPaletteBookmarkEntries`, `commandPaletteDriveEntries`, `commandPaletteEntry`, `commandPaletteImageEntries`, `commandPaletteLuaMacroEntries`, `commandPaletteMacroEntries`, `commandPalettePanelsContextEntries`, `commandPalettePrefixEntries`, `commandPaletteQueueEntries`, `commandPaletteWorkspaceEntries`, `executeCommandPaletteEntry`, `generateKeysHelpTopic`, `loadCompareOptions`, `parseGotoOffset`, `rankCommandPaletteEntries`, `rowText`, `showGotoOffsetDialog`, `systemFileManagerCommand`, `themedForeground`
   - `plughost` (Task 26): `coreAPI`, `findPanelsFrame`, `pluginSessionRegistrations`, `registerRPCPluginPanels`
   - `macro` (Task 28): `waitIdle`
   - `term` (Task 30): `answerComplete`, `cellsText`, `hostGridRect`, `hostPixelsFromIoctl`, `hostScale`, `hostTextSize`, `newTerminalRedrawScheduler`, `parseXTWinOps`, `waitForAsyncClipboard`
   - `media` (Task 31): `imageGallery`
   - `fileops` (Task 32): `calculateDeleteStats`, `deletePathWithDisposition`, `globalAwareReporter`, `reportMount`, `showAttributesUnix`, `showAttributesWindows`, `showAttributesWindowsForTargets`, `showAttributesWindowsWithProperties`, `writeFileAtomically`
   - `editor` (Task 33): `awaitOffsetAsync`, `cancelColorer`, `cancelIndexing`, `getLineLength`, `indexIsComplete`, `newEditorView`, `saveUndo`
   - `panel` (Task 34): `buildMenuItems`, `cacheKey`, `captureSelectionToken`, `clearSelectionIfUnchanged`, `consumeFolderHistorySuppression`, `dirCacheEntry`, `enterHostConsole`, `fileEntry`, `folderHistoryStep`, `folderHistorySuppression`, `getActivePanel`, `leaveHostConsole`, `leftMenu`, `loadDefaultQuickView`, `moveFolderHistory`, `panelSessionState`, `rightMenu`, `sameFolderHistoryPath`, `stopLoadingAnimation`, `suppressNextFolderHistory`, `updateMenuCheckmarks`
   - `app` (Task 36): `formatVersionSHA`, `getSessionIniPath`, `mergeWorkspaceSessionSave`, `showEditor`, `showViewer`

   `coreAPI` appears in nine rows and stays unexported per Task 26's contract;
   those tests obtain one through the constructor the host exposes for `cmd/f4`
   rather than instantiating the struct. Where the host is `internal/app` and
   step 3 says "split candidate", a split is the better engineering answer and
   the implementer may take it; the table guarantees only that the whole-file
   route compiles.

   Of the five tests the previous draft named, three —
   `cloudfox_real_ui_test.go`, `codepage_issue875_test.go`,
   `terminal_selection_test.go` — reference only exported types across packages
   and travel whole as external test packages; `command_palette_dynamic_test.go`
   and `editor_binary_open_test.go` are in the table. The graph found what the
   eight-type gate could not: an `app` share (`showEditor`, `findOpenedEditor`,
   `actionCopyMove`) in four of the five.

   The inverse hazard — one test covering several sources that land in
   different packages — is resolved by the rosters: both examples the previous
   draft gave, `ttyx_probe_test.go` over `ttyx_probe_parse.go` and
   `process_environment_test.go` over `process_environment_shell.go`, land in
   `internal/terminal` together with every source they cover. The remaining cases are
   the split-out column above.

6. Tests that read a resource directory from disk by a CWD-relative path. No
   symbol graph sees these, and `filepath.Glob` returns an empty set without an
   error, so after the move such a test passes green over zero files — in a
   codebase where the suite is the review mechanism, the worst defect this plan
   can introduce:

   | Test | Reads | Host | Required change |
   |---|---|---|---|
   | `lang_bidi_test.go` | `lang/*.lng`, `help/*.hlf` | `i18n` (Task 24) | help path through `testutil.ModuleRootDir` — `cmd/f4/help` in Task 24, `internal/dialog/help` from Task 25 on; guard both sets |
   | `lang_contamination_test.go` | `lang/*.lng`, `help/*.hlf` | `i18n` (Task 24) | same |
   | `lang_homoglyphs_test.go` | `lang/*.lng`, `lang/homoglyph_baseline.txt`, `help/*.hlf` | `i18n` (Task 24) | same |
   | `lang_scripts_test.go` | `lang/*.lng`, `help/*.hlf` | `i18n` (Task 24) | same; **has no empty-set guard today** |
   | `lang_consistency_test.go` | `lang/*.lng`, `lang/coverage_baseline.txt` | `i18n` (Task 24) | guard the set; **no guard today** |
   | `help_lang_test.go` | `help/*.hlf`, `lang/*.lng` | `dialog` (Task 25) | lang path through `testutil.ModuleRootDir` to `internal/i18n/lang`; guard both sets; **no guard today** |
   | `envman_help_test.go` | `help/` | `dialog` (Task 25) | moves with the directory |
   | `help_test.go` | `help/*.hlf` | `dialog` (Task 25) | moves with `help.go` and the directory |

   The guard is one line per set:
   `if len(paths) == 0 { t.Fatalf("no .lng files under %s", dir) }`.
   `skipIfNoRelevantChanges` takes the same CWD-relative patterns
   (`dialog_layouts_test.go`, `lang_contamination_test.go` and four more); every
   caller rewrites its patterns for its new directory.

7. Test scaffolding. Helpers declared in `_test.go` files and called from test
   files that land elsewhere — 48 by the graph. The rule: a helper that needs
   only `vtui` and the standard library goes to `internal/testutil` (Task 9); one
   that needs a panel type goes to `internal/paneltest` (Task 34); one that needs
   unexported symbols of its own package stays there, and the foreign user is
   rewritten against the exported surface or — under twenty lines — takes a
   copy. The ones with four or more users:

   | Helper | Declared in | Users (packages) | Home |
   |---|---|---|---|
   | `swapFrameManager` and family | `frame_manager_test_helpers_test.go` | 62 (17) | `testutil`, Task 9 (already) |
   | `setupMockPanelsFrame` | `panels_frame_test.go` | 27 (6) | `paneltest`, Task 34 (already) |
   | `pressKey` | `test_main_test.go` | 21 (7) | `testutil.PressKey`, Task 9 |
   | `waitForLoad` | `file_panel_test.go` | 19 (8) | `paneltest.WaitForLoad`, Task 34; it reads `fp.isLoading`, so `FileSystemPanel` gains an exported `IsLoading()` accessor in that wave |
   | `testRune`, `testInt16`, `testUint32` and six more | `numeric_conversions_test.go` | 16 (7) | `testutil`, Task 9, capitalised; the file holds no test |
   | `drainPendingTasks` | `editor_target_line_test.go` | 13 (3) | `testutil.DrainPendingTasks`, Task 9 |
   | `mockPty` | `ansi_parser_test.go` | 11 (2) | `paneltest.MockPty`, Task 34, local copy in term (already) |
   | `setFrameManagerScreensForTest` | `frame_manager_test_helpers_test.go` | 8 (6) | `testutil`, Task 9 (already) |
   | `skipIfNoRelevantChanges` | `test_cache_helper_test.go` | 6 (3) | `testutil.SkipIfNoRelevantChanges`, Task 9 |
   | `waitForToastExpiry` | `frame_manager_test_helpers_test.go` | 6 (4) | `testutil`, Task 9 (already) |
   | `preserveActionRegistry` | `test_main_test.go` | 5 (4) | `action.Snapshot() (restore func())`, Task 21 — it copies the registry maps, so it lives with them |
   | `stubHistoryProvider` | `history_hint_test.go` | 5 (2) | seven lines: duplicated in the two panel users |
   | `drainUITasks` | `managed_execution_test.go` | 4 (3) | `testutil.DrainUITasks`, Task 9 |
   | `setupPortableIni` | `portable_paths_test.go` | 4 (3) | stays with its file in `config` (Task 24); it stubs the `osExecutable` seam, so the three foreign users take a twenty-line copy |
   | `newFakeMacroHost` | `macro_lua_test.go` | 4 (2) | stays in `macro`; the one dialog user takes a copy |
   | `closeFrameManagerFrames` | `frame_manager_test_helpers_test.go` | 4 (4) | `testutil`, Task 9 (already) |
   | `moduleRootDir` | `module_root_test.go` | 3 (3) | `testutil.ModuleRootDir`, Task 9; the file holds no test |
   | `raceEnabled` | `race_enabled_test.go`, `race_disabled_test.go` | 1 | `testutil.RaceEnabled`, Task 9, build tags verbatim |

   `TestMain` (`test_main_test.go`) is process-wide setup for `package main`: a
   silent `vtui` screen, a temporary config directory (`XDG_CONFIG_HOME`,
   `APPDATA`, `userConfigDir`, `resetConfigDirForTest`), `vfs.InitSudoClient`,
   seven muted seams, `fusefs.UnmountAll` and a task-pump leak check. It becomes
   `testutil.Main(m *testing.M, before, after func())` in Task 9: the screen, the
   config directory and the leak check are Main's own; `before` sets the calling
   package's seams and `after` runs its teardown (`fusefs.UnmountAll` stays in
   the callers that mount, so `testutil` keeps importing no `internal/*`). Every
   package that receives tests declares a five-line `TestMain` calling it,
   `cmd/f4` included. The seams it mutes become exported package variables by
   the wave that moves them: `dialog.DefaultExternalUICommandRunner`
   (`external_ui.go`, Task 25), `fileops.DefaultNativePropertiesOpener` and
   `fileops.QueueShowToast` (`attributes_dialog.go`, `queue_manager.go`,
   Task 32), `panel.SpawnLocalShellPTY` (`panels_frame.go`, Task 34),
   `config.UserConfigDir` and `config.ResetConfigDirForTest` (Task 24);
   `toast.DurationOverride` already is (Task 20).

8. Module-wide auditors. Besides `command_palette_coverage_test.go` and Task 8's
   `architecture_test.go`, two more tests walk the whole module and can live only
   in `cmd/f4`: `frame_manager_capture_test.go` (parses every production file,
   through the palette auditor's `commandPaletteParseProductionGo`, for
   background work that captures `vtui.FrameManager`) and
   `hardcoded_strings_test.go` (`hardcode.Scan` over the module root against
   `tools/hardcoded_baseline.txt`, the L10N CI gate). Four auditors stay; the
   Definition of Done and Task 37 say so.

9. No wave's Tests section says "every remaining", "every neighbour" or names an
   unlisted "the … tests": each names its roster or points at step 4. Task 36
   step 4 does not end in "and whatever else remains".

### Required Interfaces and Contracts

- Every one of the 345 non-test files and 346 test files is assigned to exactly
  one wave, or explicitly marked "stays in `cmd/f4`" with the reason. There is
  no residual category.
- A test with a same-named source follows it. A test without one is assigned by
  the symbols it references, never by its filename.
- A test in step 5 moves in its host's wave; the symbols it needs are exported
  by the waves that move them, in those commits, so it compiles in `cmd/f4` in
  between.
- The assignment is data for the waves; this task moves nothing.

### Error Handling and Logging

Not applicable — no product code changes.

### Tests

No new tests. Step 1's first two `comm` invocations are the check, and both must
come back empty against this bundle.

### Acceptance Criteria

- The first two `comm` commands in step 1 return nothing.
- Each of the 61 tests in step 5 has a host, a form and an export list.
- Each of the eight tests in step 6 has a host and a guard requirement.
- No Tests section in Phases 5-10 contains "every remaining", "every neighbour"
  or an unnamed "the … tests"; Task 36 step 4 does not end in a catch-all.

### Verification

- The first two `comm` commands from step 1.
- Expected result: no output from either.
- `ls cmd/f4/*_test.go | wc -l`
- Expected result: `346`, the number of files in step 4's roster.

---

## Phase Risks and Mitigations

- **Risk:** the work runs long and upstream accumulates commits in files that have
  already moved, turning each one into a manual conflict inside a stranger's
  change.
  **Mitigation:** Task 0 syncs once, before anything moves, and forbids touching
  upstream again until the PR. The waves should run close together: the cost of
  this risk grows with the length of the window, and upstream gained a commit in a
  first-wave file within a single day of this plan being written.
- **Risk:** Task 3's golden order is captured after an accidental change, freezing
  a wrong menu order forever.
  **Mitigation:** capture the golden slice from a build of the unmodified tree
  (`git stash` if needed) and record the revision it came from in the test file.
- **Risk:** Task 9's drain refactor changes when workers are joined and surfaces a
  latent race in 63 tests at once.
  **Mitigation:** run the race shard before and after
  (`go test -race -shuffle=on ./cmd/f4`), and keep the drain ordering comments
  verbatim so the reason for each wait survives.
- **Risk:** Task 7's `ModelKey` field is set but never read, silently blanking the
  WSL GPU row.
  **Mitigation:** the added info-panel assertion in Task 7's Tests section fails if
  the row renders a key or an empty string.
- **Risk:** Task 5 starts the queue worker after the first enqueue, and a startup
  operation waits a full fallback cycle.
  **Mitigation:** locate the earliest `GlobalQueueManager` use with the graph
  before choosing the call site, as step 3 requires.

## Phase Completion Checklist

- Every Task 0-9 and Task 43 satisfies its acceptance criteria.
- The branch is level with `upstream/main` and a backup branch exists.
- `CGO_ENABLED=0 go build ./...`, `go vet ./...` and `go test -timeout 25m ./...`
  match `.ai-factory/RESTRUCTURE_BASELINE.md` exactly.
- `go test ./cmd/f4 -run '^TestArchitecture'` passes.
- No file has moved between directories in this phase.
- Every `cmd/f4` file has a wave, per Task 43; no residual category remains.
- `index.md` task checkboxes 0-9 and 43 are ticked.
