# Phase 5: Leaf Packages

Plan: [index.md](index.md)
Tasks: 22-24
Depends on: Phase 4

## Objective

The three lowest-outbound groups leave `cmd/f4`: `internal/sysinfo` (1 outbound
edge), `internal/update` (3), and the four-package config group
(`config`/`i18n`/`theme`/`keymap`, 15 as a group). After this phase, `config.App`
is a real package-level global in a real leaf package, and the auditor's layer
table has five more entries.

## The Wave Procedure

Every extraction task in Phases 5-9 follows these six steps. They are repeated in
each wave phase file so a task can be implemented from one file.

1. **Roster.** Start from the file families named in the task. For each candidate
   file, run the gate:
   ```
   grep -cE '\b(PanelsFrame|EditorView|ViewerView|FileSystemPanel|TerminalView|CommandLine|coreAPI|pluginPanelInstance)\b' cmd/f4/<file>.go
   ```
   - `0` → the file moves whole.
   - `>0` → open it. Either the offending function stays behind with its view, or
     — when its logic belongs to this package — it becomes a plain function taking
     the type, per the project decision on cross-package methods. Never move a file
     that names a type from a package that has not been extracted yet.
   Then run the same gate for the packages that are still inside `cmd/f4`:
   ```
   npx -y @colbymchenry/codegraph@1.6.0 callees <entry point>
   ```
   over the wave's exported entry points, and confirm no target is an
   unextracted `cmd/f4` symbol.
2. **Move by build tag, never by name.** Select per-OS files by their `//go:build`
   line. Two files in this tree lie about their platform by name:
   `pty_unix.go` is `//go:build linux`, and `solaris_pty.go` is
   `//go:build !windows` and contains no PTY code. There are 28 distinct tag
   expressions across 97 non-test files.
3. **Take the tests — by subject, not by filename.** `git mv` each `_test.go`
   file that Task 43's assignment table gives to this wave. A package extraction
   that drops coverage is not done.
   The neighbour rule alone is not enough: 154 of the 346 `_test.go` files have no
   same-named source, because they are named for the scenario they exercise
   (`bom_test.go`, `attributes_test.go`, `ansi_parser_sync_test.go`). 108 of those
   reference no view type at all, so the gate cannot classify them either — that is
   what Task 43 is for. Take its table as the roster and do not re-derive it here.
   Two hazards it records, and this wave must honour both:
   - a test whose unexported references land in **different** packages has a
     host, a form and an export list in Task 43 step 5 — it moves in its host's
     wave, never whole on a guess;
   - a test that covers **several** sources at once loses coverage of the ones that
     went elsewhere. Task 43 step 5's split-out column names these; check it
     before committing;
   - a test that reads `lang/`, `help/` or `styles/` from disk by a CWD-relative
     path follows the directory it opens first; a second directory is reached
     through `testutil.ModuleRootDir`, and both sets are guarded against coming
     back empty — `filepath.Glob` reports no error for zero matches. Task 43
     step 6 names the eight files.
4. **Rename to the package convention.** `<topic>.go` and `<topic>_<aspect>.go`,
   where the topic is the subject *inside* the package and never the package name
   — `panel/frame.go`, not `panel/panel_frame.go`. Platform suffixes compose on
   the end: `frame_dragdrop_windows.go`.
5. **Export the minimum.** `cmd/f4` still calls in, so exported names are needed;
   everything else stays unexported. Update `command_palette_coverage_test.go`'s
   file→target-package map (one line, Task 2 step 3) if this wave owns an audited
   symbol, and add the package to `architecture_test.go`'s layer map (one line).
   **How to do the rewrite.** Do not hand-edit the call sites; there are hundreds
   per wave. Drive it from the compiler, in a loop: build, read the `undefined:`
   names out of the output, export the ones the moved package declares, qualify
   them at the call sites, insert the import into any file that gained a
   reference, and repeat until the build is clean. A second loop over
   `"…" imported and not used` removes the imports the wave orphaned. Four
   hazards, each of which produces a tree that compiles into something wrong
   rather than failing loudly, so check for all four before committing:

   - **String literals.** A word-boundary rewrite of `Action` hits every
     `"Action.App.ScreenGrab"` catalogue key — 375 of them in Task 21. Skip
     anything inside quotes; the repair is to re-scan the literals afterwards
     and undo the qualifier there.
   - **Method declarations.** `func (h *host) RegisterDrive(…)` becomes
     `func (h *host) sysinfo.RegisterDrive(…)`, which is a syntax error — the
     good case, because it stops the build.
   - **Struct fields and composite-literal keys.** `hotkeyRow.Action` and
     `Action:` in a literal are names, not references. These compile as
     "invalid field name", which is also loud. Watch the ini keys among them:
     `GetString("Interface", "ColorStyle", …)` names a line in the user's
     settings file, and a qualifier there compiles and silently reads the wrong
     key.
   - **The gate itself produces false positives.** The eight-type grep counts
     names, and a name can be a string. `colors.go` scored 5 and moved whole:
     `"CommandLine"`, `"CommandLine.Prefix"` and three more are colour slot
     names in a table, not the type. A score above zero is a reason to open the
     file, not a verdict — in either direction.
   - **Locals shadowing the package.** 61 loops read `for _, action := range …`,
     and a plain assignment does the same thing while looking far more innocent:
     `macro := engine.Find(…)` turned `macro.Description` into `Description` in
     two of Task 28's test files. Rename the local inside the affected function;
     renaming the package qualifier instead is how a wave loses a call it meant
     to keep.

   - **A GOOS or GOARCH name at the end of a new filename.** Go reads
     `transport_wasm.go` as constrained to `GOARCH=wasm`: the package still
     compiles and the file is simply absent everywhere else. There are about
     forty such suffixes and half of them read as ordinary words — `_js`,
     `_ios`, `_android`, `_386`, `_mips`, `_plan9`, `_aix`. After renaming, run
     ```
     ls internal/<pkg>/*.go | grep -oE '_(android|ios|js|wasm|wasip1|plan9|aix|s390x|riscv64|loong64|mips|mipsle|ppc64|ppc64le|386)\.go$'
     ```
     and rename anything it prints. A file that already carries the matching
     `//go:build` line is fine; a file that does not has silently left the build.

   And check the round trip on anything the rewrite touched that carried data: a
   struct literal replaced by a constructor drops the fields the constructor does
   not take. Task 20 lost two seeded history fixtures that way, and only one of
   the two failed a test.

6. **Close the references and compare against the baseline.** Grep `docs/`,
   `README.md`, `AGENTS.md` and `.ai-factory/rules/base.md` for every path this
   wave changed; fix the CI and tooling lines this wave touches. A surviving
   reference is an unfinished move.
   Then run the suites and **diff the result against the recorded baseline**. Any
   test red here and green there is a regression of this commit.
   `.ai-factory/RESTRUCTURE_BASELINE.md` is an immutable snapshot of the revision
   recorded in its own header — the post-rebase HEAD from Task 0, which Task 1
   wrote into it. Never rewrite it from a fresh run, and never assume a revision
   for it: read the header. A regression written into the
   file on wave N becomes "known red" on wave N+1 and is lost for good, which is
   exactly the failure the baseline exists to prevent.
   **A cross-compile of `./...` does not build `_test.go` files.** A wave that
   strands a test on a platform the host is not — a `//go:build linux` test
   naming a constant the wave exported — leaves a package whose test binary
   does not compile there, and every local check stays green. `go vet`
   type-checks tests, so the sweep is
   ```
   for os in linux windows darwin; do GOOS=$os go vet ./...; done
   GOOS=freebsd go vet -gcflags=github.com/go-webgpu/goffi/internal/fakecgo=-std ./...
   ```
   Task 23 stranded `child_env_universal_linux_test.go` this way and four waves
   passed before anything noticed.

   **The freebsd line needs the same `-gcflags` as the cross-compile below, and
   for the same reason.** `go vet` compiles, so without it every freebsd run
   stops on `fakecgo` — a third-party file, before reaching any of ours. A check
   whose answer never changes is not a check: a real type error under freebsd
   would arrive looking exactly like that noise. The matrix's own vet cell
   already passes the flag (`build.yml:912`); this is the local gate catching
   up with it.

   Run all six modules, not only `go test ./...`: the four `tools/` modules are
   invisible to it and are touched by Task 13 (plugring) and Task 27 (icons).
   A cross-compile loop that includes freebsd or netbsd passes
   `-gcflags=github.com/go-webgpu/goffi/internal/fakecgo=-std` for those two
   targets — the flag the matrix itself passes (`build.yml:160,411`). Without it the
   build stops on `//go:cgo_export_dynamic … only allowed in cgo-generated code`,
   which looks like a breakage this plan caused and is not one. Every loop in
   Phases 3-8 carries the `case` that adds it.
   When a wave *legitimately* changes the test inventory — Task 9 converts three
   packages' tests to `package X_test`, Task 2 re-keys the palette auditor — record
   that in the commit message, not in the baseline file. The baseline answers "what
   was true before all of this", never "what was true yesterday".

## Current-Code Evidence

| Path | Signal | Consequence |
|---|---|---|
| `cmd/f4/config.go` | 1368 lines, `AppConfig` ×339, views 0, `Msg` 0 | clean layer-0 core |
| `cmd/f4/config_overlay.go`, `ini.go`, `codepage_state.go` | views 0, `Msg` 0, `AppConfig` 0 | move whole |
| `cmd/f4/portable.go` | views 1, `Msg` ×10 | one function to resolve |
| `cmd/f4/startup_settings.go` | views 1, `Msg` ×11, `AppConfig` ×6 | one function to resolve |
| `cmd/f4/codepage_settings.go` | views 1, `Msg` ×5 | one function to resolve |
| `cmd/f4/lang.go:13`, `lang_packs.go:11` | `//go:embed lang/en.lng`, `//go:embed lang/*.lng` | **both** must land in `internal/i18n` or the directory is duplicated |
| `cmd/f4/style.go:15` | `//go:embed styles/*.ini` | `styles/` moves to `internal/theme` |
| `cmd/f4/colors.go` | 461 lines, **views 5** | cannot move whole |
| `cmd/f4/colorspace.go`, `farcolor_exp.go` | views 0 | move whole |
| `cmd/f4/hotkeys.go` | views 2, `AppConfig` ×4 | two functions to resolve |
| `cmd/f4/hotkeys_ui.go` | views 1, `Msg` ×24 | UI; likely `internal/dialog`, not `keymap` |
| `cmd/f4/input_translation.go`, `translate_kitty.go`, `mackeys.go`, `ttyx_keys.go` | views 0 | move whole |
| `cmd/f4/keymap.go` | views 1 | one function to resolve |
| `cmd/f4/cpu_info*.go`, `mem_info*.go`, `fs_info*.go`, `gpu_info*.go` | 19 files, views 0, `Msg` 0 after Task 7 | move whole |
| `cmd/f4/drives_unix.go`, `drives_windows.go`, `drive_registry.go` | `DriveEntry` only, after Task 6 | move whole |
| `.github/workflows/build.yml:87` | `go run ./tools/langfmt -check cmd/f4/lang/*.lng` | moves with `lang/` |
| `.github/workflows/build.yml:198,426,600,832` | `cp -r cmd/f4/lang cmd/f4/help build/` | **`lang` half here, `help` half in Task 25** |
| `tools/langfmt/main.go:47` | default `-source cmd/f4/lang/en.lng` | moves with `lang/` |
| `cmd/f4/lang/README.md:5-6,24` | three command examples | move with the directory |
| `plugins/netfox/lang_test.go:22` | `const hostStringsPath = "../../cmd/f4/lang/en.lng"` | read at test runtime |

## Files to Change

| Path | Action | Required change |
|---|---|---|
| `internal/sysinfo/` | create | 19 info files + 2 drives files + the registry |
| `internal/update/` | create | Self-update, elevation, helper args |
| `internal/config/` | create | `F4Config`, `AppConfig`, ini parsing, overlay |
| `internal/i18n/` | create | `Msg`, language packs, `lang/` |
| `internal/theme/` | create | Colours, colour space, styles, `styles/` |
| `internal/keymap/` | create | Hotkeys, remap, input translation |
| `.github/workflows/build.yml` | modify | Five lang paths |
| `tools/langfmt/main.go` | modify | Line 47 |
| `plugins/netfox/lang_test.go` | modify | Line 22 |
| `cmd/f4/main.go` | modify | `action.Localize = i18n.Msg` |
| `cmd/f4/architecture_test.go` | modify | Five layer-map entries |

---

## Task 22: Extract `internal/sysinfo`

### Intent

One outbound edge, and Phase 1 removed it: Task 7 took the `Msg` call out of
`gpu_info_linux.go:113`, Task 6 lifted the drive registry off `panels_frame.go`,
and Task 19 gave `cpu_info_darwin.go` a private `boundedUint64ToInt`. The
dependency rules require this package to import **no** other `internal/*` package,
which is what lets any layer call it.

### Implementation Steps

1. Apply the wave procedure. Roster, all with a gate score of `0`:
   `cpu_info.go`, `cpu_info_darwin.go` (`//go:build darwin`), `cpu_info_linux.go`
   (`linux`), `cpu_info_other.go`, `cpu_info_windows.go` (`windows`);
   `mem_info.go`, `mem_info_linux.go`, `mem_info_other.go`, `mem_info_windows.go`;
   `fs_info.go`, `fs_info_darwin.go`, `fs_info_linux.go`, `fs_info_other.go`,
   `fs_info_windows.go`; `gpu_info.go`, `gpu_info_darwin.go`, `gpu_info_linux.go`,
   `gpu_info_other.go`, `gpu_info_windows.go`; `drives_unix.go`
   (`//go:build !windows`), `drives_windows.go` (`windows`); `drive_registry.go`
   (created in Task 6). Twenty-two files.
2. Rename to the topic convention: `cpu.go`, `cpu_darwin.go`, `cpu_linux.go`,
   `cpu_other.go`, `cpu_windows.go`, `mem*.go`, `fs*.go`, `gpu*.go`, `drives.go`,
   `drives_unix.go`, `drives_windows.go`. The `_info` suffix was disambiguating
   inside a flat package and is redundant inside `sysinfo`.
3. There is no `_test.go` file to move: Task 43's roster lists none for
   `internal/sysinfo` — the info family has no test in `cmd/f4` today.
4. Export the entry points `cmd/f4` still calls. Two name collisions come out of
   this, and both are the same shape: the probe function and the struct it
   returns were distinguished only by case inside the flat package. `memInfo()`
   returning `MemInfo` becomes `Mem()`, `fsInfo()` returning `FSInfo` becomes
   `FS()`, and `cpuInfo`/`gpuInfo` follow them to `CPU()`/`GPU()` for the sake of
   one rule instead of two. `sysinfo.CPU()` reads better than `sysinfo.CPUInfo()`
   anyway — the package name already says what kind of information it is.

   Watch the mechanical rewrite for method declarations: three files declare
   `func (h *someHost) RegisterDrive(…)` as part of the plugin host interface,
   and a qualifier in front of a method name is a syntax error rather than a
   wrong program, which is the good case.

   The registry's mutex stays unexported, so the tests that swapped
   `DriveRegistry` under it get `SnapshotDrives`, `SetDrives` and `Drives`
   instead — the same seam shape as `action.Snapshot`. One of them was mutating
   `DriveRegistry[0].Factory` by hand; `RegisterDrive` already replaces a factory
   in place for a name it knows, which is what a reloaded plugin does, so the
   test says that instead. Find them with
   `npx -y @colbymchenry/codegraph@1.6.0 callers` on each package-level function
   before exporting — export only what has an external caller.
5. Do **not** take `drive_menu_options*.go` or `drive_bookmarks*.go`: menu UI over
   the registry, destined for `internal/panel`, which may legally import
   `internal/sysinfo`.
6. Add `"internal/sysinfo": 0` to `architecture_test.go`'s layer map, and add the
   assertion that makes the leaf rule enforceable: **no package under
   `internal/sysinfo` imports any `internal/*` path.** This is rule 5 of the
   auditor and it is what stops a future contributor from "cleaning up" the
   private `boundedUint64ToInt`.

### Required Interfaces and Contracts

- `internal/sysinfo` imports: standard library, `golang.org/x/sys/windows`
  (three files), `github.com/ebitengine/purego` (`cpu_info_windows.go`),
  `github.com/unxed/f4/vfs` (`drives_windows.go`, `drive_registry.go`). **No
  `internal/*`.**
- `DriveEntry`, `DriveRegistry` and `RegisterDrive` keep their names and
  semantics, including replace-in-place on a duplicate name.
- The GPU struct's `ModelKey` field from Task 7 is part of this package's contract:
  a non-empty `ModelKey` means the caller localizes; `Model` is a vendor string.

### Error Handling and Logging

Unchanged. The `*_other.go` fallbacks already return empty structures rather than
errors on unsupported platforms; keep that. No logging is added — this package
returns data.

### Tests

No test file travels with this wave — the info family has none in `cmd/f4`
today and Task 43's roster lists none — so the platform matrix is the check,
not just the host:

```
go test ./internal/sysinfo/...
for t in linux/amd64 darwin/arm64 windows/amd64 freebsd/amd64 solaris/amd64 illumos/amd64; do
  extra=(); case $t in freebsd/*|netbsd/*) extra=(-gcflags=github.com/go-webgpu/goffi/internal/fakecgo=-std);; esac
  GOOS=${t%/*} GOARCH=${t#*/} CGO_ENABLED=0 go build "${extra[@]}" ./internal/sysinfo/... || echo "FAIL $t"
done
```

### Acceptance Criteria

- `go list -f '{{join .Imports "\n"}}' ./internal/sysinfo | grep 'f4/internal'`
  returns nothing.
- Twenty-two files moved; `ls cmd/f4/cpu_info*.go` fails.
- The auditor's new sysinfo-leaf assertion passes.
- The suite matches the Task 1 baseline.

### Verification

- `go test ./cmd/f4 -run '^TestArchitecture' -v`
- Expected result: five subtests including the sysinfo leaf rule, all pass.
- The cross-compile loop above prints no `FAIL`.

---

## Task 23: Extract `internal/update`

### Intent

Three outbound edges. Self-update is a leaf with a narrow surface — check,
download, elevate, restart — not an interactive subsystem, which is why the layer
table places it at layer 1 rather than 3.

### Implementation Steps

1. Apply the wave procedure. Roster: `updater.go`, `update_cli.go`,
   `update_helper_args.go`, `update_elevation_other.go` (`//go:build !windows`),
   `update_elevation_windows.go` (`windows`), `self_exec.go`,
   `self_exec_linux.go`, `self_exec_other.go`, `self_exec_termux.go`. Nine files —
   run the step-1 gate on each. Eight score `0`. **`updater.go` does not**, and it
   is the roster's largest file:
   - `func CheckForUpdates(pf *PanelsFrame, manual bool)` (`:248`)
   - `func performUpdate(pf *PanelsFrame, cand updateCandidate)` (`:375`)
   - `api := &coreAPI{}` (`:94`)

   `PanelsFrame` lands in `internal/panel` at Task 34 and `coreAPI` in
   `internal/plughost` at Task 26 — eleven and three waves later, so **this file
   does not move whole.** Resolve the three the way Task 32 step 2 resolves
   `file_ops.go`'s five: `internal/update` exposes check / download / elevate /
   restart as functions that take no view type, and the two panel-facing entry
   points stay in `cmd/f4` and travel with the composition root in Task 36. The
   `coreAPI` construction is a plugin-notification concern and stays behind with
   them.

   Four more edges the eight-type gate is silent about, all resolved in this
   wave:
   - `buildLibc` (`libc_default.go` / `libc_musl.go`) has exactly one non-test
     consumer, the asset picker. The two build-tagged files and their two tests
     come here rather than to `internal/app`, against phase 10's roster: leaving
     a constant behind whose only reader is this package would make it a seam
     set from the root for no gain.
   - `formatBuildTimeForDisplay` (`title.go`) is called by
     `nightlyDisplayVersion`, which moves. It becomes `update.FormatBuildTime`
     and `title.go` calls it — the direction the layers allow, and the comment on
     it already said it exists so the updater and F1's Help Index show one value.
   - `update_elevation_windows.go` reads `shellExecuteInfo`, `swShow`,
     `seeMaskFlagNoUI` and `procShellExecuteEx` from
     `attributes_dialog_windows.go`, which is bound for `internal/dialog` — above
     this layer. The package declares its own, the same way `internal/sysinfo`
     keeps a private `boundedUint64ToInt`. It is thirty lines of Win32 ABI, and
     the alternative is an upward import.
   - `sanitizeExtractPath` has a consumer in `colorer_downloader.go` and the two
     zip/tar extractors have one in `plugring_ui.go`, neither of them updater
     code. They went to **`internal/unpack`** rather than staying exported from
     here: three places unpack an archive f4 did not create — a release from
     GitHub, a plugin from a third-party catalogue, a colour scheme from
     somebody's host — and `SanitizePath` is the zip-slip guard all three need.
     A guard that lives inside the updater is a guard the fourth caller will not
     find. `internal/update` becomes one consumer of three; the package imports
     `vfs` and nothing else of ours, so any layer may call it.
2. `AppConfig` fields `UpdateChannel`, `UpdateInterval`, `LastUpdateCheck` and
   `LastUpdateVersion` are read here. `internal/config` does not exist yet
   (Task 24), so this wave must not read `AppConfig` directly. Pass the four values
   in as parameters from the caller in `cmd/f4`; after Task 24 the caller reads
   them from `config.App`. This is the only cross-wave coupling in this phase and
   it is one function signature.
3. Rename to the topic convention: `update.go`, `cli.go`, `helper_args.go`,
   `elevation_other.go`, `elevation_windows.go`, `selfexec.go`,
   `selfexec_linux.go`, `selfexec_other.go`, `selfexec_termux.go`.

   **Issue-numbered tests are renamed here too, and the tree already shows the
   right shape.** Three schemes coexist today: `issue54_test.go` (number only),
   `issue856_mouse_capture_test.go` (number then topic) and
   `codepage_issue875_sticky_test.go` (topic, then number, then qualifier). The
   third one is the file-naming convention this project already uses —
   `<topic>_<aspect>.go` — and it sorts the test beside its subject instead of
   into an `issue*` block at the top of the directory. Rename to it:
   `<topic>_issue<N>[_<qualifier>]_test.go`.
   The topic never has to be invented: the test function already carries it.
   `issue54_test.go` holds `TestIssue54_History`, `issue631_test.go` holds
   `TestIssue631TrashSettingIsInPanelSettings`, `issue821_test.go` holds
   `TestIssue821CommandHistoryEnterPastesSelectedEntry`. Take the topic from the
   function name, keep the number as the aspect — the issue stays traceable, and
   the file stops hiding what it covers. Leave the function names alone; renaming
   those breaks nothing but buys nothing either, and a move commit does not
   rewrite code.
4. Move the `_test.go` files whose subjects moved. Five travel whole —
   `manual_uac_validation_windows_test.go` → `elevation_manual_windows_test.go`,
   `self_exec_linux_test.go`, `self_exec_test.go`, `update_cli_test.go` →
   `cli_test.go`, `updater_libc_test.go` → `assets_test.go` (its subject is asset
   selection; the libc flavour is what it varies) — plus the two `libc_*_test.go`
   that follow their constants from step 1.

   **Not** `updater_issue635_test.go` and not `updater_repro_test.go`: both call
   `performUpdate(pf, …)` on a `NewPanelsFrame()`, and `performUpdate` is one of
   the two entry points step 1 leaves in `cmd/f4`; they travel with it in
   Task 36. An earlier draft listed `updater_issue635_test.go` here — a
   measurement error.

   **And not `updater_repro_lock_{other,windows}_test.go` either**, which the
   same draft did list. They declare nothing but `lockFileExclusively`, and its
   only caller is `updater_repro_test.go`, which stays. Moving them would leave
   the repro test undefined on both platforms.

   `updater_test.go` **splits**, because its subject does. Eight of its sixteen
   declarations follow the machinery — `ParseHelperArgs`, the build-timestamp
   comparison, the three extractors and the three `writeFileSafe` cases, plus the
   `memoryWriteSeeker` the 7z case writes through. The other eight test
   `shouldCheck`, `CheckForUpdates`, `getCurrentVersion` and `performUpdate` and
   stay with them; they reach the moved package through `update.APIURL`,
   `update.CurrentOS`, `update.CurrentArch`, `update.Release` and `update.Asset`,
   which is what those five are exported for. `title_test.go` loses one function
   the same way, to `FormatBuildTime`.
5. Add `"internal/update": 1` to the auditor's layer map.

### Required Interfaces and Contracts

```go
// The update check takes its settings rather than reading a global, so the
// package stays independent of internal/config's extraction order.
type Settings struct {
    Channel       int    // 0 = Stable, 1 = Nightly
    Interval      int    // 0 = Never, 1 = Every start, 2 = Daily, 3 = Weekly
    LastCheck     int64  // Unix timestamp
    LastVersion   string
}
```

- The four fields keep the exact semantics documented on `F4Config`
  (`config.go`), comments included. `Check` and `RunCLI` take them by value;
  `RunCLI` writes back through a `func(Settings)` the root supplies, so the
  package never names `SaveConfig`.
- A second parameter object carries what the *binary* knows about itself, which
  `internal/app` owns and this package cannot reach: `Build{Version, IsRelease,
  TimeText}`, built in `cmd/f4/updater.go` from `getCurrentVersion`,
  `isReleaseVersion` and `getVCSInfo`. It replaces the three calls
  `fetchUpdateCandidate` used to make into `title.go` and `api.go`.
- The elevation path's platform split stays file-level; no
  `runtime.GOOS ==` branching is introduced.
- `--update` remains a `cmd/f4` flag; only the implementation moves.

### Error Handling and Logging

Update failures already report to stderr with the `f4: ` prefix on the startup and
fatal paths. Keep the exact message shapes: they are the user's only signal when
the TUI is not up yet. Do not add logging.

### Tests

The eight files named in step 4 move. `updater_test.go` builds a `coreAPI`
(`api.go`), which leaves in Task 26 — that case splits out to
`internal/plughost`'s tests then (Task 43's multi-package table).

```
go test ./internal/update/...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./internal/update/...
```

The Windows build matters here: elevation is the one genuinely Windows-specific
part of this package.

### Acceptance Criteria

- Eight files moved whole, plus the two `libc_*.go`; `updater.go` moved minus
  the three view-bound references from step 1, which stay in `cmd/f4`.
- `grep -rn 'AppConfig' internal/update/` returns nothing.
- `grep -rnE '\b(PanelsFrame|coreAPI)\b' internal/update/` returns nothing — this
  is the check that step 1 was actually performed.
- `go test ./cmd/f4 -run '^TestArchitecture'` passes with `internal/update` at
  layer 1.

### Verification

- `go test ./internal/update/... ./cmd/f4/...`
- Expected result: `ok`, matching the Task 1 baseline.

---

## Task 24: Extract `internal/config`, `internal/i18n`, `internal/theme`, `internal/keymap`

### Intent

Four layer-0 leaves that reference each other's globals today and therefore have
to be separated in one commit. This is the wave that makes `config.App` real: 133
files read `AppConfig` (55 non-test), and it stays a package-level global by
design — the package imports no other `internal/*`, so it cannot create a cycle.

### Implementation Steps

1. **`internal/config`.** Move `config.go` (1368 lines, gate score 0),
   `config_overlay.go`, `ini.go`, `codepage_state.go`. Task 4 already moved the
   five `F4Config` field types into `config.go`, so the type closure is complete.
   Rename: `config.go` → `config.go`, `config_overlay.go` → `overlay.go`,
   `ini.go` → `ini.go`, `codepage_state.go` → `codepage.go`.
   `AppConfig` → `config.App` — this is the single largest rename in the plan, 133
   files. Do it with the compiler, not with grep: rename in `config.go`, build, fix
   what the compiler names.
   `portable.go` (views 1, `Msg` ×10), `startup_settings.go` (views 1, `Msg` ×11)
   and `codepage_settings.go` (views 1, `Msg` ×5) are **settings dialogs**, not
   configuration storage. Run the step-1 gate, and send them to `internal/dialog`
   (Task 25) unless the gate comes back `0` after the offending function is
   identified as trivially separable.
2. **`internal/i18n`.** Move `lang.go` and `lang_packs.go` together — they embed
   `lang/en.lng` and `lang/*.lng` respectively, and splitting them duplicates the
   directory. `git mv cmd/f4/lang internal/i18n/lang`. Also take
   `command_palette_i18n.go` if its gate score is `0`, and the three language-list
   functions from `actions.go` (`listAvailableUILanguages`,
   `listAvailableHelpLanguages`, `getLanguageName` — all verified dependency-free).
   Export `Msg`; it has 77 non-test caller files.
   Then set the hook Task 21 left: in `cmd/f4/main.go`, replace the temporary
   assignment with `action.Localize = i18n.Msg`.
3. **`internal/theme`.** Move `colorspace.go` (gate 0), `farcolor_exp.go` (gate 0),
   `style.go` (gate 0) and `git mv cmd/f4/styles internal/theme/styles`.
   `colors.go` scores **5** on the gate — it does not move whole. Open it, and for
   each of the five references decide: the colour tables and lookup move; the
   functions that colour a specific view stay with that view. Split into
   `internal/theme/colors.go` and a remainder that travels later.
4. **`internal/keymap`.** Move `input_translation.go`, `translate_kitty.go`,
   `mackeys.go`, `ttyx_keys.go` (all gate 0). `hotkeys.go` scores 2 and
   `keymap.go` scores 1 — resolve each reference per step 1 of the wave procedure.
   `hotkeys_ui.go` (views 1, `Msg` ×24) is a settings dialog: send it to
   `internal/dialog` (Task 25), not here.
   `translate_kitty.go` and `input_translation.go` call the numeric helpers, so
   `internal/keymap` imports `internal/numeric` — allowed, both are layer 0 and
   `numeric` imports nothing.
5. **Infrastructure, in this same commit:**
   - `build.yml:87` — `go run ./tools/langfmt -check cmd/f4/lang/*.lng` →
     `internal/i18n/lang/*.lng`
   - `build.yml:198`, `:426`, `:600`, `:832` — `cp -r cmd/f4/lang cmd/f4/help build/`
     → change the **`lang` half only**; the `help` half moves in Task 25. Each of
     the four lines is edited twice across the two commits, which is expected.
   - `tools/langfmt/main.go:47` — default `-source` value.
   - `cmd/f4/lang/README.md:5-6,24` — three command examples; the file moves with
     the directory.
   - `plugins/netfox/lang_test.go:22` —
     `const hostStringsPath = "../../cmd/f4/lang/en.lng"`. This is read at test
     runtime, so a stale path is a failing test, not a compile error. Update it to
     `"../../internal/i18n/lang/en.lng"`.
6. Add four layer-map entries at layer 0 to `architecture_test.go`.

### Required Interfaces and Contracts

```go
package config
var App = F4Config{ /* 132 fields, unchanged */ }

package i18n
func Msg(key string) string   // returns "{key}" for a missing key — the
                              // fallback shape action.DisplayLabel tests for
```

- `config.App` is a package-level global. That is deliberate and permitted: the
  package imports no other `internal/*`, so it cannot create a cycle. Do not
  convert it to a constructor — 133 read sites and a hard dependency rule stand
  behind it.
- `F4Config`'s field set, order, comments and ini keys are unchanged. The `.ini`
  file format is a user-facing contract; `f4.example.ini` must still describe the
  binary's behaviour exactly.
- `Msg`'s missing-key form `{Key}` is contractual — `Action.DisplayLabel`,
  `Action.DisplayDescription` and every `Msg` caller depend on it.
- `internal/theme` embeds `styles/*.ini` and `internal/i18n` embeds `lang/*.lng`
  and `lang/en.lng`; embed patterns are relative to the embedding file's directory,
  so the directories must sit directly under the package.
- `internal/keymap` → `internal/numeric` is the only intra-`internal` import in
  this wave.

### Error Handling and Logging

- Config parse failures already report to stderr with the `f4: ` prefix before the
  TUI exists. Keep the messages verbatim.
- A missing language pack already falls back to English through `Msg`; do not add
  an error path.
- No logging is added.

### Tests

- The roster (Task 43 step 4), 33 files: `internal/config` —
  `appearance_settings_test.go`, `config_overlay_test.go`, `config_test.go`,
  `ini_test.go`, `portable_paths_test.go`, `proxy_settings_test.go`;
  `internal/i18n` — `command_palette_i18n_test.go`, `lang_bidi_test.go`,
  `lang_consistency_test.go`, `lang_contamination_test.go`,
  `lang_fallback_priority_test.go`, `lang_homoglyphs_test.go`,
  `lang_packs_test.go`, `lang_scripts_test.go`, `lang_test.go`,
  `language_list_test.go`, `test_fallback_lang_test.go`; `internal/theme` —
  `colors_test.go`, `colorspace_test.go`, `farcolor_test.go`,
  `style_combo_colors_test.go`, `style_completeness_test.go`,
  `style_custom_test.go`, `style_default_dark_test.go`, `style_overrides_test.go`,
  `style_test.go`; `internal/keymap` — `hotkeys_test.go`,
  `input_translation_test.go`, `keymap_test.go`, `mackeys_test.go`,
  `terminal_mouse_offset_test.go`, `translate_kitty_test.go`, `ttyx_keys_test.go`.
- Four of the i18n tests read **both** `lang/*.lng` and `help/*.hlf` from disk by
  CWD-relative path — `lang_bidi_test.go`, `lang_contamination_test.go`,
  `lang_homoglyphs_test.go`, `lang_scripts_test.go` — and `help/` leaves for
  `internal/dialog` in Task 25. In this commit, rewrite their help globs to
  `filepath.Join(testutil.ModuleRootDir(t), "cmd", "f4", "help", "*.hlf")` (Task
  25 repoints that one path), and guard **both** sets:
  `if len(paths) == 0 { t.Fatalf("no .lng files under %s", dir) }`, the same for
  `.hlf`. `lang_scripts_test.go` and `lang_consistency_test.go` have no such
  guard today, so after the move they would pass green over zero files.
  `lang_consistency_test.go` also reads `lang/coverage_baseline.txt`, which
  moves with the directory.
- `skipIfNoRelevantChanges` patterns (`lang/*.lng`, `help/*.hlf`, the test's own
  file name) are relative to the test's directory; rewrite the `help` pattern to
  the module-root form as well.
- `plugins/netfox/lang_test.go` must be run explicitly — it is in a different
  package and reads the moved file from disk:
  ```
  go test ./plugins/netfox/...
  ```
- `TestLangConsistency` is referenced by `cmd/f4/lang/README.md:24`
  (`F4_UPDATE_COVERAGE_BASELINE=1 go test -run '^TestLangConsistency$' ./cmd/f4`).
  Update that command in the README to the new package path and confirm it still
  passes.

```
go test ./internal/config/... ./internal/i18n/... ./internal/theme/... ./internal/keymap/... ./plugins/netfox/... ./cmd/f4/...
```

### What the wave actually found

Six corrections to the task text above, all recorded rather than silently
absorbed:

1. **`ini.go` cannot live in `internal/config`.** All four leaves parse ini
   files and none may import another of ours, so the parser went to
   `internal/ini` first, in its own commit.
2. **`config.go` left 26 symbols undefined**, reaching layers 1, 3 and 4 — not
   the clean layer-0 core the evidence table promised. They were split by the
   rule `ARCHITECTURE.md` now carries: a setting's schema travels with the
   field, its use by a running application stays behind. `saveSettingsGroups`
   is the clearest case of the second half and did not move.
3. **`codepage_state.go` did not move.** It reads `GlobalFileState` from
   `file_state.go`, which Task 32 takes to `internal/fileops` at layer 1.
4. **`colors.go` moved whole.** Its gate score of 5 is five string literals —
   `"CommandLine"`, `"CommandLine.Prefix"` and three more are colour slot names
   in a table, not the type. The first place on this branch where the
   eight-type grep said no to a file that could move.
5. **`highlight_files.go` came to `internal/theme`**, against the Task 36
   roster. `style.go` pushed the theme's rules into it, which would otherwise
   need a seam; its own subject is colouring a file by a rule; and Task 43
   already recorded that its test needs four `internal/theme` symbols.
6. **`hotkeys.go` did not move.** Its `conditionRegistry` closures ask the panel
   frame what is on screen and the command palette what is true, so it belongs
   with the application. Only the Far key-name codec came out of it and out of
   `macro.go` — `EventToFarString`, `EventToHotkeyString`, `ParseFarKey` and
   their tables, which name keys for `hotkeys.ini`, `keymap.ini` and macros
   alike.

`internal/theme` and `internal/keymap` import `internal/config`, which the task
text forbade and the layer rule permits: 0 to 0 is not an upward import, and
eight settings steer the theme. `internal/i18n` does not, and that one is not a
matter of taste — `internal/config`'s own proxy test imports `plugins/netfox`,
which imports `internal/i18n`, so the edge would close a cycle.

### Acceptance Criteria

- `grep -rn 'cmd/f4/lang' . --exclude-dir=.git` returns nothing.
- `grep -rn '\bAppConfig\b' --include='*.go' . ` returns nothing (all sites now
  read `config.App`).
- `find internal/i18n/lang -name '*.lng' | wc -l` matches the pre-move count.
- `find internal/theme/styles -name '*.ini' | wc -l` matches the pre-move count.
- `plugins/netfox` tests pass.
- `go test ./internal/i18n -run 'Bidi|Contamination|Homoglyph|Scripts|Consistency' -v`
  reports every test as `--- PASS` over a non-zero file count, none as
  `--- SKIP` and none over an empty set.
- The four packages each import zero `internal/*` packages, except
  `internal/keymap` → `internal/numeric`.

### Verification

- `go test ./plugins/netfox/... -run '^TestLang' -v`
- Expected result: `--- PASS` — this is the test that a stale path turns red
  rather than uncompilable.
- `go run ./tools/langfmt -check internal/i18n/lang/*.lng`
- Expected result: exit 0.
- `go test -timeout 25m ./...`
- Expected result: identical to the Task 1 baseline.

---

## Phase Risks and Mitigations

- **Risk:** `colors.go` (gate score 5) is moved whole into `internal/theme` and
  drags five view types into a layer-0 package.
  **Mitigation:** the gate in wave-procedure step 1 is mandatory per file, and this
  file's score is recorded in the evidence table so it cannot be missed.
- **Risk:** only one of `lang.go` / `lang_packs.go` moves and `lang/` ends up
  duplicated or, worse, embedded from two directories.
  **Mitigation:** Task 24 step 2 moves them together as a single instruction; the
  file-count check in the acceptance criteria catches a duplicate.
- **Risk:** `plugins/netfox/lang_test.go` goes red only in CI, after the commit,
  because it reads the file at runtime rather than importing it.
  **Mitigation:** it is listed in the Tests section with its own `go test`
  invocation, and in the acceptance criteria.
- **Risk:** the `AppConfig` → `config.App` rename is done with `sed` and silently
  hits a comment or a string literal.
  **Mitigation:** Task 24 step 1 requires the compiler-driven method — rename the
  declaration, build, fix what the compiler names.
- **Risk:** Task 23 leaves `internal/update` reading `AppConfig` and the config
  wave then has to touch it again.
  **Mitigation:** the `Settings` struct in Task 23's contract removes the read
  before `internal/config` exists.

## Phase Completion Checklist

- Every Task 22-24 satisfies its acceptance criteria.
- Six new packages exist; `internal/sysinfo`, `internal/config`, `internal/i18n`
  and `internal/theme` import zero `internal/*` packages.
- `build.yml`, `tools/langfmt`, `cmd/f4/lang/README.md` and
  `plugins/netfox/lang_test.go` point at the new locations.
- `go test -timeout 25m ./...` matches the Task 1 baseline.
- `go test ./cmd/f4 -run '^TestArchitecture'` passes with five new layer entries.
- `index.md` task checkboxes 22-24 are ticked.
