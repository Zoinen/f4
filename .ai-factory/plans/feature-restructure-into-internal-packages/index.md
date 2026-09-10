<!-- aif:plan-mode:ultra -->
# Ultra Implementation Plan: Restructure f4 into internal packages

Mode: ultra
Branch: feature/restructure-into-internal-packages
Created: 2026-09-07
Base revision: `0cda22a7`, level with `upstream/main` at `ef3640c7`
(`git rev-list --left-right --count upstream/main...HEAD` reports `0` on the
left). Every count in this bundle is verified against `0cda22a7`.

## Original Request

> Задание: составить план реструктуризации репозитория f4 через /aif-plan full. Все
> архитектурные решения уже приняты и закоммичены — их не пересматривать, а превратить
> в исполнимый план.
>
> Что от тебя нужно: план с фазами и задачами, где каждая задача называет конкретные
> файлы и правки, а не «перенести подсистему». Порядок обязан быть таким, чтобы КАЖДЫЙ
> промежуточный коммит собирался на всей матрице — в частности, общие примитивы (toast,
> реестр действий, framework_actions, истории) обязаны покинуть cmd/f4 до пакетов,
> которые их зовут, иначе получится циклический импорт internal/panel ↔ internal/app.
>
> Цифры в этом сообщении — вход, а не истина: проверяй их через codegraph, прежде чем
> строить на них фазу. Где расходится — доверяй графу и скажи об этом.
>
> Следующий шаг: пересобрать его в ultra-бандл, /aif-plan ultra. Ultra здесь не
> «спланировать заново», а «раскрыть каждую фазу до уровня, на котором её выполняет
> модель послабее без догадок».

## Settings

- Testing: yes — the `cmd/f4/*_test.go` files (346 at the base revision, 343 once
  Phase 1 has moved five helper-only files out and added three tests) move with
  their subjects; a
  module boundary auditor is added in Task 8; `internal/numeric` gets the one new
  test suite in the plan.
- Logging: project convention, nothing added. Diagnostics stay on `VTUI_DEBUG`
  (`cmd/f4/debug_log.go`); user-facing failures go to stderr with the `f4: `
  prefix. A move commit that introduces a log line is not a move commit.
- Docs: yes — mandatory `/aif-docs` checkpoint (Task 40), plus the per-commit
  reference sweep required by `ARCHITECTURE.md`, plus a dedicated
  `ARCHITECTURE.md` rewrite (Task 41).

Delivery: **one pull request to `unxed/f4`** with meaningful commits inside. The
harness scaffolding (`.ai-factory/`, `.claude/`, `.mcp.json`, `AGENTS.md`) ships
in that PR deliberately, as part of the proposal.

## Architecture and Decisions

Cross-phase decisions an implementer must not re-open. Everything here was
re-derived from the CodeGraph index against the working tree; where it disagrees
with `ARCHITECTURE.md` or with the reconnaissance input, the graph won and the
disagreement is stated.

**Package names chosen during planning.** `ARCHITECTURE.md` says the shared
primitives leave first but does not name their package. These four are the plan's
own choice and the one place the user may want to overrule with a single reply:

| Package | Holds | Why this name |
|---|---|---|
| `internal/action` | `Action`, `RegisterAction`, registry order and lookup | Exact and unambiguous |
| `internal/toast` | `toast.Show` and its test-duration seam | One subject, one file; a package of one file is normal in Go |
| `internal/history` | `history_provider.go`, `history_dialog.go`, `command_history_paths.go`, `search_history.go`, `menu_history.go`, plus four far2l helpers from `actions.go` | A real cluster, verified free of `Msg`, `AppConfig`, `showToast` and every view type |
| `internal/numeric` | seven `bounded*`, `NonNegativeUint64`, `RuneCodepoint`, `ReleaseHeavyMemory` | Justified by count: **seven** consuming packages after `internal/sysinfo` takes its private copy — plughost, term, keymap, macro, viewer, panel, editor |

An earlier draft proposed one `internal/vtuix` for toast plus the histories. It
was split on review: the name rests on no established abbreviation the way
`internal/ttyx` does, and it would have held two unrelated subjects.

**The primitives are symbols, not files.** `ARCHITECTURE.md` names
`actions.go` and the action registry among the files that leave first. Measured:

- `actions.go` holds 81 functions — 80 free plus one `*PanelsFrame` method at
  `:2194`. **61 reference a view type; 52 take `*PanelsFrame` in their
  signature.** Of the 20 that do not, only four are layer-0. The rest scatter to
  `internal/dialog` (8), `internal/i18n` (3), `internal/viewer` (3),
  `internal/editor` (1) and the external-editor path (1).
- `action_registry.go` splits: the `Action` type is clean — `Checked`, `Visible`
  and `Handler` are `func() bool` — but its 2554-line `init()`
  (`:264-2817`, 173 `RegisterAction` calls) mentions `PanelsFrame` 114 times and
  `EditorView` 47 times. Locate it as `grep -n '^func init()'`, never by the line
  number: it has already moved once, ten lines down, when Task 3 documented the
  ordering above it. Mechanism is layer 0; the table is layer 4.
- `framework_actions.go` is **not** a primitive at all. Of its 25 functions, 18
  have no external callers — they are `Handler:` values reached from the table,
  plus `main.go:actionScreenDump`. It travels whole with `internal/app`.

Moving the named *files* instead of the named *symbols* produces an uncompilable
commit. Phase 4 carries the full table.

**Five more corrections that change the plan.**

1. Three `F4Config` field types are declared outside `config.go`, not two:
   `PanelNavigationMode` (`navigation_mode.go:7`), `compareOptions`
   (`compare_folders.go:61`) and **`StartupMode` (`startup_backend.go:11`)** —
   the third in composition-root code that leaves last, which would make layer 0
   depend on layer 4. Task 4.
2. `misc.go`'s split is inverted relative to the document: the numeric helpers
   have the widest fan-out; `ScreenRow` has five callers and all are `_test.go`.
   Tasks 9 and 19.
3. `internal/sysinfo` **cannot** import the extracted numeric helpers — the rule
   says it imports no other `internal/*`, so the extraction would create the
   forbidden edge rather than remove it. It keeps a private five-line copy for its
   two call sites at `cpu_info_darwin.go:39` and `:47`. Task 19.
4. The shared test harness cannot be one package. `setupMockPanelsFrame` calls
   `NewTerminalView`, `NewCommandLine`, `NewFileSystemPanel` and
   `PanelsFrame.initPTY`, so a package holding it imports three layer-3 packages —
   and their own in-package tests then cannot import it back. Split into
   `internal/testutil` (vtui glue, caller-supplied drains) and
   `internal/paneltest`, with the affected tests becoming `package X_test`.
   Tasks 9 and 34.
5. Two more barriers the reconnaissance did not name: the drive registry
   (`DriveEntry`, `DriveRegistry`, `RegisterDrive`) is parked on
   `panels_frame.go:26-40` although it needs only `sync` and `vfs`, and
   `gpu_info_linux.go:113` is the single `Msg` call inside the sysinfo family.
   Both must go before the first wave. Tasks 6 and 7.

**A check that names a place must assert it found something there.** This was
written as one prediction about one test and it turned out to be a class. The
prediction: `build.yml` skips `TestAllDialogs_LayoutValidation` globally and
re-runs it single-threaded only for a named target, so when the test moves it is
skipped everywhere and re-run nowhere — a green build with a missing test. Task
25 saw it happen and fixed the path. Task 36 moved the test again and the same
trap fired in the same place, because fixing the path fixes the instance.

Three of these landed in this work, and they differ only in what emptied:

| what named a place | what emptied | who noticed |
|---|---|---|
| the layout test's isolated re-run | the package it named | nobody, twice — `go test -run` prints `[no tests to run]` and exits 0 |
| `go generate ./cmd/f4` | the directive, which travelled with `main.go` | nobody — `go generate` on a package with no directives exits 0 |
| the race job's three heaviest shards | the package they filtered over | nobody — they ran four auditor files and passed |
| the per-commit build walk | every merge commit — `git diff-tree` prints nothing for one without `-m --first-parent` | nobody — 59 of 68 commits were checked and the count looked complete |
| a test double embedding a live `vfs.VFS` | the two methods a regexp copy left behind | **nobody at all** — the package compiled, the calls resolved to the real filesystem through the embedded value, and the test hung for two seconds instead of failing |
| `docs/FILELIST.md` | its subject — it described the base revision's tree | nobody, for eleven phases; no check regenerates it |

The rule that closes the class rather than the case: **a check that names a
location must fail when it finds nothing there**, and it has to say so itself,
because the tool will not. The layout re-run now greps its own `--- PASS`; the
generate step now asserts a directive exists before running it; the shards no
longer name a package at all.

**A number carried without being re-derived is the same failure wearing a
different coat.** Task 39 was written around ~2450 pre-existing lint findings —
the maintainer's first impression of the pull request would be a red job with
thousands of entries, and the task existed to prevent that. Measured on the
finished tree: `origin/main` has **389**, `upstream/main` **375**, this branch
**276**, and the incremental job reports **0**. Nobody re-derived the 2450 in
eleven phases; it appears to come from a configuration that has since narrowed.
A whole task stood on it, and the problem it guarded against did not exist. Like
a check that names a place, a number looks like it is working right up until
somebody asks it.

The distinguishing question is what a command does with empty input, not how
important the command is. `cp -r internal/i18n/lang internal/dialog/help build/`
is self-checking — `cp` fails on a missing source — which is why moving `lang/`
and `help/` in phases 5 and 6 broke nothing quietly. `go test`, `go generate` and
a shard filter all succeed on nothing.

**The extraction gate.** A package leaves `cmd/f4` only when every symbol it calls
already lives in an extracted package, in itself, or outside the module. The
mechanical form is per **(file, destination)**: count references to types whose
own package is extracted *later*, and resolve each non-zero before moving.
Measured over all 345 non-test files at the base revision: **235 score zero on
every type** and are a pure `git mv`; 58 score 1-3; 52 score 4 or more. Phase 1
leaves 346: `navigation_mode.go` is deleted, `drive_registry.go` and
`action_order.go` are created, and all three score zero, so the zero bucket
becomes 236.

**A call graph attributes by name, and 722 names here have more than one
bearer.** Across 4247 nodes, `Close` has 203 and `Read` 95 — Go's implicit
interfaces mean a method name is shared by everything that satisfies the same
shape. So a graph query that asks "which package does this file point at" can
answer with a package the file does not import: `internal/plughost/application.go`
and `internal/viewer/application.go` both came back pointing at `internal/app`,
because each declares its own `Application`. The graph proposes; the file's own
import list decides. That is one extra command and it removes fourteen of
fifteen candidates.

**The eight-type grep is necessary, not sufficient.** It finds view types only. A
file also may not reference any *other* symbol still in `cmd/f4`, and the gate is
silent about those. `plugin_permissions_ui.go` scores `0` on all eight and still
cannot move to `internal/dialog`, because its entry point takes a
`*PermissionStore` that lives in `plugin_permissions.go` and leaves with
`internal/plughost` one task later. The `codegraph callees` half of wave-procedure
step 1 is the binding check; a zero score licenses nothing on its own.

**Tests travel by subject, not by filename.** 154 of the 346 `_test.go` files have
no same-named source — they are named for the scenario they exercise. "Take the
`_test.go` neighbour" therefore strands 45% of the suite. Task 43 holds the
assignment, measured on the CodeGraph index: a table of all 154, a roster of all
346 per wave, the 61 tests whose unexported references span more than one
package (each with a host and an export list), the eight tests that read
`lang/`, `help/` or `styles/` from disk, and the scaffolding — 48 shared helpers
and `TestMain` — the waves would otherwise strand.

**Ground rules for every commit.**

- **Builds on the whole matrix — 26 targets, exotic ones included, `CGO_ENABLED=0`.**
  Verified at two levels, because the two cost differently:
  - *After every commit, locally.* `CGO_ENABLED=0 GOOS=… GOARCH=… go build ./...`
    across the tag-sensitive targets — windows, linux, darwin, solaris, illumos,
    freebsd, plus one exotic arch such as linux/mips. Seconds per target, and it
    catches exactly the mistake this plan is most likely to make: a file selected
    by its name instead of its `//go:build` line.
    **freebsd and netbsd need `-gcflags=github.com/go-webgpu/goffi/internal/fakecgo=-std`**
    (the flag the matrix itself passes, `build.yml:160,411`). Without it the build
    fails on `//go:cgo_export_dynamic … only allowed in cgo-generated code`, which
    looks exactly like a breakage we caused and is not one. Verified on the
    current tree: all six sampled targets build clean, freebsd only with the flag.
  - *After every phase, in CI.* `gh workflow run build.yml --ref <branch>`.
    It must be `workflow_dispatch`, **not** a pull request: `build-batch`, which
    holds every exotic target, is gated on
    `github.event_name != 'pull_request'` (`build.yml:344`), and so is the
    cross-libc smoke test (`build.yml:251`). A PR therefore builds only the six
    desktop cells and would report green while the targets most at risk went
    unbuilt. Note also that a commit touching only `.md` and `docs/` skips CI
    entirely on a PR (`paths-ignore`), which is why the documentation phases
    cannot be checked this way at all.
  - *Read the previous run before starting the next phase.* Launching does not
    block — that half was written down — but a run nobody comes back to is a
    check that was not performed. Run `34204074671` sat red for six hours with
    the answer available ten minutes in, and it was red for exactly the reason
    the local sweep could not see: a `//go:build linux` test naming a constant a
    wave had moved. First thing at every phase boundary, before anything else:
    ```
    gh run list --repo ArtemYurov/f4 --workflow=build.yml --limit 3
    ```
  - **A run number without its revision is not a fact.** Write it as
    "34219411634 — green on 947d58a9", and before calling a run current, ask
    what it actually tested:
    ```
    gh run view <id> --repo ArtemYurov/f4 --json headSha,conclusion -q '.headSha[0:8]+" "+.conclusion'
    git rev-list --count <headSha>..HEAD
    ```
    A second number above zero is how many commits the run is behind, and its
    "green" belongs to a different tree. The last known green was quoted for
    three waves while it described a revision from before `internal/terminal`,
    `internal/media`, `internal/fileops` and `internal/editor` existed; the
    first push after that found seven red cells at once.

    The number is not the cause. Phases 6 and 7 closed without a push, and a
    phase boundary without a push is a boundary **without** CI, not one with CI
    deferred.
  - **`F4_FORCE_TESTS=1` belongs in the local sweep**, beside the cross-build and
    the `GOOS` vet. `testutil.SkipIfNoRelevantChanges` skips by a hash of the
    files a test covers, and never skips in CI, where `CI=1` is set. A full local
    run can therefore be green because a test silently did not run —
    `TestAllDialogs_LayoutValidation` is the one that matters, and it was green
    locally and panicking in CI at the same time. Same class as `xbuild.sh`
    swallowing `go build`'s status.
  - **`go test` makes a package's own directory the working directory.** Any path
    a test builds relative to it moves with the test and breaks silently. Task 43
    counts eight tests that read `lang/`, `help/` or `styles/` from disk; the
    ConPTY bundle check is a ninth, and it broke because CI copies the bundle to
    `cmd/f4`, where the test used to live.
  - *Not after every commit in CI.* One run is ~30 jobs against 20 free-tier
    runners, and `concurrency` cancels the in-flight run on the same ref
    (`build.yml:29-31`), so consecutive pushes would queue up and kill each
    other. Eleven phase runs give the same coverage as twenty-seven commit runs.
- **Lint what you touched, before you commit.**
  `golangci-lint run --new-from-rev=origin/main <the packages you changed>`, at the
  version CI pins (v2.13.1). Incremental mode says nothing about existing code —
  roughly 2450 findings of backlog sit behind it — but every line the diff calls
  new is checked, and that has two consequences. An edit inside a file is checked
  at once. And a file that *moves* changes its `package` line, so if git does not
  detect the rename the whole file counts as new and empties its share of the
  backlog into the report; Task 39 is written for exactly that.

  Note what "new" means here: the base is `origin/main`, the fork's own main, not
  `upstream/main`. Whatever the fork is behind by is reported as yours. Task 39
  levels them before it measures anything.

  **Task 39's premise has changed and the task must be re-measured, not
  executed as written.** It was written around a rename git cannot detect
  against `origin/main` dumping a file's whole share of the backlog into the
  report at once. That share has been paid down wave by wave: after Task 35,
  `golangci-lint --new-from-rev=origin/main` reports **nothing** across the
  tree. Measure first; the task's shape depends on what is left.

  **Pass `--max-same-issues=0 --max-issues-per-linter=0`.** The defaults are 3
  and 50, and the tool prints the count *after* truncation. A run that ends
  "3 issues: errcheck: 3" looks like a short list to fix and was 30 in the config
  wave — the same finding on thirty lines, twenty-seven of them hidden. The
  report reads as reassuring precisely when there is most to do.

- **Line numbers in `.github/workflows/build.yml` are a moving target.** The file
  grew from about 1350 lines to 1601 during this work and every upstream merge
  shifts it again. Every citation of it in this bundle is a convenience, not an
  address: locate the line by grepping its content — `cp -r cmd/f4/lang`,
  `TestAllDialogs_LayoutValidation`, `new-from-rev`, `fakecgo=-std` — and treat a
  mismatch as drift in the number rather than a change in the workflow.

- **Move by `//go:build` line, never by filename.** `pty_unix.go` is
  `//go:build linux`; `solaris_pty.go` is `//go:build !windows` and holds no PTY
  code. 97 non-test files carry a tag across 28 distinct expressions.
- **Export a symbol declared under several build tags in every variant at once.**
  The compiler on your platform shows exactly one: only your half compiles
  locally, and the other stays lower-case until the cross-build says so. The
  check is not "it built" but
  `grep -l "func <name>" <every variant>` and a comparison of the case between
  them. `IsBatchCommand` and `ResolveWindowsCommand` are declared once for
  `windows` and once for `!windows`; exporting them on darwin left two targets
  failing.
- No rewrites inside a move commit. A reviewer must read the diff as a rename.
- Tests move with their subject in the same commit; a test without a subject
  moves with the wave Task 43's roster names.
- Compare against `.ai-factory/RESTRUCTURE_BASELINE.md`; **never rewrite it.** It
  is an immutable snapshot of the Task 0 revision, and only Task 42 touches it.
  Run all six modules — `go test ./...` sees only the main module's 38 packages.
- Each commit closes its own references in `docs/`, `README.md`, `AGENTS.md` and
  `.ai-factory/rules/base.md`, and fixes the CI lines it breaks.
- `git mv` for anything git already tracks; the executable bit and build tags must
  survive.
- **Track upstream at phase boundaries; merge whatever touches Go.**
  Upstream is active — 29 commits landed on this branch's base in a day — so a
  single sync at the start is not a plan, it is a deferral.

  *Check at every phase boundary.* It costs a second and touches nothing:
  ```
  git fetch upstream --quiet
  git rev-list --count HEAD..upstream/main
  git diff --name-only HEAD...upstream/main
  ```

  *Merge if anything Go-related came in*, not only what the current wave moves.
  Every commit left unmerged gets more expensive as files scatter: a change to
  `cmd/f4/pty_windows.go` merges by itself while the file sits there, and becomes
  a manual reconstruction of somebody else's intent once the file has moved to
  `internal/terminal` with a new package clause. Documentation-only arrivals
  (`docs/LUNOBOT/*` and the like) can wait for the next batch — they conflict
  with nothing.

  Waiting for a file to become "near" is the wrong instinct: upstream touches
  what we will move several phases from now, not what we are holding. All three
  overlaps so far went that way — `panels_frame_test.go`, `actions.go` with
  `file_panel.go`, then `pty_windows.go`.

  *If a phase runs long*, check once in the middle too, on any green commit. Not
  on a timer — just when the phase has visibly stretched.

  *`git merge upstream/main`, not rebase.* Rebase replays each of our commits
  onto the new base separately, so one foreign edit to a file five of our commits
  touch is reconciled five times — each against an intermediate state of that
  file which does not exist in the result. A merge resolves it once, against the
  branch as it actually stands. Rebase also rewrites every one of our commits on
  every sync, which at two syncs a day is a great deal of rewriting of code we
  did not write.

  *Never mid-task*, and never with anything staged: `git commit` the current task
  first, or the merge trips over the index. A merge rewrites no history, so it
  needs no backup branch — `git merge --abort` before it lands, `git revert -m 1`
  after. Keep the backup-branch rule for operations that do rewrite history, and
  delete those branches in the same sitting.

  *After each merge:* compare against the baseline, run the cross-compilation
  sweep, and **re-measure every number the next tasks stand on**. This is not
  ceremony: the first mid-work rebase moved `action_registry.go`'s `init()` by
  ten lines, took `PanelsFrame` mentions inside it from 109 to 114, and added a
  106th method to `FileSystemPanel` — all of them quoted in task text.

**Open questions:** none blocking. The four package names above are the only
planning decision the user may wish to overrule, and doing so changes four task
titles, not the ordering.

**Session handoff:** [HANDOFF.md](HANDOFF.md) — where the work stands, the
upstream merge that must happen first, the open tails and the tool hazards.

## Phase Index

1. [Phase 1: Upstream Sync, Baseline and Barrier Removal](phase-01-baseline-and-barriers.md) — Tasks 0-9 and 43
2. [Phase 2: Clear the Repository Root](phase-02-repository-root.md) — Tasks 10-15
3. [Phase 3: Self-Contained Subsystems Under internal/](phase-03-subsystems.md) — Tasks 16-17
4. [Phase 4: The Shared Primitives Leave cmd/f4](phase-04-shared-primitives.md) — Tasks 18-21
5. [Phase 5: Leaf Packages](phase-05-leaf-packages.md) — Tasks 22-24
6. [Phase 6: Hosts and Services](phase-06-hosts-and-services.md) — Tasks 25-28
7. [Phase 7: Viewer, Terminal and Media](phase-07-view-and-terminal.md) — Tasks 29-31
8. [Phase 8: File Operations and the Editor](phase-08-fileops-and-editor.md) — Tasks 32-33
9. [Phase 9: Panels and the Command Line](phase-09-panel-and-cmdline.md) — Tasks 34-35
10. [Phase 10: The Composition Root](phase-10-composition-root.md) — Tasks 36-37
11. [Phase 11: CI, Lint and Documentation](phase-11-ci-and-docs.md) — Tasks 38-42

## Cross-Phase Dependencies

- **Task 1 depends on Task 0** — a baseline is valid only for the revision it was
  taken at, so the rebase must immediately precede it. Run them as one sitting.
- **Task 18 depends on Task 3** — splitting the registry across a package boundary
  reorders the menu unless the order is already explicit and golden-tested.
- **Task 21 depends on Task 18** — the mechanism must be separated from the table
  before it can move.
- **Task 22 depends on Tasks 6, 7, 19, 20, 21 and 43** — the drive registry must be
  off the panel type, the `Msg` call out of `gpu_info_linux.go`, and the private
  numeric copy in place, or `internal/sysinfo` is not a leaf and cannot go first.
  Task 20 is what makes `toast.Show` callable from a package: `showToast` has 16
  call sites spanning eight future packages, so every wave from here on needs it.
  Task 43 is what tells this wave which files it owns.
- **Task 24 depends on Task 4** — `internal/config` cannot be a leaf while
  `StartupMode` lives in composition-root code.
- **Tasks 22-35 depend on Tasks 19-21** — 37 call edges run into what would
  otherwise be `internal/app` from lower layers. Extracting in the other order
  makes every intermediate commit uncompilable.
- **Task 31 depends on Task 30** — six of media's ten outbound edges point at
  `internal/terminal`, which is why term precedes media despite the higher count.
- **Task 32 depends on Tasks 5 and 30** — `clipboard.go`, `clipboard_async.go` and
  `background_jobs.go` are pulled into the term wave specifically to prevent a
  `fileops ↔ term` cycle here; and `queue_manager.go` only becomes movable once
  Task 5 has taken the goroutine launch out of its `init()`.
- **Task 33 depends on Tasks 14 and 29** — the `editor ↔ viewer` cycle is removed
  by the viewer wave taking `top_bar.go`, `file_title.go` and `url_links.go`; and
  `colorer_plugin.go` reads `colorer.RadiolaHRD`, which does not exist until
  Task 14 creates `internal/colorer`.
- **Task 34 depends on Tasks 26, 29, 30, 32 and 33** — `panel_plugins.go`'s
  `coreAPI` method is cut out in Task 26, and `internal/panel` imports viewer,
  term, fileops and editor.
- **Task 34 depends on Task 35**, not the other way round. The dependency the
  plan recorded rested on a cycle that measurement did not find: five files
  rostered for `internal/cmdline` read private members of the panel types and
  belong to `internal/panel`, and with them placed there `internal/cmdline`
  names no panel type at all. The command line goes first because thirteen of
  its seventeen files move mechanically, and because a boundary that exists
  before the panel wave starts cannot be crossed by accident.
- **Task 36 depends on every wave** — the composition root is what is left.
- **Task 41 depends on Task 37** — `ARCHITECTURE.md` can describe the tree as a
  fact only once the tree is the tree.
- **Re-homing what upstream adds past the roster is a step of Task 44, not a
  task after the merge.** It was written to run post-PR, because the list is
  only complete once upstream stops moving under an open pull request. Three
  things retired that: the branch is level with `upstream/main`, `cmd/f4` is
  five files so a stray arrival breaks the build rather than landing quietly,
  and every merge has placed its own arrivals as it went. The residue is covered
  by merging once more immediately before opening the PR.
- **Task 46 depends on nothing, and Task 32 must not start without it.** It
  guards Tasks 32-36 against the one damage the waves actually cause: a script
  that rewrites identifiers rewriting a string literal instead. Run after the
  waves it would find nothing left to find.

## Tasks

### Phase 1: Upstream Sync, Baseline and Barrier Removal
- [x] Task 0: Rebase onto `upstream/main` behind a backup branch, re-verify the plan's counts ([details](phase-01-baseline-and-barriers.md#task-0-synchronize-with-upstreammain))
- [x] Task 1: Record the immutable pre-restructuring baseline across all six modules ([details](phase-01-baseline-and-barriers.md#task-1-record-the-pre-restructuring-test-baseline)) (depends on 0)
- [x] Task 2: Re-key the command-palette auditor's 42 entries to qualified symbols ([details](phase-01-baseline-and-barriers.md#task-2-re-key-the-command-palette-auditor-to-qualified-symbols))
- [x] Task 3: Make action registration order explicit and golden-tested ([details](phase-01-baseline-and-barriers.md#task-3-make-action-registration-order-explicit))
- [x] Task 4: Move `F4Config`'s three stray field types into `config.go` ([details](phase-01-baseline-and-barriers.md#task-4-move-f4configs-field-types-into-configgo))
- [x] Task 5: Stop `queue_manager.go` starting a goroutine from `init()` ([details](phase-01-baseline-and-barriers.md#task-5-stop-starting-a-goroutine-from-init))
- [x] Task 6: Lift the drive registry out of `panels_frame.go` ([details](phase-01-baseline-and-barriers.md#task-6-lift-the-drive-registry-out-of-panels_framego))
- [x] Task 7: Remove sysinfo's last localization call (`gpu_info_linux.go:113`) ([details](phase-01-baseline-and-barriers.md#task-7-remove-sysinfos-last-localization-call))
- [x] Task 8: Add the module boundary auditor `cmd/f4/architecture_test.go` ([details](phase-01-baseline-and-barriers.md#task-8-add-the-module-boundary-auditor))
- [x] Task 9: Split the shared frame harness into `internal/testutil` + `internal/paneltest` ([details](phase-01-baseline-and-barriers.md#task-9-give-the-shared-frame-harness-a-home))
- [x] Task 43: Assign every `cmd/f4` file to a wave — 23 stray sources, 154 subject-less tests, 61 multi-package tests, 48 shared helpers ([details](phase-01-baseline-and-barriers.md#task-43-assign-every-cmdf4-file-to-a-wave)) (depends on 8)

### Phase 2: Clear the Repository Root
- [x] Task 10: Move the three shell scripts to `scripts/` ([details](phase-02-repository-root.md#task-10-move-the-shell-scripts-to-scripts)) (depends on 1)
- [x] Task 11: Move `screenshot.png` to `.github/assets/` ([details](phase-02-repository-root.md#task-11-move-screenshotpng-to-githubassets))
- [x] Task 12: Move the loose prose into `docs/` and delete `time.txt` ([details](phase-02-repository-root.md#task-12-move-the-loose-prose-into-docs-and-delete-timetxt))
- [x] Task 13: Rename the 40 issue reviews to `ISSUE_<number>_<SLUG>.md` ([details](phase-02-repository-root.md#task-13-rename-the-issue-reviews-to-issue_number_slugmd)) (depends on 12)
- [x] Task 14: Move `colorer/` to `internal/colorer/` with its own embed ([details](phase-02-repository-root.md#task-14-move-colorer-to-internalcolorer))
- [x] Task 15: Keep `plugring/` in the root; spell its URL once and make its policy test run ([details](phase-02-repository-root.md#task-15-keep-plugring-in-the-root-and-make-its-two-seams-honest))

### Phase 3: Self-Contained Subsystems Under internal/
- [x] Task 16: Move `piecetable`, `textlayout` and `sheet` under `internal/` ([details](phase-03-subsystems.md#task-16-move-piecetable-textlayout-and-sheet)) (depends on 15)
- [x] Task 17: Move `fusefs`, `vtvibe` and `luaplug` under `internal/` ([details](phase-03-subsystems.md#task-17-move-fusefs-vtvibe-and-luaplug))

### Phase 4: The Shared Primitives Leave cmd/f4
- [x] Task 18: Split `action_registry.go` into mechanism and table, in place ([details](phase-04-shared-primitives.md#task-18-separate-the-action-registrys-mechanism-from-its-table)) (depends on 3, 17)
- [x] Task 19: Create `internal/numeric`; give sysinfo its private copy ([details](phase-04-shared-primitives.md#task-19-create-internalnumeric)) (depends on 9)
- [x] Task 20: Create `internal/toast` and `internal/history` ([details](phase-04-shared-primitives.md#task-20-create-internaltoast-and-internalhistory))
- [x] Task 21: Create `internal/action` with a localizer hook ([details](phase-04-shared-primitives.md#task-21-create-internalaction)) (depends on 18)

### Phase 5: Leaf Packages
- [x] Task 22: Extract `internal/sysinfo` (1 outbound) ([details](phase-05-leaf-packages.md#task-22-extract-internalsysinfo)) (depends on 6, 7, 19, 20, 21, 43)
- [x] Task 23: Extract `internal/update` (3 outbound) ([details](phase-05-leaf-packages.md#task-23-extract-internalupdate)) (depends on 22)
- [x] Task 24: Extract `internal/config`, `internal/i18n`, `internal/theme`, `internal/keymap` ([details](phase-05-leaf-packages.md#task-24-extract-internalconfig-internali18n-internaltheme-internalkeymap)) (depends on 4, 23)

### Phase 6: Hosts and Services
- [x] Task 25: Extract `internal/dialog`, and fix the silent dialog-test drop ([details](phase-06-hosts-and-services.md#task-25-extract-internaldialog)) (depends on 24)
- [x] Task 26: Extract `internal/plughost`; cut `panel_plugins.go`'s `coreAPI` method ([details](phase-06-hosts-and-services.md#task-26-extract-internalplughost)) (depends on 25)
- [x] Task 27: Extract `internal/gui`; move two of three `tools/icons` paths ([details](phase-06-hosts-and-services.md#task-27-extract-internalgui)) (depends on 26)
- [x] Task 28: Extract `internal/macro` ([details](phase-06-hosts-and-services.md#task-28-extract-internalmacro)) (depends on 27)

### Phase 7: Viewer, Terminal and Media
- [x] Task 29: Extract `internal/viewer`, removing the `editor ↔ viewer` cycle ([details](phase-07-view-and-terminal.md#task-29-extract-internalviewer)) (depends on 28)
- [x] Task 30: Extract `internal/terminal`, including eleven misfiled files ([details](phase-07-view-and-terminal.md#task-30-extract-internalterm)) (depends on 29)
- [x] Task 31: Extract `internal/media` ([details](phase-07-view-and-terminal.md#task-31-extract-internalmedia)) (depends on 30)

### Phase 8: File Operations and the Editor
- [x] Task 46: Audit the message keys — every literal `Msg`/`HelpMsg` key exists in `en.lng` ([details](phase-08-fileops-and-editor.md#task-46)) — runs before Task 32, outside the phase order
- [x] Task 32: Extract `internal/fileops` ([details](phase-08-fileops-and-editor.md#task-32-extract-internalfileops)) (depends on 5, 30, 31)
- [x] Task 33: Extract `internal/editor` ([details](phase-08-fileops-and-editor.md#task-33-extract-internaleditor)) (depends on 14, 29, 32)

### Phase 9: The Command Line and the Panels
- [x] Task 35: Extract `internal/cmdline` ([details](phase-09-panel-and-cmdline.md#task-35-extract-internalcmdline)) (depends on 33) — runs first
- [x] Task 34: Extract `internal/panel`, finish `semantic.go`, fill `internal/paneltest` ([details](phase-09-panel-and-cmdline.md#task-34-extract-internalpanel)) (depends on 26, 33, 35)

### Phase 10: The Composition Root
- [x] Task 36: Extract `internal/app` ([details](phase-10-composition-root.md#task-36-extract-internalapp)) (depends on 35)
- [x] Task 37: Reduce `cmd/f4` to the entry point ([details](phase-10-composition-root.md#task-37-reduce-cmdf4-to-the-entry-point)) (depends on 36)

### Phase 11: CI, Lint and Documentation
- [x] Task 38: Rebalance the CI shards; measure before and after ([details](phase-11-ci-and-docs.md#task-38-rebalance-the-ci-shards)) (depends on 37)
- [x] Task 39: Run the incremental lint, and verify every commit builds, before opening the PR ([details](phase-11-ci-and-docs.md#task-39-run-the-incremental-lint-against-originmain-before-opening-the-pr)) (depends on 38)
- [x] Task 40: `/aif-docs` checkpoint; rewrite `AGENTS.md` and `rules/base.md` ([details](phase-11-ci-and-docs.md#task-40-aif-docs-checkpoint)) (depends on 37)
- [x] Task 41: Rewrite `ARCHITECTURE.md` from target to fact ([details](phase-11-ci-and-docs.md#task-41-rewrite-architecturemd-from-target-to-fact)) (depends on 40)
- [x] Task 42: Drop the migration baseline ([details](phase-11-ci-and-docs.md#task-42-drop-the-migration-baseline)) (depends on 41)
- [x] Task 44: Review the finished tree before calling it done ([details](phase-11-ci-and-docs.md#task-44-review-the-finished-tree-before-calling-it-done)) (depends on 42)
- [x] Task 45: Write the pull request ([details](phase-11-ci-and-docs.md#task-45-write-the-pull-request)) (depends on 44) — the last task

## Open Findings

Things measured and not yet settled. Each names the evidence and the next step,
so that whoever picks this up does not re-derive it.

### Decided: `internal/plughost` gets an application interface — Task 26

**The question.** Every extraction through Task 25 was mechanical: score the
gate, `git mv`, requalify the call sites. Task 26 is the first that cannot be,
and the next four (`gui`, `macro`, `viewer`, `term`) are likely the same shape.
Somebody has to decide whether this pull request designs a host interface or
stops short of the packages that need one.

**What was measured.** The wave was attempted and reverted; the tree is
unchanged and green. Thirteen files moved cleanly into `internal/plughost`
before anything broke — all four transports, permissions, scaffold, the plugring
reader, the sqlite actions. Then:

- All twenty candidate files name `Plugin` or `PluginTransport`, both declared
  in `plughost.go`. Nothing moves until that file does.
- `plughost.go` is 272 lines in two halves. Lines 1-94 are host plumbing —
  `pluginInitTimeout`, `startPluginSession`, the `PluginTransport` interface —
  and move. Lines 95-238 are `newHostMethods`, the RPC table a plugin calls
  into, and two of its entries do `pf := findPanelsFrame()` and then
  `pf.RunProgressTask(…)` / `pf.Menu(…)`. That is layer 4 by definition.
- `extui_host.go` (678 lines) calls `SetupUI()` at :473, `setF4Clipboard` at
  :584 and `HandleSemanticAction` at :592. It is the external-UI protocol: a
  plugin asking f4 to restart its UI, set the clipboard, run a semantic action.
- `api.go` is 60 lines and 10 methods, and the task text calls it "this is its
  home". It is not: `coreAPI` implements `vfs.HostAPI` and its bodies call
  `getShortVersionInfo`, `RunAction`, `RegisterGlobalHotkey`,
  `findPanelsFrameAnyScreen` and `SetupUI`. It is the composition root's
  implementation of an interface that already exists in `vfs` (layer 0).

**The dead end, so it is not walked again.** Moving `api.go` into
`internal/plughost` and resolving symbols outward does not converge: each
resolved name pulls in another app function, and the closure is most of
`cmd/f4`. The gate says 0 for eleven of these files and it is right about view
types and wrong about the package — the same false confidence `colors.go` and
`grabber.go` gave in the other direction.

**The options, and what each costs.**

1. *Design the host boundary.* `internal/plughost` takes `vfs.HostAPI` — which
   it already receives as a parameter — plus the app-facing RPC table, supplied
   by the root rather than built in the package. `newHostMethods` and
   `extui_host.go` stay in `cmd/f4` and travel to `internal/app`; `api.go` goes
   with them. This is what `ARCHITECTURE.md` already prescribes ("a lower layer
   that needs something from the host defines its own interface") and it is the
   only option that produces the package the target tree names. It is design
   work per subsystem, not a move, and the same question returns for `gui`,
   `macro`, `viewer` and `term`.
2. *Extract only what moves mechanically, and say so.* Run the gate over the
   remaining rosters, report how much of each package is reachable without
   designing an interface, and finish at Phase 11 with the tree that produces.
   Cheap, honest, and leaves `ARCHITECTURE.md`'s tree partly unrealised — which
   Task 41 then has to describe as fact rather than as target.
3. *Stop at Phase 5.* Twenty-six of forty-six tasks, `cmd/f4` down from 687
   files to 592, ten new packages, every commit green. Phase 11 closes the pull
   request on that.

**The decision: option 1, design the boundary.** The branch promised to lay the
application out in modules, not to move files, and seams are already how it
does that — `config.Executable`, `keymap.Suspended`, `action.Localize`, the
seven `TestMain` seams. The host boundary is the same technique at a larger
size, not a new one.

**It is smaller than the options above suggested, and here is why.** The
interface between the host and a plugin does not need designing: it exists and
it is public. `vfs.HostAPI` is ten methods, and every transport already takes it
— `Plugin.Init(api vfs.HostAPI)` at `plugins.go:26`, implemented by the RPC,
WASM and Lua transports.

What is missing is the interface in the other direction: what `internal/plughost`
needs **from the application**. Measured, that is five methods, and they are the
five that blocked the wave:

| method | today | called from |
|---|---|---|
| run a progress task | `pf.RunProgressTask` | `plughost.go:157` |
| open a menu | `pf.Menu` | `plughost.go:227` |
| rebuild the UI | `SetupUI()` | `extui_host.go:473` |
| set the clipboard | `setF4Clipboard` | `extui_host.go:584` |
| run a semantic action | `HandleSemanticAction` | `extui_host.go:592` |

Declared in `internal/plughost`, implemented in `internal/app`, handed to the
host by its constructor. `newHostMethods` and `extui_host.go` then stop naming
the panel frame, and `api.go` — the root's implementation of `vfs.HostAPI` —
goes to `internal/app` where it belongs.

**What must not be done: these five do not go into `vfs.HostAPI`.** That
interface is the security perimeter. Everything in it is reachable by any
plugin, including a third-party one installed from the catalogue, and
`plugin_permissions.go:20` keeps the permission list deliberately narrow — "a
permission f4 cannot actually enforce is theatre, and teaches people that the
dialog means nothing". `SetupUI` would let a plugin rebuild the entire
interface; clipboard access would hand it the user's data. A progress task and a
menu are arguably fine in a public plugin API, but that is a product decision
with its own discussion, not a side effect of a refactor.

So: two interfaces, two audiences. `vfs.HostAPI` is what the host gives a
plugin — public, guarded, unchanged by this branch. The new one is what the
application gives the host — internal, and no plugin ever sees it.

**The list is measured, never inferred — and Task 26 proved it on itself.** Its
own count moved in both directions once the wave was actually run: two of the
five above were unnecessary, because `vfs.App` already declares
`RunProgressTask` and `Menu`, so one method reaching the current application
replaced both; and four were missing — `AskOverwrite` and `AskError`, which
`newHostMethods` calls and this table did not count, plus `OpenPanelProvider`
and `IsStale`. The last one is the warning worth keeping: translating the
panel-liveness check as `Current() != app` compiles, reads correctly, and
silently rejects every application object that is not a panels frame. Two tests
caught it. A method list derived from another wave's is a guess with a
plausible shape.

**This decision covers the four waves after it.** `internal/gui`,
`internal/macro`, `internal/viewer` and `internal/terminal` will each hit the same
wall, because each is a subsystem the application drives rather than a leaf it
calls. Apply the same answer — the package declares what it needs from above,
the root supplies it — instead of reopening the question per wave. What each
wave still has to do on its own is *measure* its list the way Task 26's was
measured, because five methods is this host's number and not a general one.

### Confirmed: a rewritten string literal, and where it was found

`internal/dialog/settings_portable.go` called
`i18n.Msg("PortableSettings.ini.File")`. No `.lng` carries that key — the
language files spell it `PortableSettings.IniFile` — so the portable-mode dialog
rendered `{PortableSettings.ini.File}` in place of the profile-path caption. A
requalification pass turned `IniFile` into `ini.File` inside the string, which
compiles and passes every test.

Found by the `04ba3125` merge: upstream's side of the conflict carried the
correct key. The class is described in `HANDOFF.md` as a hazard; this is the
first instance with an address. **Task 46 closes it** — and note that it takes
the second of that task's two sweeps to do so, because the literal is handed to
a helper and never reaches `Msg` directly.

### Files that arrive from upstream after Task 43's roster

Two so far, both assigned by subject, both travelling further than `cmd/f4`, and
neither in the roster that is supposed to say where they go:

- **`cmd/f4/local_language_files_test.go`** — subjects are `initLang`
  (`cmd/f4/lang.go`) and `InitHelpSystem` (`cmd/f4/help_topics.go`), both bound
  for `internal/app` in Task 36. It holds three exports created for it:
  `config.CachedF4ConfigDir`, `config.CachedF4Portable`,
  `dialog.HelpActionStrings`. All three keep an external caller after the move,
  so the exports stay legitimate.
- **`cmd/f4/edit_command_test.go`** — subject is `parsePlainEditCommand`
  (`cmd/f4/panels_frame.go:5684`), bound for `internal/panel` in Task 34.

The rule this implies: **a file arriving from upstream after Task 43 has no home
in the roster.** Assign it by subject and record it here, or it stays in
`cmd/f4` on its own wave with nobody to notice. Upstream is active and two
merges have produced two such files; there will be a third.

### Closed: `attributes_dialog.go` goes to `internal/dialog`

The open tail is settled by the Task 32 wave. Its `PanelsFrame` ×9 were one
parameter threaded through for one `RefreshAll`; as a `refresh func()` the file
scores zero, and what it holds — 34 message lookups, a stack of vtui widgets,
no file operation — puts it with its siblings in `internal/dialog`. Task 43's
export table moves with it: `attributes_test.go` takes `ShowAttributes*` from
`internal/dialog`.

### `qual2.py` rewrote three packages it had no business in

The export-and-qualify loop renames an identifier everywhere it appears, and
"everywhere" includes other packages that happen to share the name and the
method declarations of interfaces. Running it for `internal/fileops` turned
`closeOnce` into `fileops.CloseOnce` in four `internal/terminal/pty_*.go` files
and in `panel_plugins.go`, and rewrote `AskOverwrite`/`AskError` — method names
in `internal/plughost`'s `Application` interface — into `fileops.AskOverwrite`
and `fileops.AskError`, comments included. All of it is a syntax error, which is
the only reason it was cheap to find.

The same pass also rewrote **four string literals**, which is not a syntax
error and was caught by the diff grep instead: a command-palette coverage key
became `"fileops.(*fileops.QueueFrame).ProcessKey"`, and three test messages
started naming package-qualified symbols in prose. The palette key is the one
that mattered — an auditor keyed by a name nothing produces.

The rule that holds, and it is the one HANDOFF already states: **grep the
wave's own diff for changed string content before running the suite.** Compare
the literals removed against the literals added; anything present on one side
only is either the change you meant or damage.

```
git diff --cached -M -- cmd/f4 internal | grep -E '^[+-]' | grep '"'
```

### A third package the plan did not name: `internal/appcmd`

`commands.go`'s 72 `vtui.CmApp + iota` constants are the protocol every frame
speaks — a panel raises `CmEdit`, the editor answers it — and open tail 3 left
their home to Task 34 or 35. Task 33 reached the question first: the editor
names four of them, and a package cannot import `cmd/f4`.

`internal/appcmd` is layer 0 and imports only `vtui`, which is what lets the
panels, the editor, the viewer and the command line all name the same numbers
without importing one another. The values are positional, so a line inserted in
the middle renumbers everything below it; the file says so.

### `internal/editor` imports `internal/viewer`, and that is correct

Task 33's contract ruled the edge out, and its reason was a cycle. The cycle is
gone: Task 29 moved `top_bar.go`, `file_title.go` and `url_links.go` into the
viewer, and the editor reads 22 symbols from them — `UrlLink` ×8, the
disassembly helpers, the word-category helpers, `TopBar`. `internal/viewer`
imports nothing back.

Same layer, one direction, no cycle. `architecture_test.go` — which checks the
edge rather than the sentence — passes. The contract line is the stale half.

### Five packages the plan did not name

`internal/ini` and `internal/unpack` (phase 5), `internal/textsearch` (phase 7),
`internal/appcmd` (Task 33) and `internal/semantic` (before Task 34). Not one of
them was chosen. Each is forced by a dependency rule that only becomes visible
once the boundaries exist: **a symbol three or more packages of one layer need
has a home in none of them.**

The plan named packages by subsystem — by what the code does. These five are
decided the other way, by who calls the code. Planning from the call graph finds
such places; planning from subject areas does not, and that is a limit of the
method rather than an oversight.

The mechanical form of the rule, which is also how each of the five was found:
**a function that has been copied three times has no home.** `internal/semantic`
came from exactly that count — the four readers that unpack a semantic action
and the two that turn screen cells into run models had been copied into the
viewer, the editor, the terminal and the command line, each with a `ponytail:`
marker promising a home later.

#### The first two, as they were recorded at the time

Both were forced by the dependency rules rather than chosen, and both import
nothing of ours, which is what makes them shareable by the layer-0 leaves.

- **`internal/ini`** (Task 24 groundwork). `config`, `i18n`, `theme` and
  `keymap` all parse ini files and none of them may import another of ours, so
  the parser cannot live in `internal/config` as the task text says.

  It was called `inifile` for a day, on the argument that fifty locals named
  `ini` would shadow it. Measured properly the shadowing is real but harmless:
  of 73 call sites, six sit in a scope where `ini` is already bound, and every
  one of those is a compile error rather than a silent misread — `ini.Load` is
  not a method on `*ini.File`. Six locals were renamed and the package took the
  short name Go convention asks for, the same way `net/url` keeps its name
  against `url := url.Parse(...)`.
- **`internal/unpack`** (Task 23 follow-up). Archive extraction has three
  callers — the updater, the plugin catalogue, the colorer downloader — and
  `SanitizePath` is the zip-slip guard for all three.

`ARCHITECTURE.md`'s tree and layer list now name both, and the leaf rule is
stated through the layers instead of through an exception: a layer-0 package
imports layer-0 packages and nothing else. `config` → `inifile` is legal as
0 → 0, and so is `keymap` → `numeric`, which the plan already sanctioned.

That wording is what it is because the auditor now checks it. `architectureLayers`
carried a layer number for every package and nothing read the numbers — only the
names, to catch a stale line. So `internal/config` importing `internal/panel`
would have passed every rule: it is not a cycle, and rule 3 names only
`internal/app`. Rule 6 rejects any edge from a lower layer to a higher one, and a
second test requires every `internal/*` package to appear in the map, because an
unplaced package is unchecked rather than exempt. Verified to fire by moving
`internal/unpack` to layer 2 and watching `internal/update` fail.

### `internal/config` needs an `Executable` seam in Task 24

`self_exec*.go` went to `internal/update` in Task 23, as the roster says, and
with it `Executable` — os.Executable corrected for the universal Linux build,
where `/proc/self/exe` names the loader. Five callers outside the updater use
it, and two of them are `config.go:33` and `portable.go:80`: the
portable-configuration probe, which is the first thing that runs.

Those two land in `internal/config`, a layer-0 package that may import no other
`internal/*`. So Task 24 cannot simply carry the call across. It declares the
seam instead — `var Executable = os.Executable` in `internal/config`, set to
`update.Executable` by the root — which is the shape `history.SamePath` and
`action.Localize` already use.

The hazard to watch is the default. `os.Executable` is *wrong* on a universal
build rather than merely unavailable, and a forgotten assignment leaves the
portable probe reading the ini next to `ld.so` with no error. Set it in
`main.go` beside the other two hooks, and before the first `GetF4ConfigDir()`.

The other three callers — `pty_windows.go`, `session_unix.go` and
`detach_unix.go` — are layer 3 and above and keep calling `update.Executable`
directly.

### The darwin mackeys flake has a mechanism, and it is closed

`TestMacKeysSkipsCommandRulesWithoutTheChannelSplit` failed once on
`Test (darwin/amd64)` with `mackeys_test.go:109: Opt+Left was not rewritten`,
and nothing on this branch had touched it. Task 24 found the mechanism while
extracting `internal/keymap`.

`applyMacKeys` refused to rewrite whenever `keyRemapSuspended()` said a foreign
program owned the keyboard, and that function read
`vtui.FrameManager.GetTopFrame()` — a process-wide global. A test that left a
`PanelsFrame` on the frame manager with `showPanels == false` and an AltScreen
`termView` made every later mackeys assertion report exactly "was not
rewritten": not a wrong rule, a suspended one. `-shuffle=on` decides whether
those tests are adjacent, which is why it failed once and passed the next run.

It is closed rather than diagnosed-and-left. The check is now
`keymap.Suspended`, a seam the root assigns, and `withMacKeys` pins it to false
for the duration of a test — the tests no longer read the frame manager at all.

The other cell, `TestMainMenuFilePath_HasExpectedSuffix` on linux/amd64, is
still open and its lead is unchanged: an empty `cachedF4ConfigDir` observed
between `setupPortableIni`'s cleanup replacing `configDirOnce` and the next
`Do` completing.

### Task 35 step 3 is wrong in its premise, and there is no cycle

The step reads "resolve the non-zeros against the panel type:
`command_prefix_registry.go` (1), `cmd_session.go` (2), `simple_exec.go` (2),
`apply_command.go` (5). A command that acts on the active panel takes it as a
parameter." There is nothing to resolve. All five read **private** members of
`PanelsFrame` and `FileSystemPanel`, and a parameter does not open a private
member:

| File | Private members reached |
|---|---|
| `cmd_session.go` | eight — `catchUpProcessEnvironment`, `endExecution`, `executing`, `ignoreNextPrompt`, `localPTY`, `noteLocalShellBusy`, `shellPromptReady`, `termView` |
| `apply_command.go` | nine, across both types — `activeIdx`, `closed`, `getActivePanel`, `getInactivePanel`, `ptyMutex`, `entries`, `vfs`, `clearSelectionIfUnchanged`, `getRawSelectedName` |
| `simple_exec.go` | eight, plus two methods declared on `*PanelsFrame` |
| `command_prefix_registry.go` | three — `closed`, `getActivePanel`, `switchToVFS` |
| `remote_command.go` | two — `getActivePanel`, `vfs` |

So the five belong to `internal/panel` by Go's rule, not by anyone's decision.

**And with them gone, the edge `cmdline → panel` does not exist.** The cycle the
step was written to break is not there. `internal/panel` imports
`internal/cmdline` in one direction, on one layer, the way `internal/editor`
imports `internal/viewer`. No interface, no seam.

For the record, since the shape of the alternative matters: `panel → cmdline` is
272 references to `.cmdLine` (257 of them production) across seventeen distinct
members, and one of the seventeen is `semanticModel`, an unexported method. The
interface the step implied would have required exporting a private method to
satisfy a rule about a cycle that does not exist. The reverse direction is zero.

**Order: `internal/cmdline` is extracted before `internal/panel`.** Thirteen of
the roster's seventeen files name no panel type at all and move mechanically;
the boundary then exists before the largest wave starts, which is what stops a
file from going the wrong way because it happened to sit next door.

Two roster corrections found in the same pass: `commands.go` is listed for
`internal/cmdline` and became `internal/appcmd` in Task 33, and
`simple_exec_other.go` / `simple_exec_windows.go` from step 5 do not exist.

### Closed: the empty config directory, and the flake it caused

`GetF4ConfigDir` returned `""` whenever a test had swapped `CachedF4ConfigDir`
and put back the empty value it found before the resolver first ran:
`ConfigDirOnce` had already fired, so nothing recomputed it. Fifteen tests
assign that cache directly, and any of them ordered before a reader produced
it.

What it looked like from outside: `filepath.Join("", "settings.ini")` is
`"settings.ini"`, relative to the working directory — which for `go test` is
the package's own. A shuffled run left `settings.ini`, `session.ini` and
`playlist.json` in `cmd/f4`, and `TestMainMenuFilePath_HasExpectedSuffix`
failed with `MainMenuFilePath()="settings/user_menu.ini"` instead of the
`f4/`-prefixed path. That is the CI-only failure recorded below, and the lead
noted there — "an empty `cachedF4ConfigDir` observed between `setupPortableIni`'s
cleanup and the next `Do`" — was right about the mechanism and wrong about the
race: no concurrency is needed, only an order.

`GetF4ConfigDir` now treats an empty cache as unresolved and resolves again.
No branch of `ResolveProfileDir` can return `""`, so the value is unambiguous.
Verified: eight shuffle seeds of the flaking test, zero failures, and no stray
files left in the package directory.

### Run 34284446204 on `7d7f5309`: two failures, neither where they looked

Read late, and read wrong the first time. Recording both the failures and the
misreading, because the misreading is the more useful half.

**Build (linux/amd64) failed the `gofmt -s` gate**, not `affected.calc`. The
step's log shows `git fetch --quiet --depth=1 origin "refs/heads/$GITHUB_BASE_REF"`
as its last echoed command and the failure 1.5 seconds later, which reads as
that line failing — but the echo is the *group header* GitHub prints for a step,
and the real output is two lines below it:

```
The following files are not properly formatted:
internal/cmdline/line_semantic.go
internal/terminal/view_semantic.go
```

`affected.calc` cannot fail on `workflow_dispatch` at all: its first statement is
`if [ "$GITHUB_EVENT_NAME" != "pull_request" ]; then all "not a pull request"`,
which exits 0 before the fetch. The empty `GITHUB_BASE_REF`, the "cannot
attribute to a package" branches and the empty race list were all read out of
that one misattributed line and none of them happened.

The two files were misformatted from the moment they were written, and the local
gate never said so because it runs `gofmt -w`, while CI runs **`gofmt -s -l`** —
the simplify pass. `gofmt -l` did list both files during the panel wave and the
listing was dismissed as "they parse". Add `gofmt -s -l .` to the local gate;
`gofmt -w` is not the same command and never was. Both files were reformatted in
passing by `f59248f4`, so this failure is already fixed.

GitHub names the failing steps itself, which is the reading to trust over any
log: `gh run view <id> --json jobs --jq '.jobs[] | select(.conclusion=="failure")
| {name, steps: [.steps[] | select(.conclusion=="failure") | .name]}'` returns
`Build (linux/amd64) -> Check formatting` and `Race (packages) -> Run race
detector`. `affected.calc` is not among them.

**Race (packages) failed on a goroutine-leak check, after every test passed.**
`internal/editor` left one `vtui` task-pump goroutine alive, and
`testutil.Main`'s check turned that into a failing package. It did not recur on
the next full run (34295760690, green), so it joins the two flakes below.

*Established, and not worth re-deriving:*

- The `TestMain` is not the cause. `internal/editor`'s and `internal/viewer`'s
  are byte-identical — `testutil.Main(m, theme.SetDefaultF4Palette, nil)` — and
  only the editor leaked.
- Teardown discipline is not the cause. `testutil.Main` already shuts down the
  global manager, and the base manager too when they have diverged, and counts
  the pumps only after that (`internal/testutil/main.go:73-93`).
- No test in any of the six packages assigns `vtui.FrameManager` directly, so
  the orphaned manager did not come from there.
- The goroutine profile says `runnable`, not `blocked`: the pump was working, so
  it started *after* the shutdown began.
- It does not reproduce locally. Five attempts: four seeds on the tree as it was,
  and CI's own seed `1788905440127742299` on the tree as it is.
- `vtui`'s `stopTaskPump` joins its goroutine through `taskWG.Wait()`, so the
  pump that leaked belongs to a manager whose `finishShutdown` never ran. The
  guard there is `shutdownDone`, and `Init` resets it.

*Hypothesis, untested:* four editor test files start goroutines that read
`vtui.FrameManager.TaskChan` by hand — `editor_search_lazy_test.go:32`,
`editor_find_all_test.go:170`, `buffer_async_test.go:236` and `:410`,
`editor_view_test.go:3790` — and the same files call `Init` directly. A drain
goroutine outliving its test could eat the shutdown signal, and the next `Init`
would raise a second pump. Its check is also its fix: give each of those
goroutines a done channel and a `t.Cleanup` that joins it, and see whether CI's
seed stops catching it.

---

### Two data races the detector found, both fixed

Neither is this branch's doing, and neither had been seen before: the race
detector runs `-shuffle=on` with a fresh seed, so a race that needs a particular
interleaving surfaces when it surfaces.

**`internal/editor`: `Close` cancelled the highlighter and did not wait for it.**
`EditorView.Close` cancels `highlightCancel` and then calls
`BaseFrame.Close`, which writes the field `IsDone` reads; the highlighting
goroutine asks `IsDone` between slices. Two lines above, the indexing goroutine
is joined with `indexWG.Wait()` — the highlighter simply had no equivalent, and
the asymmetry is the bug. It has a `highlightWG` now, waited in the same place.
Caught on run 34298174157, `TestEditor_StatefulHighlighting_DynamicCatchUpSpeed`;
not reproducible locally on five seeds including CI's own, green on five after.

**`internal/fileops`: a background toast read the frame manager a test was
replacing.** `Enqueue` spawns a goroutine that sleeps 500 ms and then may
announce "operation went to the background"; the toast reads
`vtui.FrameManager`. A test that enqueues, finishes inside that half second and
swaps the manager in its cleanup races that read. `QueueShowToast` is a seam
whose comment already said "a test silences it" — and `internal/fileops` had no
`TestMain` to do it, while `internal/app`'s does. It has one now. Twenty
consecutive `-race -shuffle=on` runs clean after, from one in four before.

The second is a test-side race and the first is not: `BaseFrame`'s field is read
and written from two goroutines in ordinary operation, and only the editor's
side of it is ours to fix.

---

### Two CI-only test failures, cause not identified

The first full matrix run after Phase 4 failed two test cells that pass locally
on darwin/arm64, in isolation and in a shuffled full-suite run:

- `Test (linux/amd64)` — `TestMainMenuFilePath_HasExpectedSuffix`:
  `MainMenuFilePath()="settings/user_menu.ini"`, want suffix
  `"f4/settings/user_menu.ini"`.
- `Test (darwin/amd64)` — `TestMacKeysSkipsCommandRulesWithoutTheChannelSplit`:
  `mackeys_test.go:109: Opt+Left was not rewritten`.

Neither test, and neither subject, was touched by this branch. The phase-3 run
was green, which narrows the window to Phase 4 or the upstream merge inside it —
but CI runs with `-shuffle=on` and a fresh seed each time, so a green run is not
evidence that an order-dependent test is healthy.

The lead worth following first: `MainMenuFilePath` returned a path built from an
**empty** config directory, and no branch of `resolveProfileDir` can return `""`
— the system branch yields at least `"f4"`, the portable branch at least
`"Profile"`. An empty `cachedF4ConfigDir` is what a reader observes between
`setupPortableIni`'s cleanup replacing `configDirOnce` and the next `Do`
completing (`portable_paths_test.go:44-49`, `config.go:118`). That is a race
between a fixture's teardown and any concurrent reader, and shuffling changes
which tests are adjacent enough to hit it.

The next full matrix run, on a tree that differed only by the 32-bit fixes to
`internal/numeric`, was **green on all 34 cells** — neither cell failed again.
Two failures that do not reproduce on the following run are consistent with a
seed-dependent order sensitivity and inconsistent with a deterministic
regression, which is the reading the evidence above already pointed at.

Not closed, though: a flake that fails one run in two is still a flake, and on a
branch this size it will be read as "the restructuring broke something". Next
step, in order: run `go test ./cmd/f4 -shuffle=<seed>` over several seeds on this
branch and on `upstream/main`, and compare. If it reproduces on `upstream/main`
it is pre-existing, and it belongs in the PR body as a known flake rather than in
a fix here.

---

## Commit Plan

Thirty-nine commits. Task 0 is a rebase, Task 39 a measurement, and Task 43 a
classification recorded in this bundle rather than in the tree; none produces one.

| # | Tasks | Message |
|---|---|---|
| 1 | 1 | `test: record the pre-restructuring test baseline` |
| 2 | 2 | `test(palette): key the coverage auditor by qualified symbol` |
| 3 | 3 | `refactor(actions): make registration order explicit` |
| 4 | 4 | `refactor(config): give F4Config's field types a home in config.go` |
| 5 | 5 | `refactor(queue): start the worker from the root, not from init()` |
| 6 | 6 | `refactor(drives): lift the drive registry off the panels frame` |
| 7 | 7 | `refactor(sysinfo): return a message key from the GPU probe` |
| 8 | 8 | `test(arch): add the module boundary auditor` |
| 9 | 9 | `test: give the shared frame harness a home` |
| 10 | 10-12 | `chore: move scripts, media and prose out of the repository root` |
| 11 | 13 | `docs: name the issue reviews by their subject` |
| 12 | 14 | `refactor(colorer): move the colour scheme to its consumer` |
| 13 | 15 | `refactor(plugring): spell the catalogue URL once` |
| 14 | 16 | `refactor: move piecetable, textlayout and sheet under internal/` |
| 15 | 17 | `refactor: move fusefs, vtvibe and luaplug under internal/` |
| 16 | 18 | `refactor(actions): split the registry mechanism from its table` |
| 17 | 19 | `refactor: extract internal/numeric` |
| 18 | 20 | `refactor: extract internal/toast and internal/history` |
| 19 | 21 | `refactor: extract internal/action` |
| 20-33 | 22-35 | one per wave: `refactor(<pkg>): extract internal/<pkg> from cmd/f4` |
| 34 | 36 | `refactor(app): extract the composition root` |
| 35 | 37 | `refactor: reduce cmd/f4 to the entry point` |
| 36 | 38 | `ci: rebalance the shards for the split tree` |
| 37 | 40 | `docs: update the structural map and the subsystem pages` |
| 38 | 41 | `docs(ai-factory): describe the architecture as built` |
| 39 | 42 | `chore: drop the migration baseline` |

## Definition of Done

- `cmd/f4` holds `main.go`, the four module-wide auditors
  (`command_palette_coverage_test.go`, `architecture_test.go`,
  `frame_manager_capture_test.go`, `hardcoded_strings_test.go`), the five-line
  `TestMain` they need, and `rsrc_windows_{amd64,arm64}.syso`. Nothing else: no
  test of the wiring exists today, and one written later lives here too.
- Every package in `ARCHITECTURE.md`'s target tree exists, and every package that
  exists is in the document.
- `go test ./cmd/f4 -run '^TestArchitecture'` passes with all five rules active
  and **no exemptions** — including rule 3, that nothing below layer 4 imports
  `internal/app`, and the sysinfo leaf rule.
- `command_palette_coverage_test.go` passes with 42 keys, none containing a path.
- The full 26-target build matrix is green, `CGO_ENABLED=0`, exotic targets and
  the `noffi` tag included.
- `go test -timeout 25m ./...` plus the five other modules match the Task 1
  baseline's green set. The three pre-existing failures — `tools/icons`,
  `tools/wine_syscall_probe` on darwin/arm64, the inert
  `plugring_policy_test.go` — are unchanged and recorded somewhere that outlives
  this branch.
- No document describes the migration. `ARCHITECTURE.md` describes the tree;
  `AGENTS.md` and `.ai-factory/rules/base.md` describe the packages that exist,
  with counts re-derived rather than estimated.
- The pull-request body states, in the author's own words rather than buried in a
  diff: the README image URL being branch-scoped and 404 until merge; the
  incremental-lint finding count measured against `origin/main`; and the three
  pre-existing failures.
- Nothing in this branch changes anything a user outside the repository can
  observe. The one candidate — moving `plugring/` and with it a published URL —
  was considered and dropped: the catalogue is a submission surface, not a
  compiled plugin, and its URL is a contract. The pull request says so, so that
  the question is answered rather than left for a reviewer to ask.
