# Handoff — where this stands

Everything here is either a thing that must happen first or a thing that lives
nowhere else.

## Where this stands

Written at `f59248f4`, on a clean, green tree, level with `upstream/main`
(`git rev-list --count HEAD..upstream/main` reports 0). Two merges brought in
27 upstream commits; both are recorded in `index.md`'s Open Findings, along with
the live bug the first of them fixed.

**The run on `7d7f5309` was read late, and it was red for two reasons, neither
of them `affected.calc`.** Build failed the `gofmt -s` gate on two files that
were misformatted from birth — the local gate runs `gofmt -w`, CI runs
`gofmt -s -l`, and they are not the same command. Both were reformatted in
passing by `f59248f4`, so that half is fixed. Race failed a goroutine-leak check
in `internal/editor` after every test passed, and does not reproduce locally
over four seeds. Both are written up in `index.md`, together with the misreading
that sent the first diagnosis to `affected.calc`: the last line a failing step
echoes is its group header, not its last command.

Run **34295760690** on `b4721d48` is **green on every cell**, read at the commit
that followed it. That is the first full-matrix confirmation the branch has had
since the terminal wave, and it covers the largest commit on it: 252 files, 70
renames, `internal/panel` under the race detector, and `Test (windows/arm64)`
and `Test (windows/amd64)` on the seams that only Windows compiles.

The editor task-pump leak did not reproduce — `Race (packages)` passed — so it
stays a flake with the other two, and `internal/panel` did not add one of its
own despite 156 direct `FrameManager.Init` calls (see Task 44 step 7).

## Where the work stands

**Every task is done.** Phases 1 through 11 are closed, the checkboxes in
`index.md` match the tree, and what remains is not a task but a decision: when
to open the pull request. `cmd/f4` is **five files** — `main.go` and the four
module-wide auditors — down from 691; `internal/app` holds 234,
`internal/panel` 80, and there are 40 packages under `internal`.

The body of the pull request is written and sits in `PR-BODY.md` beside this
file. It is prepared, not sent: `gh pr create` is the user's call, and Task 45
step 13 says what to re-measure in the minute before it runs.

Five upstream merges have landed; `HEAD`, `origin/main` and `upstream/main` all
agree. The full matrix is green on the current head — run 34312085495, 34 cells,
zero failures — and that run is the first one to see the last twenty commits.

**`origin/main` is level with `upstream/main`**, so the incremental lint runs
against a real base. Re-measured on it: still **0**. The zero held on rename
detection, not on the base lagging.

Everything that waited for Task 34 is closed. `text_editor_bridge.go` and
`visren_editor_bridge.go` are `internal/panel/bridge_texteditor.go` and
`bridge_visren.go`; `panel_lookup.go` is `internal/panel/lookup.go`;
`fuse_mount_*.go` are `fuse_mount.go` and `fuse_list.go`; `cmd/f4/semantic.go`
split, with the frame's half in `internal/panel/frame_semantic.go`; and the two
`semantic_fields.go` copies are gone into `internal/semantic`.
`TestActionOrderIsStable` passes, so the menu did not move.

## Deviations from the plan, and where each is recorded

Every one is written into the task it belongs to. This list exists so the next
session can check the record rather than rediscover it.

| Deviation | Recorded in |
|---|---|
| `internal/plughost` interface is seven methods, not five | `index.md`, the Task 26 finding |
| `api.go`, `plugin_hotkeys.go`, `plugring_ui.go`, `sqlite_actions.go` stay behind | `phase-06`, "What the wave actually found" |
| `internal/gui` is layer 2, not 1 | `phase-06` |
| `internal/term` → `internal/terminal`, layer 3 not 1 | `phase-07`, commit `a1da963e` |
| `internal/media` is layer 3, not 1 | `phase-07` |
| Nine `term` roster files could not move | `phase-07` |
| `internal/textsearch` and early `internal/fileops`, neither planned | `phase-07` |
| `fuse_mount_*.go` go to `internal/panel` (Task 34) — they read `fsp.vfs` and `pf.getActivePanel` | `phase-08`, Task 32 |
| `attributes_dialog.go` goes to `internal/dialog`, not `fileops` | `phase-08`, Task 32 |
| `commands.go` cannot go to `internal/cmdline` | `phase-07` |
| `hotkeys.go` did move after all — the manager to `internal/keymap`, its conditions to `internal/panel` | `phase-09`, "What the wave actually found" |
| `plugin_hotkeys.go` goes to `internal/panel`, not `internal/app` — it follows `HotkeyManager` | `phase-09` |
| The plugin menu and global-hotkey registries go to `internal/plughost` | `phase-09` |
| 36 panel tests stay in `cmd/f4`: they need the action table, which no seam can supply | `phase-09` |
| `app.Main()` instead of the `New`/`Run(ctx)` contract — the signature would assert an injection that 4490 global reads contradict | `phase-10`, "Deviation"; obligations in `phase-11` Task 41 step 1a and Task 44 step 2 |
| `PlayerPanel` goes to `internal/panel`; `grabber.go` and `share_dialog.go` stay in `app` | `phase-10`, "What the wave actually found"; decided in `phase-11` Task 44 step 4a |
| `cmd/f4` has no `TestMain`: the auditors parse files and build no frame | `phase-10` |
| Package-name question for Task 44 | `phase-11`, Task 44 step 3 |

## Open tails

1. **`internal/panel/panels_frame_test.go` and its four neighbours are still
   mixed.** The 36 tests that needed the action table were moved out one at a
   time, by running them and watching which failed. That found every test that
   *fails* without dispatch; it cannot find one that passes for the wrong reason
   — a test asserting "nothing happened" passes with an inert seam whatever it
   is really testing. The honest split is by what each test asserts, and nobody
   has read them one by one.
2. **`internal/paneltest` duplicates two helpers into `internal/panel`**, marked
   `ponytail:` at both copies.
   `frame_manager_test_helpers_test.go` and the mocks the moved tests share
   exist on both sides, because an in-package test cannot import a package that
   imports it. Twenty lines of scaffolding; the alternative is making every
   panel test an external test package, which the doc comment on
   `internal/paneltest/doc.go` already contemplates.
3. **`internal/editor/view.go`'s `saveUndo` op classes stay private.** The one
   external caller gets `Checkpoint()` instead. If a second appears, the enum is
   the thing to export, not another method.
4. **The darwin mackeys flake and the two CI-only failures** in `index.md`'s
   Open Findings are untouched by this session.

## Plan-versus-tree discrepancies seen and not acted on

- Task 43's roster assigns `coreAPI` to `internal/plughost` in nine test rows.
  The Task 26 decision moved `api.go` to `internal/app` instead, so those rows
  are stale. Harmless — they describe exports that turned out unnecessary — but
  a reader will trip on them.
- The plan's layer numbers for `gui` (1), `term` (1), `media` (1) and
  `fileops` (1) were assigned before the edges existed. Three are corrected in
  the auditor; `fileops` is still 1 and is still true.

## What the extraction gate does not ask

The gate counts what a file **references**. It says nothing about what a file
**declares** — and a function parked in a moving file travels with it silently,
away from callers that stay behind. Task 32 found seven of them in one wave, and
three were not cosmetic: `padLabel`, `ThemedForeground` and `UseTableColors`
made `internal/fileops` import `internal/dialog`, a layer 1 → layer 3 edge, and
with the attributes dialog pointing back it was a cycle. The compiler caught
that one only because the edge happened to be mutual.

So the wave procedure gains a backward pass, run **before** the move: for every
declaration in the files being moved, where are its callers, and are they going
to the same package?

```sql
WITH decl AS (SELECT id, name, file_path FROM nodes WHERE file_path IN (<wave files>))
SELECT d.name, s.file_path AS caller
FROM edges e JOIN nodes s ON s.id = e.source JOIN decl d ON d.id = e.target
WHERE e.kind IN ('calls','references') AND s.file_path NOT IN (<wave files>);
```

Three things about that index, all measured: run `codegraph sync` first, because
it lags commits and answers about yesterday's tree without saying so;
`is_exported` is 0 for every method, constant and variable regardless of case,
so filter on the first letter instead; and struct fields are not in the model at
all — `fsp.vfs` does not appear — so the query names candidates and grep
confirms them.

## Fourteen files placed by name, and what the graph said

The count is worth keeping because it is the branch's most reliable finding: a
file's name is evidence, and the graph is the verdict. `kitty_*` (media by name,
terminal by graph), `command_runner*` (cmdline, terminal), `colors.go`,
`attributes_dialog.go` (fileops, dialog), `fuse_mount_*` ×2 (fileops, panel),
`async_buffer.go` (app, editor), and then five at once in Task 35's roster —
`cmd_session.go`, `apply_command.go`, `simple_exec.go`,
`command_prefix_registry.go`, `remote_command.go`: all named for the command
line, all reading private members of the panel types, all `internal/panel`.

Two of the fourteen came from blocks written to *correct* the roster, which is
the part worth remembering: a correction goes stale like the thing it corrects,
and the check that catches it is the same one — measure before moving.

## Five mechanical traps, each hit at least once

**The literal multiset is the only check that catches a rewritten string.** Say
it plainly, because three other checks were running and all three said nothing.
A rename script that rewrites a string literal produces code that compiles, runs
and passes: `"panel.activate"` became `"activate"` in five semantic action names
on the panel wave — the eleventh case of this damage — and the build, the whole
test suite and `golangci-lint` were green over it. What sees it is the
before/after count of every literal in the changed files, renames resolved with
`git diff --name-status -M` so a moved file is compared against its old path
rather than against nothing. Run it before the tests, not after: a green suite
is not evidence, and once the diff is committed the comparison is harder to make.

**Both obvious ways of listing paths for a pointed commit are wrong, in
opposite directions.** `git diff --cached --name-only` prints only the *new*
path of a rename, so the old one stays in the tree and the commit holds both
copies — it does not compile. `git status --short | awk '{print $2}'` prints
only the *old* path, so the new files are left out of the commit entirely. One
wave hit each. The form that works:

```
git commit --only $(git status --short | sed 's/^...//' | sed 's/ -> /\n/' | tr '\n' ' ') -F <msg>
```

The rule exists so a commit builds and takes nothing of anyone else's; both
shortcuts broke the first half of that silently. `git ls-tree HEAD <old path>`
is what tells you afterwards, and `git commit --amend --only <all paths>` is the
repair.

**A compiler-driven rename loop must never rewrite bare identifiers.** Rewrite
selectors (`.name`) and declarations bound to a named receiver; leave everything
else alone. `qual2.py` and a loop written for Task 33 both ignored that and
wrecked packages that merely shared a name — `closeOnce` became
`fileops.CloseOnce` across four `internal/terminal` files, and `vfs`, the
*package qualifier*, became `Vfs` in 217 files at once. `exportmethods2.py` is
the shape that works, and it takes the receiver names for exactly this reason.

Two tells are worth knowing because they are what actually bites. A loop that
flips between two spellings has found two types sharing a field name — stop it
and resolve by hand; that happened four times in one wave (`vfs`, `indexWG`,
`showSearchDialog`, `showCodepageDialog`). And a loop keyed on `git ls-files`
cannot see the files the wave just created, so it spins without converging:
walk the tree instead.

**Cut test functions with `go/parser`, not with a regexp.** A regexp that finds
a function's start by scanning backwards for a blank line eats the previous
function's closing brace, and the result is still valid Go — a test that
silently moved to the wrong file, or vanished with its assertions. The compiler
cannot see it. Task 33 lost three test tails and five whole tests that way, and
only the literal-diff check plus a comparison against the previous revision
found them. One parser costs less than that comparison did.

**Deleting a line from the palette auditor's target map and adding the package
to the layer map is one operation, not two.** The auditor counts f4's own
surfaces from *both* maps — a file taken out of the first and not entered in the
second stops being counted, silently. On the cmdline wave the count fell from
42 to 40 and that was the only sign. Same family as a sweep that finds nothing
and a test that passes by never dispatching: a check that quietly starts
measuring less than it should.

**The palette auditor's target map empties itself, and a wave that forgets its
line leaves litter.** `commandPaletteTargetPackage` forward-declares where each
`cmd/f4` file will land so audit keys survive the move; the wave that moves a
file deletes its entry, at which point the directory gives the same answer.
Three entries were stale when Task 32 looked — `codepage_settings.go`,
`macro.go` and its own `queue_manager.go` — so check the whole map rather than
only the file you moved:

```
sed -n '/^var commandPaletteTargetPackage/,/^}/p' cmd/f4/command_palette_coverage_test.go \
  | grep -oE '"[a-z_0-9]+\.go"' | tr -d '"' \
  | while read f; do [ -e "cmd/f4/$f" ] || echo "stale: $f"; done
```

**Copying a type with a regexp takes the struct and leaves its methods, and
the result compiles.** Moving upstream's attributes tests into `internal/dialog`
needed a copy of `mockMetadataVFS`; the pattern matched the `type` block and not
the two methods below it. `mockMetadataVFS` embeds `vfs.VFS`, so `Stat` and
`SetAttributes` fell through to the embedded interface, the package built, and
the test reached the real filesystem and hung on a two-second timeout instead of
failing on a missing method. The compiler cannot see this one; only the test did.
Copy a type with the parser, the way declarations are cut.

**`gofmt -w` is not the check CI runs.** CI runs `gofmt -s -l .` and fails on
any output; `-s` is the simplify pass, and a file it would rewrite is a file
`gofmt -w` leaves alone. Two files sat misformatted from the day they were
written and no local gate said so, because every wave ran `gofmt -w` on what it
touched and never asked the question CI asks. `gofmt -l` did list both during
the panel wave and the listing was dismissed as "they parse" — which was true
and beside the point. The gate line is:

```
gofmt -s -l . | grep -v '^vendor' && echo "unformatted"
```

## A test whose subject is a binding belongs with the table

`internal/editor` reaches the action layer through a seam, and the registry
behind it is filled by `action_table.go`'s `init` in `cmd/f4`. Nineteen tests
that press a key and expect an action would therefore have passed in the
editor's package **by finding nothing to do** — green because the registry was
empty. They stayed with the table.

**And the wrapper is the material form of the rule.** In `cmd/f4` such a test
presses through `pressKey`, which installs `GlobalHotkeysMgr` and
`macro.MacroMgr` and passes the real macro filter. `testutil.PressKey` with a
`nil` filter skips the dispatch entirely, so the test measures the widget's own
key handling and goes green on every platform where the widget happens to
handle that key. Nineteen tests were moved for the action layer and then
converted to the nil form in the same wave; two platforms disagreed about
whether the widget handled the key, which is the only reason it surfaced.

Checked against the waves already done, and the class had not fired before:
`internal/*` holds two `RunAction` occurrences and both are mock methods, no
test there names `LookupHotkey`, and `RegisterAction` is called only from
`cmd/f4`. Every other key-pressing test drives the widget's own `ProcessKey`,
which is local. Tasks 34 and 35 sit next to the table and will meet it again.

## Tools

Live in the session scratchpad and are copied to the shared location for the
next session. `xbuild.sh` there was broken — `| head` swallowed `go build`'s
exit status, so every target printed OK — and is replaced; it now also runs the
`gofmt -s -l` gate CI runs.

The panel wave added six, every one of them go/types or go/parser rather than a
regexp, and every one because a text tool had already got the same job wrong.
Two rules came out of using them, and both are cheap to forget:

**A Go error's column is a byte offset.** A line with a Russian comment ahead of
the selector has fewer characters than bytes, so an edit made at that index in a
Python string lands in the wrong place — or, more often, finds nothing and the
loop spins reporting progress it did not make. Edit bytes.

**`go/parser`'s object resolution is what separates a local from a qualifier.**
An identifier carrying an `Object` was declared in this file; one without it is
the package name. No regexp can make that distinction, and both `stripqual.go`
and `unshadow.go` exist because one tried.

| Script | What it does |
|---|---|
| `xbuild.sh` | ten cross-build targets, fixed status check, `-o /dev/null` |
| `baseline.sh` | the six-module baseline run |
| `qual2.py` | the compiler-driven export-and-qualify loop |
| `exportmethods2.py` | export by receiver name, skipping string literals |
| `exportsyms.py` | export one package symbol the compiler says is undefined |
| `fixselectors.py` | follow the compiler's "but does have" hints |
| `literalkeys.py` | composite-literal keys — **oscillates when two types share a field name; watch it** |
| `unexport.py` | undo an over-export in a test that moved into its package |
| `splittests.py` | split the tests naming a symbol back to `cmd/f4`; appends |
| `addimports.py` | add the imports the compiler names |
| `exporter/` | rename a package's unexported members to their exported spelling, and `-down` back again, by **object identity** — a name shared by two types is renamed only on the type that owns it. Package-level declarations, struct fields and methods. |
| `allerrs/` | type-check a package **with its test files** and print every error. `go vet` stops at the first type error in a test file, which turns a 3000-error mechanical rename into 3000 full builds. |
| `stripqual.go` | drop a package qualifier from a file moved into that package, skipping a selector whose `X` resolves to a local of the same name |
| `unshadow.go` | rename a local that shadows an imported package name |
| `drive.py` | the loop that drives all of the above from the compiler's own errors: selectors, struct-literal keys, missing and dead imports, qualification |
| `decls.go` | every top-level declaration of a package, block members included |
| `deadimports.py`, `verify-commits.sh` | as inherited |

**Three of these caused damage this session, all of it the same class.** A
`name:` → `Name:` repair rewrote three lines of a YAML fixture inside a raw
string; a `path` → `Path` export and its revert rewrote `case "Path":` in two
ini readers, which compiles and reads a key the user's file does not have. Only
round-trip tests caught them. `exportmethods2.py` skips string literals now.
The rule that actually holds: **grep the wave's own diff for changed string
content before running the suite, not after.**

```
git diff HEAD -M -- cmd/f4 internal | grep -E '^[+-]' | grep '"'
```
