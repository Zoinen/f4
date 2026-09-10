# Phase 2: Clear the Repository Root

Plan: [index.md](index.md)
Tasks: 10-15
Depends on: Phase 1

## Objective

The repository root holds entry points, the plugin contract, and the reference
configs a reader is pointed at — nothing else. A newcomer reaches `README.md`
without scrolling past scripts, screenshots and loose issue write-ups.

This phase is independent of the Go package work and produces a noisy rename diff,
so it lands alone, before any package moves. Closes issue #505.

## Current-Code Evidence

| Path | Symbols / lines | Why it matters |
|---|---|---|
| `filelist_update.sh`, `test_plugins.sh`, `test_resurrect.sh` | — | The three root scripts |
| `tools/test_runner.sh:7` | `./filelist_update.sh` | The only in-tree caller of a root script |
| `README.md:3` | `https://raw.githubusercontent.com/unxed/f4/refs/heads/main/screenshot.png` | Absolute remote URL, not a relative path |
| `AGENTS.md:74` | `` | `SPREADSHEET.md` `` | The docs table entry |
| `time.txt` | 3 lines, 6 bytes | Referenced by nothing |
| `docs/ISSUES/` | 43 files | 3 `_FOLLOWUP_SOLUTION_REVIEW`, 1 already slugged, the rest plain `_SOLUTION_REVIEW`; upstream added two while this branch was in Phase 1 |
| `docs/ISSUES/ISSUE_91_FREEBSD_CONSOLE_DIAGNOSIS.md` | — | The existing precedent for the target naming |
| `docs/ISSUES/ISSUE_95_FOLLOWUP_SOLUTION_REVIEW.md`, `docs/ISSUES/issue-703-solution.md` | — | Upstream moved both out of the root itself; they now collide in subject with their neighbours there |
| `DISPATCH.md` (root) | 3 lines | Upstream's live dispatch note; the LUNOBOT notes beside it went to `docs/LUNOBOT/` and this one did not |
| `colorer/configs/base/hrd/rgb/radiola.hrd` | 1 file | The entire `colorer/` tree |
| `embedded.go:12` | `//go:embed colorer/…/radiola.hrd` | Root package's second embed |
| `cmd/f4/plugring.go:48-49` | dev fallback | Compares against that same literal URL |
| `plugring/index.yaml:7` | `url:` | Points at its own neighbour `hello_plugring.lua` |
| `cmd/f4/plugring_test.go:40` | `runtime.Caller` → `../../plugring/index.yaml` | Works today; breaks on the move |
| `cmd/f4/plugring_policy_test.go:40` | CWD-relative read | Always `t.Skipf`; asserts nothing |

## Files to Change

| Path | Action | Required change |
|---|---|---|
| `scripts/` | create | Destination for the three root scripts |
| `tools/test_runner.sh` | modify | Line 7 path |
| `.github/assets/` | create | Destination for `screenshot.png` |
| `README.md` | modify | Image URL (line 3) |
| `docs/SPREADSHEET.md` | create (move) | From the root |
| `AGENTS.md` | modify | Docs table path |
| `time.txt` | delete | Empty file, no references |
| `docs/ISSUES/*.md` | rename ×40 | Slug instead of the constant tail |
| `internal/colorer/` | create | `radiola.hrd` + its own embed |
| `embedded.go` | modify | Loses the second embed, keeps `README.md` |
| `cmd/f4/colorer_plugin.go` | modify | Reads the new symbol (`:156`) |
| `cmd/f4/colorer_plugin_test.go` | modify | Second consumer (`:56`) |
| `plugins/plugring/` | create (move) | From `plugring/` |
| `cmd/f4/plugring.go` | modify | Catalogue URL and dev fallback |
| `cmd/f4/plugring_test.go` | modify | Relative path from `runtime.Caller` |
| `cmd/f4/plugring_policy_test.go` | modify or delete | Explicit decision required |

---

## Task 10: Move the shell scripts to `scripts/`

### Intent

Three scripts sit in the root purely by history. They are developer tooling, and
the root is for entry points.

### Implementation Steps

1. `mkdir -p scripts`
2. `git mv filelist_update.sh test_plugins.sh test_resurrect.sh scripts/`
3. Update `tools/test_runner.sh:7`: `./filelist_update.sh` →
   `./scripts/filelist_update.sh`. Check the working directory the runner assumes —
   if it runs from the repository root, the new relative path is correct as
   written; if it `cd`s first, adjust accordingly.
4. Sweep for any other invocation:
   `grep -rn 'filelist_update\|test_plugins\.sh\|test_resurrect\.sh' . --exclude-dir=.git`
   and fix every hit, including `.github/workflows/` and `docs/`.
5. Preserve the executable bit — `git mv` does, a copy-and-delete does not.

### Required Interfaces and Contracts

- File modes unchanged (`0755` on all three).
- No script's content changes; only `tools/test_runner.sh` is edited, and only its
  path literal.

### Error Handling and Logging

None. Shell scripts, no product code.

### Tests

No new tests. The scripts are not covered by the Go suite; the grep in step 4 is
the check.

### Acceptance Criteria

- `ls *.sh` in the root returns nothing.
- `ls -l scripts/` shows all three with the executable bit set.
- The grep in step 4 returns no hit pointing at a root path.

### Verification

- `bash -n scripts/filelist_update.sh scripts/test_plugins.sh scripts/test_resurrect.sh`
- Expected result: exit 0 — syntax intact after the move.
- `grep -rn './filelist_update.sh' . --exclude-dir=.git`
- Expected result: no output.

---

## Task 11: Move `screenshot.png` to `.github/assets/`

### Intent

A 500 KB screenshot in the root is the second thing a browser of the repository
sees. Media belongs in `.github/assets/`.

### Implementation Steps

1. `mkdir -p .github/assets && git mv screenshot.png .github/assets/`
2. Update `README.md:3`. The current reference is an **absolute remote URL**:
   `https://raw.githubusercontent.com/unxed/f4/refs/heads/main/screenshot.png`.
   Change the path component to `.github/assets/screenshot.png`, keeping the
   `refs/heads/main` form so the README renders the same way on GitHub.
3. Note the consequence explicitly in the pull-request body: the new URL resolves
   only once this branch merges into `main`, so the README image is broken on the
   branch and on the PR preview. This is expected and unavoidable for a
   branch-scoped raw URL — the alternative (a relative path) changes how the image
   renders outside GitHub.
4. Sweep for other references:
   `grep -rn 'screenshot.png' . --exclude-dir=.git`.

### Required Interfaces and Contracts

- The rendered README on `main` after merge must show the image. Nothing else
  depends on the path.

### Error Handling and Logging

None.

### Tests

No new tests.

### Acceptance Criteria

- `ls screenshot.png` fails; `ls .github/assets/screenshot.png` succeeds.
- `README.md` contains no reference to a root-level `screenshot.png`.
- The PR body mentions the temporary 404.

### Verification

- `grep -n 'screenshot.png' README.md`
- Expected result: exactly one line, containing `.github/assets/screenshot.png`.

---

## Task 12: Move the loose prose into `docs/`, and delete `time.txt`

### Intent

Three markdown files and one empty text file sit in the root. Prose belongs in
`docs/`; `time.txt` belongs nowhere.

### Implementation Steps

1. `git mv SPREADSHEET.md docs/`. Update `AGENTS.md:74` (the Documentation table
   row) and sweep `grep -rn 'SPREADSHEET.md' . --exclude-dir=.git`.
2. `ISSUE_95_FOLLOWUP_SOLUTION_REVIEW.md` and `issue-703-solution.md` no longer
   sit in the root: upstream moved both into `docs/ISSUES/` while this branch was
   in Phase 1. What remains of this step is the collision they were moved into,
   which is Task 13's subject and not this one's. Each has a counterpart already
   in `docs/ISSUES/`
   (`ISSUE_95_SOLUTION_REVIEW.md`, `ISSUE_703_SOLUTION_REVIEW.md`). Read both pairs
   and decide per pair, then record the decision in the commit message:
   - if the root file is a genuine follow-up, move it as
     `docs/ISSUES/ISSUE_95_<SLUG>_FOLLOWUP.md` — the repository already uses that
     shape (`ISSUE_546_FOLLOWUP_SOLUTION_REVIEW.md`,
     `ISSUE_816_FOLLOWUP_SOLUTION_REVIEW.md`);
   - if it duplicates the existing review, fold the new material into the existing
     file and `git rm` the root copy.
   Do not move both into `docs/ISSUES/` under names that leave a reader unable to
   tell which is current.
3. `git rm time.txt` — three blank lines, six bytes, referenced by nothing
   (`grep -rn 'time\.txt' . --exclude-dir=.git` confirms before deleting).
4. `f4.example.ini` and `highlight.ini` **stay**: they are reference configs the
   README points at, and the target layout keeps them beside it.
5. `DISPATCH.md` **stays**, untouched. It arrived from upstream after this plan
   was written and is the maintainer's live dispatch note — three lines saying who
   is holding which ticket. Upstream has already moved its LUNOBOT siblings into
   `docs/LUNOBOT/` and left this one where it is, which is a decision, not an
   oversight. Relocating another author's live state manufactures a conflict in
   the pull request for no gain.

### Required Interfaces and Contracts

- After this task the root contains, in addition to directories: `README.md`,
  `LICENSE`, `go.mod`, `go.sum`, `embedded.go`, `f4.example.ini`, `highlight.ini`,
  `AGENTS.md`, `skills-lock.json`, upstream's `DISPATCH.md` and the dotfiles.
  Nothing else.
  `AGENTS.md` and `skills-lock.json` are harness metadata that ships in this pull
  request deliberately (see `index.md`'s Delivery note); they are not loose prose
  and this task does not move them.

### Error Handling and Logging

None.

### Tests

No new tests. Step 3's grep is the check for `time.txt`.

### Acceptance Criteria

- The root file list matches the contract above exactly.
- No dangling link to `SPREADSHEET.md` or to either root issue document.
- The commit message records the decision made in step 2 for each pair.

### Verification

- `git ls-files -- ':(exclude)*/*' | grep -v '^\.'`
- Expected result: exactly `AGENTS.md LICENSE README.md embedded.go f4.example.ini
  go.mod go.sum highlight.ini skills-lock.json`, plus upstream's `DISPATCH.md`.
  Use `git ls-files`, not `ls`: `.gitignore:3` ignores `/f4`, so on any machine
  that has run `go build ./cmd/f4` a plain `ls` also lists the built binary and the
  check fails for a reason that has nothing to do with this task.
- `grep -rn 'SPREADSHEET.md\|issue-703-solution\|ISSUE_95_FOLLOWUP' . --exclude-dir=.git --include='*.md' --include='*.go' --include='*.yml'`
- Expected result: only hits pointing at the new `docs/ISSUES/` locations.

---

## Task 13: Rename the issue reviews to `ISSUE_<number>_<SLUG>.md`

### Intent

Finding the review of a subject currently requires already knowing its issue
number: 40 of the 41 files are distinguished only by digits. The 41st,
`ISSUE_91_FREEBSD_CONSOLE_DIAGNOSIS.md`, already carries a slug and is the
precedent the convention formalizes. `SOLUTION_REVIEW` carries no information —
every file in the directory is a solution review.

### Implementation Steps

1. For each of the 42 files that still end in `_SOLUTION_REVIEW.md` or carry a
   root-era name, read the
   document and derive a SCREAMING_SNAKE slug from its actual subject —
   `ISSUE_165_SORT_GROUPS.md`, `ISSUE_546_CONPTY_FOLLOWUP.md`. The slug replaces
   the constant tail; the number stays first so numeric ordering survives.
2. The two follow-ups (`ISSUE_546_FOLLOWUP_SOLUTION_REVIEW.md`,
   `ISSUE_816_FOLLOWUP_SOLUTION_REVIEW.md`) keep a `_FOLLOWUP` marker in the slug
   so the pair with the base review stays visible:
   `ISSUE_546_CONPTY_FOLLOWUP.md` beside `ISSUE_546_CONPTY.md`.
3. `ISSUE_91_FREEBSD_CONSOLE_DIAGNOSIS.md` is already conformant. Leave it.
4. `git mv` each file. Then, for every old filename, sweep the whole tree and fix
   the links:
   ```
   grep -rn 'ISSUE_[0-9]*_SOLUTION_REVIEW' . --exclude-dir=.git
   ```
   Sources reference these documents too, not only `docs/` — check `.go` files as
   well as markdown.
5. Re-run the sweep after the last rename and confirm it is empty.

### Required Interfaces and Contracts

- Filename shape: `ISSUE_<number>_<SLUG>.md`, SCREAMING_SNAKE, no other
  punctuation.
- The number is the GitHub issue number and is not padded.
- One file per review; renaming never merges two documents.

### Error Handling and Logging

None.

### Tests

No new tests. The link sweep in step 5 is the check.

### Acceptance Criteria

- `ls docs/ISSUES/ | grep -c SOLUTION_REVIEW` returns `0`.
- `ls docs/ISSUES/ | wc -l` returns `43` — the 41 the directory held, plus the two
  upstream moved in from the root. No document is lost and none is merged.
- The step-4 grep returns nothing.

### Verification

- `ls docs/ISSUES/ | grep -c 'SOLUTION_REVIEW'`
- Expected result: `0`.
- `grep -rn 'ISSUE_[0-9]*_SOLUTION_REVIEW' . --exclude-dir=.git`
- Expected result: no output.

---

## Task 14: Move `colorer/` to `internal/colorer/`

### Intent

The `colorer/` tree at the root holds exactly one file, a colour scheme, embedded
by the *root* package rather than by its consumer. Moving it to its single
consumer leaves root `embedded.go` embedding `README.md` alone — the one case that
file exists for, since `//go:embed` cannot reach above its own directory and
`README.md` must stay in the root to render on GitHub.

This is the one extraction that can happen before the waves: one file, one
consumer, no callers.

### Implementation Steps

1. `mkdir -p internal/colorer && git mv colorer internal/colorer` — the tree
   already begins at `configs/`, so naming the destination `internal/colorer/configs`
   buries it one level too deep. Confirm with `find internal/colorer -type f` that
   the single file landed at `internal/colorer/configs/base/hrd/rgb/radiola.hrd`;
   that is the layout colorer4go expects and the path the embed directive names.
2. Create `internal/colorer/embedded.go`:
   ```go
   package colorer

   import _ "embed"

   //go:embed configs/base/hrd/rgb/radiola.hrd
   var RadiolaHRD string
   ```
3. Delete the `//go:embed colorer/configs/base/hrd/rgb/radiola.hrd` directive and
   the `RadiolaHRD` variable from root `embedded.go:11-12`. The file keeps its
   package doc, its `import _ "embed"`, and `ReadmeMD`.
4. Update **both** consumers — `cmd/f4/colorer_plugin.go:156` and
   `cmd/f4/colorer_plugin_test.go:56` — from `embedded.RadiolaHRD` to the new
   package's variable, adding the import to each.

   `colorer_plugin.go` already imports `colorer "github.com/unxed/colorer4go"`,
   the highlighting engine, so the name is taken and ours needs an alias there:
   `colorerdata "github.com/unxed/f4/internal/colorer"`. That reads correctly —
   the engine is the code, this package is the data it is fed. The test file does
   not import the engine and takes the package under its own name.

   Both files also lose their `embedded` import: `RadiolaHRD` was the only thing
   either took from the root package. Confirm the list with
   `grep -rn 'RadiolaHRD' --include='*.go' .`, which must return exactly three
   lines: the declaration and those two.
   Do **not** use `codegraph callers RadiolaHRD` for this — it reports "No callers
   found" for a package-level variable and would hide the test file.
5. Sweep `docs/` for `colorer/` path references (one file) and `AGENTS.md`.

### Required Interfaces and Contracts

```go
// internal/colorer
var RadiolaHRD string   // was embedded.RadiolaHRD
```

- Content byte-identical. The embed path is relative to `internal/colorer/`, so
  the directory structure under it must match the directive exactly.
- Root `embedded.go` keeps `package embedded` and `ReadmeMD`; its doc comment is
  updated to say it bridges `README.md` and only that.

### Error Handling and Logging

None. A wrong embed path is a compile error, which is the check.

### Tests

No new tests. Any existing colorer test must pass:

```
go test ./cmd/f4 -run '^TestColorer'
```

### Acceptance Criteria

- `ls colorer` fails.
- `grep -c 'go:embed' embedded.go` returns `1`.
- `find internal/colorer -name '*.hrd'` returns exactly one path.
- Syntax highlighting still resolves the Radiola scheme at runtime.

### Verification

- `CGO_ENABLED=0 go build ./...`
- Expected result: exit 0.
- `go test ./cmd/f4 -run '^TestColorer'`
- Expected result: `ok`.

---

## Task 15: Keep `plugring/` in the root, and make its two seams honest

### Intent

The catalogue was going to move to `plugins/plugring/`, on the reasoning that a
catalogue of plugins belongs beside the plugins. It does not, and the move was
reverted after it was made.

`plugins/` holds the Go packages compiled into the binary. `plugring/` holds a
community catalogue of plugins that are **downloaded at runtime from somewhere
else** — the opposite kind of thing — and it is the surface an outside
contributor is pointed at: `docs/PLUGINS.md` and `docs/PLUGRING.md` both tell a
plugin author to add an entry to `plugring/` in a fork and open a pull request.
A submission surface belongs where somebody browsing the repository sees it,
not two levels down inside the compiled plugins.

It also has a published URL. Moving it breaks that URL for every build already
in the field, and buys nothing in return.

Note while reading those two documents: they say "a workflow compiles the
frontmatter into the index f4 downloads". No such workflow exists —
`.github/workflows/` holds `build.yml` and `conpty-probe.yml`, neither of which
mentions plugring, and `index.yaml` is maintained by hand. That is a
documentation bug, not this plan's business, but it is worth reporting upstream.

### Implementation Steps

1. `plugring/` stays where it is. `PlugRingCatalogURL` and the `url:` field in
   `index.yaml` keep their published paths, and `docs/PLUGINS.md`,
   `docs/PLUGRING.md` keep theirs.
2. Spell the catalogue URL **once**. `FetchCatalog` compared a second copy of
   the literal against the variable to tell an overridden URL from the default;
   two spellings of one URL are how they drift apart. Extract
   `const defaultPlugRingCatalogURL` and initialise the variable from it.
3. Repair `cmd/f4/plugring_policy_test.go`. It reads a CWD-relative
   `plugring/index.yaml`, and the test's working directory is `cmd/f4/`, so it
   has always taken its `t.Skipf` branch: it reports as passed and asserts
   nothing. Resolve the catalogue from the test's own source path with
   `runtime.Caller`, the way `plugring_test.go` already does, and turn the skip
   into a failure — the file is shipped in the repository, so not finding it is
   a broken test rather than an absent fixture.

   Repair rather than delete: it asserts that the shipped entries meet the
   policy f4 enforces on everyone else, which is a real invariant and was
   unguarded.
4. Record `plugring/` in `AGENTS.md` and `.ai-factory/rules/base.md` for what it
   is — data, not a Go package, and deliberately in the root.

### Required Interfaces and Contracts

- The published catalogue URL does not change. Builds in the field keep
  resolving it.
- After step 2 the URL literal appears exactly once in Go code.
- `index.yaml`'s schema and contents are untouched.

### Error Handling and Logging

Unchanged. A catalogue fetch failure already surfaces through the PlugRing UI.

### Tests

```
go test ./cmd/f4 -run '^TestBundledPlugRing|^TestShippedCatalog' -v
```

### Acceptance Criteria

- `ls plugring/index.yaml` succeeds and the root still holds the directory.
- `grep -c 'raw.githubusercontent' cmd/f4/plugring.go` returns `1`.
- Both plugring tests report `--- PASS`, neither reports `--- SKIP`.

### Verification

- `go test ./cmd/f4 -run '^TestBundledPlugRing|^TestShippedCatalog' -v`
- Expected result: two `--- PASS` lines, zero `--- SKIP` lines.

---

## Phase Risks and Mitigations

- **Risk:** `git mv` of `colorer/` loses the sub-path and the embed directive
  silently matches nothing.
  **Mitigation:** an embed pattern matching no file is a compile error, and Task
  14's step 1 verifies the tree with `find` before the directive is written.
- **Risk:** Task 13's 40 renames drop a link that no grep pattern catches because
  the reference is split across lines or uses a different case.
  **Mitigation:** grep case-insensitively for the bare issue number pattern
  (`ISSUE_[0-9]*`) as well as the full old name, and re-run after the last rename.
- **Risk:** Task 12 moves a root issue document next to its near-duplicate and
  leaves a reader unable to tell which is authoritative.
  **Mitigation:** step 2 requires reading both and recording a per-pair decision in
  the commit message; a plain `git mv` of both is explicitly forbidden.
- **Risk:** a later reader re-proposes moving `plugring/` under `plugins/`,
  having seen only that both hold plugins.
  **Mitigation:** Task 15 records why it stays: `plugins/` is compiled into the
  binary, `plugring/` is a submission surface with a published URL.

## Phase Completion Checklist

- Every Task 10-15 satisfies its acceptance criteria.
- The repository root contains only the seven files listed in Task 12's contract.
- `CGO_ENABLED=0 go build ./...` and `go test -timeout 25m ./...` match the Task 1
  baseline.
- `grep -rn 'ISSUE_[0-9]*_SOLUTION_REVIEW\|screenshot.png'` finds no stale
  reference.
- `index.md` task checkboxes 10-15 are ticked.
