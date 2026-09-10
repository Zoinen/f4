# Phase 4: The Shared Primitives Leave `cmd/f4`

Plan: [index.md](index.md)
Tasks: 18-21
Depends on: Phase 1 (Tasks 3, 5, 6, 7), Phase 3

## Objective

Three small layer-0 packages exist — `internal/numeric`, `internal/toast`,
`internal/history` — plus `internal/action`, and every one of them imports
nothing from `internal/`. This is the phase that makes the extraction waves
possible: 37 call edges run *into* what would otherwise be `internal/app` from
lower layers, and the dependency rules forbid exactly that. Extracting in the
other order makes every intermediate commit uncompilable.

## What the Graph Actually Says About the "Primitives"

`ARCHITECTURE.md` names "the action registry and `actions.go`, `toast`, the
panels-frame state, `path_identity`, `search_history`, `menu_history`, and
`misc.go` minus its numeric helpers" as the pile that leaves first. Re-derived
from the index, that list resolves as follows. **This is the single most important
table in the plan** — an implementer who moves the named *files* instead of the
named *symbols* produces an uncompilable commit.

| Named in the architecture | What the graph shows | Where it goes |
|---|---|---|
| `action_registry.go` — registry | `Action` (`:24`) has only `func() bool` fields; `RegisterAction` (`:121`) is clean; `RegisterAction` is called from 7 files that land in 5 different packages | **`internal/action`**, Task 21 |
| `action_registry.go` — `init()` | 2554 lines, 173 calls, `PanelsFrame` ×114, `EditorView` ×47 inside the closures; find it with `grep -n '^func init()'` | **stays**, travels with `internal/app` (Task 36) |
| `actions.go` | 81 functions: 80 free + one `*PanelsFrame` method (`:2194`). 61 reference a view type. Of the 20 that do not, only 4 are layer-0 | **splits six ways** — see the table below |
| `framework_actions.go` | 25 functions; 7 touch `PanelsFrame`/`QueueFrame`. The other 18 have **no external callers at all** — they are reached only as `Handler:` values from the registration table and from `main.go:actionScreenDump` | **stays whole**, travels with `internal/app` (Task 36) |
| "the panels-frame state" | `DriveEntry`/`DriveRegistry`/`RegisterDrive`, `panels_frame.go:26-40`, needs only `sync` + `vfs` | already lifted in Task 6; goes to **`internal/sysinfo`** (Task 22) |
| `path_identity.go` | 2 functions over `vfs`; callers are `file_ops.go` (fileops) and `panels_frame.go`/`file_panel.go` (panel) | **`internal/fileops`** (Task 32); panel → fileops is a legal layer-3 → layer-1 edge |
| `search_history.go`, `menu_history.go` | zero `Msg`, zero `AppConfig`, zero `showToast`; only `vtui`/`vtinput` | **`internal/history`**, Task 20 |
| `misc.go` minus numeric helpers | inverted: the numeric helpers have the widest fan-out (8 packages), `ScreenRow` has five callers and all are `_test.go`, `ReleaseHeavyMemory` has two | numeric → **`internal/numeric`** (Task 19); `ScreenRow` → `internal/testutil` (already, Task 9) |

`actions.go`'s 20 view-free functions, by verified dependency:

| Functions | Uses | Destination |
|---|---|---|
| `extractNames`, `decodeFar2lTime`, `unescapeFar2lString`, `importFar2lHistory` | nothing | **`internal/history`** (Task 20) |
| `confirmAndClearHistory`, `confirmAndClearRichHistory`, `confirmAndPruneMissingFolderHistory`, `showPluginFileDialog`, `elementWidth`, `checkboxColumnWidth`, `boolToCheckboxState`, `choiceText` | `Msg` ×2-4, `vtui` ×1-18 | `internal/dialog` (Task 25) |
| `listAvailableUILanguages`, `listAvailableHelpLanguages`, `getLanguageName` | nothing | `internal/i18n` (Task 24) |
| `viewerSearchOffset`, `viewerSearchMatch`, `readViewerSearchData` | `*ViewerBackend` | `internal/viewer` (Task 29) |
| `editorHeaderIsBinary` | nothing | `internal/editor` (Task 33) |
| `configuredExternalEditorCommand` | `AppConfig` ×5 | with the external-editor launch path |

So `actions.go` contributes exactly **four** functions to this phase. The other
5300-odd lines travel with their subjects.

## Current-Code Evidence

| Path | Symbols / lines | Why it matters |
|---|---|---|
| `cmd/f4/misc.go` | 11 functions, 102 lines | `bounded*` ×7, `nonNegativeUint64`, `runeCodepoint`, `ReleaseHeavyMemory`, `ScreenRow` |
| `cmd/f4/toast.go` | 20 lines, `showToast` + `toastDurationOverride` | called from 16 files spanning 8 future packages |
| `cmd/f4/history_provider.go` | 349 lines, `HistoryRecord` (`:14`), `F4HistoryProvider` (`:63`) | the history storage model |
| `cmd/f4/history_dialog.go` | 718 lines | builds its dialog directly on `vtui`; zero `Msg`, zero view types |
| `cmd/f4/command_history_paths.go` | 65 lines | `vtui` only |
| `cmd/f4/search_history.go` | 73 lines, `attachHistory` (7 caller files), `commitHistory` (6) | `vtui.Edit` glue |
| `cmd/f4/menu_history.go` | 204 lines | `vtui.VMenu`/`vtinput` glue, self-contained |
| `cmd/f4/viewer_editor_history.go` | 317 lines, `PanelsFrame` ×3, `FileSystemPanel` ×1 | **not** part of `internal/history`; travels with `internal/panel` |
| `cmd/f4/action_registry.go:24-135` | `Action`, `DisplayLabel`, `DisplayDescription`, `actionRegistry`, `actionOrder`, `RegisterAction` | the mechanism half |
| `cmd/f4/cpu_info_darwin.go:39,:47` | `boundedUint64ToInt` | sysinfo's only two numeric calls; gets a private copy |

Numeric-helper consumers after sysinfo takes its private copy — the count that
justifies the package: `extui_host.go` (plughost), `session_unix.go` (term),
`input_translation.go` and `translate_kitty.go` (keymap), `macro_lua_api.go`
(macro), `disasm.go` (viewer), `quick_view_panel.go` (panel), `editor_view.go`
(editor), `viewer_view.go` (viewer), plus `semantic.go`'s `semanticInt`. **Seven
distinct packages.**

## Files to Change

| Path | Action | Required change |
|---|---|---|
| `cmd/f4/action_registry.go` | modify | Keeps the mechanism |
| `cmd/f4/action_table.go` | create | Receives the 2554-line `init()` |
| `internal/numeric/*.go` | create | Numeric helpers, zero imports |
| `cmd/f4/misc.go` | delete | Fully distributed |
| `cmd/f4/cpu_info_darwin.go` | modify | Gains a private `boundedUint64ToInt` |
| `internal/toast/toast.go` | create | From `cmd/f4/toast.go` |
| `internal/history/*.go` | create | Five files + four functions from `actions.go` |
| `internal/action/*.go` | create | From the mechanism half of `action_registry.go` |
| ~16 files calling `showToast` | modify | `toast.Show` |
| ~10 files calling history helpers | modify | `history.*` |
| 7 files calling `RegisterAction` | modify | `action.RegisterAction` |

---

## Task 18: Separate the action registry's mechanism from its table

### Intent

`action_registry.go` is two unrelated things in one file: a 130-line registry with
no dependency above layer 0, and a 2554-line `init()` whose closures reach
`PanelsFrame` and `EditorView`. Task 21 can only move the first. Splitting it in
place, in its own commit, keeps the move commit readable as a rename.

Nothing moves between directories in this task.

### Implementation Steps

1. Create `cmd/f4/action_table.go` with `package main` and move
   the whole `func init()` of `action_registry.go` — into it, verbatim.
   Move only the imports that block actually uses; `gofmt` and the compiler will
   flag the rest.
2. `action_registry.go` keeps: the `Action` struct and its doc comment
   (`:19-93`), `DisplayLabel` (`:96`), `DisplayDescription` (`:106`),
   `var actionRegistry` (`:116`), `var actionOrder` (`:119`), `RegisterAction`
   (`:121`), the ordering machinery added in Task 3, and the lookup helpers.
3. Verify the mechanism half is clean:
   ```
   grep -cE '\b(PanelsFrame|EditorView|ViewerView|FileSystemPanel|TerminalView|CommandLine|coreAPI)\b' \
     cmd/f4/action_registry.go
   ```
   Expected `0`. If it is not, the split line is wrong — find the straggler and
   move it to `action_table.go`.
4. Leave the six other registration files (`fuse_mount_action.go` ×2,
   `fuse_mount_list.go`, `sheet_actions.go`, `sqlite_actions.go`,
   `static_direct_actions.go`, `vtvibe_host.go`) untouched. Their `init()`s stay
   with their subjects and will call `action.RegisterAction` after Task 21.
5. Do **not** split `actions.go` or `framework_actions.go` in this task. The graph
   shows `framework_actions.go` has no external callers and travels whole with
   `internal/app`; `actions.go`'s pieces travel with their destination waves,
   where the compiler can confirm each landing.

### Required Interfaces and Contracts

- Registration order is unchanged. Task 3 made it explicit precisely so this split
  cannot reorder the menu; `TestActionOrderIsStable` is the proof.
- `action_registry.go` after the split declares no identifier that names a view
  type — this is the invariant Task 21 depends on.
- File-order `init()` semantics: `action_registry.go` sorts before
  `action_table.go` alphabetically, which is the same relative order as before.
  It must not matter, because of Task 3 — but keeping it the same removes one
  variable.

### Error Handling and Logging

None. No behaviour changes.

### Tests

No new tests. The golden order test from Task 3 is the guard:

```
go test ./cmd/f4 -run '^TestActionOrder|^TestCommandPalette'
```

### Acceptance Criteria

- The `grep` in step 3 returns `0`.
- `wc -l cmd/f4/action_registry.go` is roughly 250, `cmd/f4/action_table.go`
  roughly 2560.
- `TestActionOrderIsStable` passes without touching its golden slice.

### Verification

- `go test ./cmd/f4 -run '^TestActionOrder' -v`
- Expected result: `--- PASS`, golden slice unmodified in git
  (`git diff --stat cmd/f4/action_registry_order_test.go` is empty).
- `CGO_ENABLED=0 go build ./...`
- Expected result: exit 0.

---

## Task 19: Create `internal/numeric`

### Intent

`misc.go` is an 11-function junk drawer whose members belong to seven different
future packages. The seven `bounded*` helpers plus `nonNegativeUint64` and
`runeCodepoint` are pure, dependency-free, and shared by seven packages —
duplicating them seven times would scatter the `#nosec G115` annotations that
`gosec` checks in CI. `ReleaseHeavyMemory` joins them: two consumers in two
different packages, and no better home.

### Implementation Steps

1. Create `internal/numeric/numeric.go` with `package numeric` and move, exported
   and otherwise verbatim including every comment and `#nosec` annotation:
   - `boundedInt64ToInt` → `BoundedInt64ToInt` (`misc.go:24`)
   - `boundedUint64ToInt` → `BoundedUint64ToInt` (`:32`)
   - `nonNegativeUint64` → `NonNegativeUint64` (`:40`)
   - `boundedInt16` → `BoundedInt16` (`:48`)
   - `boundedInt32` → `BoundedInt32` (`:56`)
   - `boundedUint16` → `BoundedUint16` (`:64`)
   - `boundedUint32` → `BoundedUint32` (`:72`)
   - `boundedRune` → `BoundedRune` (`:80`)
   - `runeCodepoint` → `RuneCodepoint` (`:89`)
2. Create `internal/numeric/memory.go` with `ReleaseHeavyMemory` (`:98`). It is
   the one function here that is not a conversion, and it imports
   `runtime/debug`; a separate file keeps `numeric.go` at zero imports except
   `strconv`.
3. **`internal/sysinfo` does not import this package.** Add a private
   `boundedUint64ToInt` to `cmd/f4/cpu_info_darwin.go` — five lines, the same body,
   the same `#nosec` comment — because `cpu_info_darwin.go:39` and `:47` are the only call
   sites in the sysinfo family (line 33 is `func readStaticDarwinCPU()`, not a call) and the dependency rules state that
   `internal/sysinfo` imports no other `internal/*` package. Importing the shared
   helper would *create* the forbidden edge, not remove it.
4. `misc.go:12` `ScreenRow` already moved to `internal/testutil` in Task 9. With
   steps 1-2 done, `cmd/f4/misc.go` is empty: `git rm cmd/f4/misc.go`.
5. Update every call site. Find them with
   `npx -y @colbymchenry/codegraph@1.6.0 callers <symbol>` per symbol rather than
   grepping — the names are short and grep over 109k lines matches identifiers it
   should not.

### Required Interfaces and Contracts

```go
package numeric

// Every function keeps its two-value (value, ok) shape; a false ok means the
// conversion would not have been representable on this platform's int size.
func BoundedInt64ToInt(v int64) (int, bool)
func BoundedUint64ToInt(v uint64) (int, bool)
func NonNegativeUint64(v int64) uint64      // clamps, does not report
func BoundedInt16(v int) (int16, bool)
func BoundedInt32(v int) (int32, bool)
func BoundedUint16(v int) (uint16, bool)
func BoundedUint32(v int) (uint32, bool)
func BoundedRune(v int) (rune, bool)
func RuneCodepoint(r rune) (uint, bool)
func ReleaseHeavyMemory(sizeBytes int64)    // frees OS memory above 50 MB
```

- `numeric.go` imports `strconv` and `unicode/utf8` — `BoundedRune` and
  `RuneCodepoint` reject surrogates through `utf8.ValidRune`, which a range check
  alone does not; `memory.go` imports `runtime/debug` and nothing else. Any other import means something was moved
  here that does not belong.
- The `strconv.IntSize == 32` branches are load-bearing on the exotic 32-bit
  targets (`linux/386`, `linux/mips`, `linux/mipsle`, `linux/arm`) and must be
  copied exactly.
- Every `// #nosec G115` comment travels with its function. `gosec` runs in CI and
  a dropped annotation is a new finding.

### Error Handling and Logging

No new failure modes and no logging. These are pure functions; the `bool` is the
error channel and every caller already handles it.

### Tests

`misc.go` has no dedicated test file today. Add `internal/numeric/numeric_test.go`
with table-driven cases per the project convention (`for _, tt := range`,
`t.Helper()` first in helpers, standard library only):

- boundary cases for each `Bounded*`: exact maximum, maximum+1, exact minimum,
  minimum-1, zero.
- `NonNegativeUint64`: negative clamps to `0`, positive passes through.
- `RuneCodepoint`: valid rune, negative rune.
- `ReleaseHeavyMemory`: below and above the 50 MB threshold — assert it does not
  panic; do not assert on GC behaviour.

This is the one place in the plan where a move adds tests: nine pure functions
with platform-dependent branches, moving into a new package, previously covered
only indirectly.

### Acceptance Criteria

- `ls cmd/f4/misc.go` fails.
- `go list -f '{{join .Imports "\n"}}' ./internal/numeric` lists only `strconv`,
  `unicode/utf8` and `runtime/debug`.
- `grep -n 'boundedUint64ToInt' cmd/f4/cpu_info_darwin.go` shows a local
  definition plus its two call sites at `:39` and `:47`.
- `grep -rn 'numeric\.' cmd/f4/cpu_info*.go cmd/f4/mem_info*.go cmd/f4/fs_info*.go cmd/f4/gpu_info*.go`
  returns nothing.

### Verification

- `go test ./internal/numeric/...`
- Expected result: `ok`.
- `GOOS=linux GOARCH=386 CGO_ENABLED=0 go build ./...` and
  `GOOS=linux GOARCH=mips CGO_ENABLED=0 go build ./...`
- Expected result: exit 0 — the 32-bit branches compile.
- `go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...` is not the check here;
  instead run the strict lint over the new package:
  `golangci-lint run -c .golangci-strict.yml ./internal/numeric/...`
- Expected result: no G115 findings.

---

## Task 20: Create `internal/toast` and `internal/history`

### Intent

Two subjects, two packages, both layer-0 leaves. `showToast` is called from 16
files spanning eight future packages, so it must leave before any of them. The
history cluster is a genuine subsystem — storage model, dialog, providers — and
it is verified free of `Msg`, `AppConfig`, `showToast` and every view type, so it
can leave now rather than waiting for `internal/i18n`.

They are one commit because neither is large enough to justify its own and they
have no dependency on each other.

### Implementation Steps

**`internal/toast`:**

1. `git mv cmd/f4/toast.go internal/toast/toast.go`, change the package clause to
   `package toast`, rename `showToast` → `Show` and `toastDurationOverride` →
   `DurationOverride`. The call reads `toast.Show(msg, d)` — do not name it
   `toast.ShowToast`.
2. `DurationOverride` is a test seam (`toast.go:12`: tests shorten f4-owned toast
   lifetimes without changing vtui's timer). It stays an exported package-level
   `func(time.Duration) time.Duration`, nil in production. Document that it is for
   tests only.
3. Update all 16 call sites: `action_registry.go`, `actions.go`, `ai_chat_panel.go`,
   `apply_command.go`, `editor_view.go`, `info_panel.go`, `macro_host.go`,
   `panel_plugins.go`, `panels_frame.go`, `plugring.go`,
   `process_environment_shell.go`, `queue_manager.go`, `terminal_view.go`,
   `terminal_workspace.go`, `url_links.go`, `vtvibe_ap.go`. Confirm the list is
   still exactly these with
   `npx -y @colbymchenry/codegraph@1.6.0 callers showToast`.
4. `queue_manager.go:308` wraps it in `queueShowToast` — repoint that wrapper, do
   not remove it; it is the queue's own indirection seam.

**`internal/history`:**

5. `git mv` **four** files into `internal/history/`, package `history`, renamed to
   the package's topic convention (topic inside the package, never the package
   name):
   - `history_provider.go` → `provider.go` (`HistoryRecord`, `F4HistoryProvider`)
   - `command_history_paths.go` → `paths.go`
   - `search_history.go` → `edit.go` (`attachHistory`, `attachHistoryUseLast`,
     `commitHistory`, `inputBoxEdit` — all `vtui.Edit` glue)
   - `menu_history.go` → `menu.go` (moves whole, including
     `actionSelectLastMenuItem` and `handleMenuHistoryEvent`; both are
     self-contained and reference nothing outside the file)
6. Move four functions out of `cmd/f4/actions.go` into
   `internal/history/far2l.go`: `extractNames`, `decodeFar2lTime`,
   `unescapeFar2lString`, `importFar2lHistory`. Verified: none uses `Msg`,
   `AppConfig`, `showToast` or `vtui`. `importFar2lHistory` has a test in
   `actions_test.go` — move that test case with it.
7. Do **not** move `viewer_editor_history.go` despite the name: it references
   `PanelsFrame` ×3 and `FileSystemPanel` ×1 and travels with `internal/panel`
   (Task 34).
8. Do **not** move the three `confirmAnd*History` functions from `actions.go`:
   each uses `Msg` ×2 and builds a `vtui` dialog, so they are dialog code and go
   to `internal/dialog` in Task 25.
9. Export what leaves and update the call sites — `attachHistory` has 7 caller
   files, `commitHistory` 6.

10. **`history_dialog.go` does not move here.** The gate this phase applies —
    no `Msg`, no `AppConfig`, no `showToast`, no view type — passes on it, and it
    is still not a leaf: `actions.go` and `viewer_editor_history.go` construct
    `historySearch` and assign its unexported fields (`pinSlotOf`, `onCtrlF10`,
    `onDetails`) and read `search.all`. Moving it would mean exporting that whole
    surface, which is API design and not a move. It goes to `internal/dialog` in
    Task 25, beside the three `confirmAnd*History` functions that drive it, and
    the `historyType*` / `historyShow*` constants go with it — `config.go` is
    their other consumer and both stay in `cmd/f4` until then.

11. Four symbols the file-level gate did not see, each resolved where it belongs:
    - `GetF4ConfigDir` — `NewF4HistoryProvider` takes the directory as an
      argument instead of asking the configuration for it. `NewProviderAtPath`
      is the same thing for one named file, and is what the tests use.
    - `LoadIni` — `ImportFar2lHistory` takes an already-opened reader
      (`Far2lHistoryFile`, one `GetString` method) rather than opening the file.
    - `sameFolderHistoryPath` — a `history.SamePath` seam, set by the
      composition root, exactly as `action.Localize` is. The real comparison
      normalises URIs and lives above this layer.
    - `len(BookmarkSet{})` — the pin-slot count is the history dialog's, so
      `history.PinSlots` is the constant and `BookmarkSet` is sized from it.
      One number, one place.
    And one that does not belong here at all: `actionSelectLastMenuItem` calls
    `activateMainMenuAt` in `framework_actions.go`, which is layer 4. It is an
    action rather than history machinery, so it stays in `cmd/f4` as
    `menu_history_action.go` and travels to `internal/app`.

### Required Interfaces and Contracts

```go
package toast

// Show displays a toast and returns the duration actually used.
func Show(message string, duration time.Duration) time.Duration

// DurationOverride lets tests shorten f4-owned toast lifetimes without
// changing vtui's timer. Production leaves it nil.
var DurationOverride func(time.Duration) time.Duration
```

```go
package history

type HistoryRecord struct{ /* unchanged */ }
type F4HistoryProvider struct{ /* unchanged */ }

func AttachHistory(edit *vtui.Edit, historyID string) *vtui.Edit
func AttachHistoryUseLast(edit *vtui.Edit, historyID string) *vtui.Edit
func CommitHistory(edit *vtui.Edit, value string)
func ImportFar2lHistory(path string) ([]HistoryRecord, error)
// … menu-history and dialog entry points, exported as their callers need
```

- `internal/toast` imports `time` and `github.com/unxed/vtui`. Nothing else.
- `internal/history` imports `github.com/unxed/vtui`,
  `github.com/unxed/vtinput`, `github.com/mattn/go-runewidth` and standard
  library. **No `internal/*` import.** If a moved function needs `Msg`, it does
  not belong here — send it to `internal/dialog` with the `confirmAnd*` three.
- `HistoryRecord` and `F4HistoryProvider` keep their names: Far-derived structures
  are never renamed during a move.
- Type identity matters across the boundary: `plugins/` and `sdk/` do not
  reference these types today; confirm with
  `grep -rn 'HistoryRecord' sdk/ plugins/ vfs/` before exporting, so the export
  does not accidentally widen the public contract.

### Error Handling and Logging

- `ImportFar2lHistory` already returns `(…, error)` wrapped with context per the
  project convention. Keep the message text: error strings frequently become
  dialog text and `ST1005` is disabled for that reason.
- `toast.Show` cannot fail and returns a duration, not an error. Do not add one.
- No logging is added. Toasts *are* the user-facing notification channel in this
  application.

### Tests

- `actions_test.go`'s `importFar2lHistory` cases move to
  `internal/history/far2l_test.go`.
- The five files Task 43's roster lists for `internal/history` move:
  `command_history_paths_test.go`, `history_dialog_test.go`,
  `history_provider_test.go`, `menu_history_test.go`, `search_history_test.go`.
  `search_history_test.go` also drives `actionFindFile`, `actionMkDir`,
  `showReplaceDialog` and `applyPathHintSettings`; those cases split out to the
  packages that own them (Task 43's multi-package table).
- Add nothing new: these are moves, and the existing coverage travels.

```
go test ./internal/toast/... ./internal/history/... ./cmd/f4/...
```

### Acceptance Criteria

- `go list -f '{{join .Imports "\n"}}' ./internal/toast ./internal/history | grep internal/`
  returns nothing.
- `grep -rn 'func showToast' cmd/f4/` returns nothing.
- `ls cmd/f4/history_provider.go cmd/f4/search_history.go cmd/f4/menu_history.go`
  all fail; `ls cmd/f4/history_dialog.go` still succeeds, per step 10.
- `ls cmd/f4/viewer_editor_history.go` still succeeds.
- The suite matches the Task 1 baseline.

### Verification

- `go list -f '{{join .Imports "\n"}}' ./internal/toast ./internal/history | sort -u`
- Expected result: only `time`, `vtui`, `vtinput`, `go-runewidth` and standard
  library entries.
- `go test -timeout 25m ./...`
- Expected result: identical to the Task 1 baseline.
- `go test ./cmd/f4 -run '^TestArchitecture'`
- Expected result: `ok`.

---

## Task 21: Create `internal/action`

### Intent

`RegisterAction` is called from seven files that land in five different packages
(`fuse_mount_action.go` and `fuse_mount_list.go` → panel, `sheet_actions.go` →
sheet, `sqlite_actions.go` → plughost, `static_direct_actions.go` and
`vtvibe_host.go` → app/vtvibe, `action_table.go` → app). Every one of them needs
the registry before it can move. This is the last primitive and it unblocks the
waves.

### Implementation Steps

1. `git mv cmd/f4/action_registry.go internal/action/registry.go`, package
   `action`. After Task 18 the file is the mechanism only.
2. Export the surface the callers need: `Action` and its fields are already
   exported; `RegisterAction` already is; export `actionRegistry`'s accessors and
   the ordering helpers rather than the maps themselves — a package-level mutable
   map read from five packages is the "package-level mutable state as a shortcut
   across a boundary" the anti-patterns list bans. Provide:
   `Lookup(name string) (Action, bool)`, `All() []Action` (in registered order),
   `Len() int`, and `Snapshot() (restore func())` — the test seam that replaces
   `preserveActionRegistry` (`test_main_test.go`): five test files in four
   packages copy the registry maps and restore them in `t.Cleanup`, and only
   this package can reach the maps once they are unexported (Task 43's helper
   table).
3. `DisplayLabel` and `DisplayDescription` call `Msg`, which lands in
   `internal/i18n` (Task 24) — three tasks later. Rather than reordering the waves,
   give the package a localizer hook:
   ```go
   // Localize resolves a message key to display text. The composition root sets
   // it to i18n.Msg. The default reproduces i18n's missing-key form, because
   // DisplayLabel's fallback tests for exactly that shape.
   var Localize = func(key string) string { return "{" + key + "}" }
   ```
   `DisplayLabel` keeps its current logic verbatim, calling `Localize` instead of
   `Msg`. The default must produce `{Key}` — the fallback is
   `if s := Localize(k); !strings.HasPrefix(s, "{")`, so an identity default would
   silently return raw keys as labels.
4. Wire `action.Localize = i18n.Msg` in the composition root in Task 24, when
   `internal/i18n` exists. Until then it is set in `cmd/f4/main.go` to the local
   `Msg`. Record this as a two-line follow-up in Task 24's steps, not as a TODO in
   the code.
5. Update the seven registration files and `action_table.go` to call
   `action.RegisterAction` and to construct `action.Action`.

   Two hazards in the mechanical rewrite, both of which produce a tree that
   compiles into something wrong rather than failing loudly:

   - **`Action.` appears inside string literals.** Every `LabelKey` and `DescKey`
     in the table is a catalogue key spelled `"Action.App.ScreenGrab"`. A
     word-boundary rewrite turns 375 of them into `"action.Action.…"`, the
     catalogue lookup then misses, and every menu falls back to its English
     label — which is visible only if a test asserts a localized string.
   - **`action` is a common local name.** 61 loops read `for _, action := range
     …`, and `hotkeyRow` has a field called `Action`. Both shadow the package;
     rename the locals inside the affected functions rather than the field.

6. Assign `action.Localize` in `cmd/f4`. Two places need it, not one: `SetupUI`
   for the four production entry points, and the test binary's own seams —
   `TestMain` never calls `SetupUI`, and without the assignment every action
   renders its English fallback.
7. **Split** `cmd/f4/action_registry_order_test.go` (Task 3) — do not move it
   whole. `TestActionOrderIsStable` asserts the full ordered list, which is
   produced by `action_table.go` and still lives in `cmd/f4` at this point.
   - The ordering-mechanism cases
     (`TestActionOrderIndependentOfRegistrationSequence`,
     `TestActionOrderCoversRegistry`) go to
     `internal/action/registry_order_test.go`.
   - The golden-list case and its golden slice stay in `cmd/f4`, in a file named
     **`cmd/f4/action_table_order_test.go`** — named for the table it guards, so it
     travels with `action_table.go` to `internal/app` in Task 36 step 8.
   - `cmd/f4/action_registry_order_test.go` no longer exists after this task. Every
     later task that verifies the golden slice cites
     `cmd/f4/action_table_order_test.go` (Task 32), and Task 36 step 8 moves that
     file. Task 18 runs *before* this split and correctly cites the original name.

### Required Interfaces and Contracts

```go
package action

type Action struct { /* unchanged, all fields exported already */ }

func (a Action) DisplayLabel() string
func (a Action) DisplayDescription() string

func RegisterAction(action Action)
func Lookup(name string) (Action, bool)   // name matched case-insensitively
func All() []Action                        // registration order, per Task 3
func Len() int
func Snapshot() (restore func())           // copies the registry; restore puts it back — a test seam

var Localize = func(key string) string { return "{" + key + "}" }
```

- `RegisterAction` keeps its exact semantics including case-insensitive keying on
  `strings.ToLower(action.Name)` and the ignore-duplicates behaviour.
- Ordering is Task 3's explicit order and must be identical before and after.
- `internal/action` imports `strings` and, after step 3, nothing from
  `internal/*`. It must not import `internal/i18n` — the hook exists to keep it a
  leaf.
- Setting `Localize` is composition-root work. No package below layer 4 assigns it.

### Error Handling and Logging

`RegisterAction` silently ignores a duplicate name today. Preserve that: changing
it to panic or to report would be a behaviour change inside what must remain a
mechanical move, and the duplicate case is already covered by
`TestActionOrderCoversRegistry`.

No logging. The registry is fully determined before the UI exists.

### Tests

- `TestActionOrderIndependentOfRegistrationSequence` and
  `TestActionOrderCoversRegistry` move to
  `internal/action/registry_order_test.go`.
- `TestActionOrderIsStable` (the golden list) stays in `cmd/f4`, in
  `cmd/f4/action_table_order_test.go`.
- Add `TestDisplayLabelUsesLocalizeFallback`: with the default `Localize`, a
  `LabelKey` resolves to the English `Label`, not to `{Key}` — this is the exact
  regression step 3's `{`-prefixed default prevents.

```
go test ./internal/action/... ./cmd/f4/...
```

### Acceptance Criteria

- `go list -f '{{join .Imports "\n"}}' ./internal/action | grep internal/` returns
  nothing.
- `ls cmd/f4/action_registry.go` and `ls cmd/f4/action_registry_order_test.go`
  both fail; `ls cmd/f4/action_table.go` and
  `ls cmd/f4/action_table_order_test.go` both succeed.
- `TestActionOrderIsStable` passes with its golden slice unmodified.
- `TestDisplayLabelUsesLocalizeFallback` passes.

### Verification

- `go test ./cmd/f4 -run '^TestActionOrderIsStable' -v`
- Expected result: `--- PASS`; the golden slice inside
  `cmd/f4/action_table_order_test.go` is byte-identical to the one Task 3 captured.
- `go test ./internal/action/... -v`
- Expected result: three tests pass.
- `go test -timeout 25m ./...`
- Expected result: identical to the Task 1 baseline.

---

## Phase Risks and Mitigations

- **Risk:** an implementer moves the *files* `actions.go` and
  `framework_actions.go` because the architecture names them, producing an
  import cycle or a 5000-line uncompilable package.
  **Mitigation:** the "What the Graph Actually Says" table at the top of this
  phase is the authority, and Task 18 step 5 forbids the split explicitly.
- **Risk:** `action.Localize` defaults to identity and every menu label silently
  becomes a raw key.
  **Mitigation:** the `{`-prefixed default is specified in Task 21 step 3 and
  guarded by `TestDisplayLabelUsesLocalizeFallback`.
- **Risk:** a `#nosec G115` annotation is dropped in the `misc.go` move and the
  strict lint reports nine new findings.
  **Mitigation:** Task 19's contract requires the annotations verbatim and its
  Verification runs `.golangci-strict.yml` over the new package.
- **Risk:** exporting `HistoryRecord` widens the third-party contract by accident.
  **Mitigation:** `internal/history` is under `internal/`, so Go itself forbids an
  external import; the grep in Task 20's contract confirms nothing in `sdk/`,
  `plugins/` or `vfs/` referenced it in the first place.
- **Risk:** `internal/sysinfo` picks up `internal/numeric` in a later wave because
  the private copy looks like duplication to a reviewer.
  **Mitigation:** the private copy carries a comment naming the dependency rule it
  satisfies, and Task 8's auditor grows a layer assertion for sysinfo in Task 22.

## Phase Completion Checklist

- Every Task 18-21 satisfies its acceptance criteria.
- `internal/numeric`, `internal/toast`, `internal/history` and `internal/action`
  each import zero `internal/*` packages.
- `cmd/f4/misc.go`, `cmd/f4/toast.go`, `cmd/f4/action_registry.go` and the five
  history files no longer exist; `cmd/f4/action_table.go` does.
- `go test -timeout 25m ./...` matches the Task 1 baseline.
- `go test ./cmd/f4 -run '^TestArchitecture'` passes.
- `index.md` task checkboxes 18-21 are ticked.
