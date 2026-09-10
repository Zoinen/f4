# Phase 11: CI, Lint and Documentation

Plan: [index.md](index.md)
Tasks: 38-42
Depends on: Phase 10

## Objective

The build pipeline is rebalanced for a tree that no longer has one giant package,
the pull request opens with no surprise lint backlog, and the prose describes the
repository that now exists rather than the one that used to.

`ARCHITECTURE.md` gets its own task, not a line in the docs sweep, because it is
the one document that changes *genre*: today it describes a target, and after this
plan it describes a fact.

## Current-Code Evidence

| Path | Signal | Consequence |
|---|---|---|
| `build.yml:983` | `matrix: { include: [{ shard: 'cmd/f4', … }, { shard: rest, … }] }` | lint shards named by package path |
| `build.yml:1042` | `"$module/cmd/f4"\|"$module/cmd/f4"/*)` | the shard router |
| `build.yml:1056-1068` | `if [ "$SHARD" = "cmd/f4" ]` … `./cmd/f4/...` | shard-specific arguments |
| `build.yml:1262-1268` | comment: "cmd/f4's suite alone takes as long under the detector as every other package combined" | the stated reason for the split |
| `build.yml:1271-1273` | `cmd/f4 A` / `B-L` / `rest`, `run: '^TestA'` etc. | three race shards splitting one package by test-name letter |
| `build.yml:1318`, `:1326` | `github.com/unxed/f4/cmd/f4` guard and `go test -race … ./cmd/f4` | the shards' bodies |
| `build.yml:1395` | `go list ./... \| grep -Ev '^github.com/unxed/f4/cmd/f4$'` | the `packages` scope, which absorbed every migrated package automatically |
| `.golangci.yml`, `.golangci-strict.yml` | contain no paths | need no edit |
| incremental lint | `--new-from-rev=origin/main` | rename detection across a `package` clause change |
| `AGENTS.md` | 6 `cmd/f4` mentions; "687 files in one flat package main" (`:23`) | structural map, now wrong |
| `.ai-factory/rules/base.md` | "Module Structure" describes the pre-move tree | conventions file |
| `.ai-factory/ARCHITECTURE.md` | 559 lines; `(move)`/`(extract)` markers, migration policy, extraction order | changes genre |
| `docs/` | 48 top-level pages, 13 mentioning `cmd/f4`; 20 across the whole `docs/` tree including `ISSUES/` | per-commit sweeps kept paths current; subjects still need review |

## Files to Change

| Path | Action | Required change |
|---|---|---|
| `.github/workflows/build.yml` | modify | Shard definitions, `packages` scope |
| `AGENTS.md` | modify | Project Structure, counts, docs table |
| `README.md` | modify | Build and icon instructions |
| `.ai-factory/rules/base.md` | modify | Module Structure section |
| `.ai-factory/ARCHITECTURE.md` | modify | Target → fact |
| `docs/*.md` | modify | Subjects, not only paths |
| `.ai-factory/RESTRUCTURE_BASELINE.md` | delete | Or keep, with a stated reason |

---

## Task 38: Rebalance the CI shards

### Intent

The lint and race shards are named after `cmd/f4` because one package held 345
files and 96 495 lines of tests. That package now holds `main.go`. The shards
stayed *correct* throughout the migration — files migrated between them on their
own, and `build.yml:1395` computed the `packages` scope by exclusion — but they
are now badly imbalanced: three race runners split a package with almost no tests
while one runner carries fifteen packages.

This is done **once, here**, not fourteen times during the waves.

### Implementation Steps

1. Replace the lint matrix at `build.yml:983`. Instead of two shards named by
   package path, compute the split from `go list ./...` at job time — for example
   two shards taking alternate entries of the sorted package list, or a split on a
   stable hash of the import path. The requirement is that **no shard definition
   names a package path**, so the next restructuring does not have to touch it.
3. Simplify the shard router at `build.yml:1035-1075`. The `cmd_affected` special
   case and the `./cmd/f4/...` argument branch exist only to serve the named
   shards; with a computed split, the router reduces to "map each affected package
   to its shard".
4. Replace the race matrix at `build.yml:1271-1273`. Drop the `cmd/f4 A` /
   `B-L` / `rest` letter split and its `run:` filters, and drop the `packages`
   scope's exclusion at `build.yml:1395` — with no giant package there is nothing
   to exclude. Shard by package the same way as the lint job.
5. Keep the two behaviours that are not about sharding:
   - the global `-skip '^TestAllDialogs_LayoutValidation$'` at `build.yml:1235`
     and its single-threaded re-run. Confirm which package it points at — Task 25
     step 5 leaves it at `./internal/dialog`, `./cmd/f4` or `./internal/panel`
     depending on how `dialog_layouts_test.go` was split — and require an explicit
     `--- PASS` from that target here. The reason for the isolation is unchanged:
     layout validation mutates shared UI registries from parallel subtests.
   - the race-instrumented cache keys at `build.yml:1288-1296`. Update the
     `cache-key` values to match the new shard names; the reason for a separate key
     (the shared setup-go key is claimed by a non-race job) still holds.
6. **Measure.** Record the wall-clock of the `lint` and `race` jobs before and
   after on the same commit. A rebalancing that makes CI slower is not done.


### What Task 38 actually found

Two jobs were not sharded badly — they were **not running what they named**.

**`TestAllDialogs_LayoutValidation` was running nowhere.** The test job removes
it from every target with `-skip` and re-runs it single-threaded, because layout
validation mutates shared UI registries from parallel subtests. The re-run named
`./cmd/f4`. Task 36 moved the test to `internal/app`, and
`go test -run '^TestAllDialogs_LayoutValidation$' ./cmd/f4` prints
`ok ... [no tests to run]` and **exits 0**. So the test was excluded everywhere,
re-run nowhere, and every cell stayed green — including the full-matrix run this
branch read as its confirmation. It now names `./internal/app` and asserts its
own `--- PASS`, because the same silence would return the moment it moves again.

**The race job's three heaviest shards were running four auditor files.** They
filtered by test-name letter over `./cmd/f4`, which is now `main.go` and four
auditors, while the `packages` scope carried `internal/app`'s 235. Measured on
34298174157: the three shards took 0.7-0.9 minutes each and `packages` 2.7.

Both are the same shape, and it is the shape this branch keeps finding: a check
that names a location, and goes quiet rather than red when the thing it names
moves away.

**So the shards are numbered now, and their contents computed.** The plan asked
for that and it was right, for a reason stronger than balance: a shard that
names a package path has to be edited by every restructuring, and the one that
did — `cmd/f4` — kept the application in its name for three phases after the
application left. `.github/actions/shard-packages` assigns each package to a
shard by weight, longest-processing-time first, weight being the number of test
files. Nothing in `build.yml` names a package.

Measured on the finished tree, 69 packages:

| shards | per-shard weight | per-shard packages |
|---|---|---|
| 2 (lint) | 291, 291 | 40, 29 |
| 4 (race) | 157, 142, 142, 141 | 1, 17, 18, 33 |

The shard holding `internal/app` gets it alone, and 157 of the 581 total weight
is irreducible: a package cannot be split across runners without a test-name
filter, and a filter has to name the package it filters — which is the thing
being removed. That floor is acceptable because it is measured to be under the
Test cells' wall clock (3.9-4.9 minutes on 34295760690), so the race job is not
on the critical path and does not need the split it used to have.

**The empty-shard guard changed meaning and had to be kept explicitly.** The old
one errored when the package list came out empty, which could only happen by
accident. A computed split has two empty cases that look identical and are not:
a shard of a *filtered* list is legitimately empty when nothing this diff
touched landed in it, and a shard of the *whole module* being empty means the
assignment dropped packages. The action errors on the second and skips the
first.


### Required Interfaces and Contracts

- Every package in `go list ./...` is linted by exactly one shard and raced by
  exactly one shard. No package is covered twice and none is dropped — this is the
  property the named shards guaranteed by construction and a computed split must
  prove.
- The incremental-lint path (`--new-from-rev=origin/main`) is unchanged in
  behaviour; only the shard assignment changes.
- `fail-fast: false` stays: a shard failing must not cancel its siblings.

### Error Handling and Logging

An empty shard is a workflow error, not a silent skip — `build.yml:1336-1339`
already errors when the package list comes out empty, and that guard must survive
the rewrite. A shard computing to zero packages after a future rename is exactly
the failure mode it protects against.

### Tests

The workflow is the test. Validate before merging:

```
# every package assigned exactly once
go list ./... | sort > /tmp/all.txt
# run the shard-assignment snippet for each shard, concatenate, sort, compare
diff /tmp/all.txt /tmp/sharded.txt
```

Then push the branch and confirm both jobs go green with no package unaccounted
for.

### Acceptance Criteria

- `grep -n "shard: 'cmd/f4'\|cmd/f4 A\|cmd/f4 B-L" .github/workflows/build.yml`
  returns nothing.
- The `diff` above is empty.
- Lint and race wall-clock times are recorded in the commit message and are not
  worse than before.

### Verification

- The `diff /tmp/all.txt /tmp/sharded.txt` check.
- Expected result: no output.
- A CI run on the branch.
- Expected result: both jobs green, every shard non-empty.

---

## Task 39: Run the incremental lint against `origin/main` before opening the PR

### Intent

`.golangci.yml` and `.golangci-strict.yml` contain no paths and need no edit. The
risk is elsewhere: the incremental lint runs `--new-from-rev=origin/main`, and a
changed `package` clause can defeat golangci-lint's rename detection. If it does,
the project's existing backlog surfaces as new findings on this pull request —
roughly 2450 of them — and the maintainer's first impression of the PR is a red
job with thousands of entries.

Finding this locally costs one run. Finding it in review costs the PR.

### Implementation Steps

1. **Level `origin/main` with `upstream/main` first.** The incremental lint takes
   `origin/main` as its base, and the fork lags upstream — 37 commits at the time
   this step was added. Every one of those commits is reported as this branch's
   work: the first CI run of the branch returned two gosec findings in
   `cmd/f4/input_translation.go`, which came from the upstream commit `5b21864e`
   and not from this work at all. Until the bases agree, a backlog measurement
   says nothing, because there is no telling whose backlog it is.
2. Fetch the base and run the same command CI runs, over the whole branch:
   ```
   git fetch origin main
   golangci-lint run --new-from-rev=origin/main ./...
   ```
3. Record the finding count. Compare against a run on `origin/main` itself to
   establish what the pre-existing backlog is:
   ```
   git stash && git checkout origin/main
   golangci-lint run ./... 2>&1 | tail -1
   git checkout - && git stash pop
   ```
4. If the branch run reports substantially more than the genuine new-code findings,
   rename detection failed. Do not fix it by adding nolint directives. Instead
   state the measured numbers in the PR body — "golangci-lint's `--new-from-rev`
   does not track these renames; the incremental job reports N findings, of which
   the pre-existing backlog on `main` is M" — so the maintainer sees a known
   quantity rather than a mystery.
5. **Measure the exported surface by taking it away, not by counting it.** Each
   wave exported whatever the layer above turned out to need, and a mechanical
   rename exports more than it should. A text search cannot tell a name that is
   still used from one that only looks used: `.Dir` matches `filepath.Dir`,
   `.Label` matches every widget. Un-export the suspects instead and let the
   compiler answer — it resolves each selector to the object that declares it,
   which no grep can do. Task 34 did this for the five most generic names on
   `internal/panel` (`Pf`, `Free`, `SourcePath`, `Filesystem`, `Chord`) and every
   one of them broke a caller, which is the answer: nothing was over-exported.
   The tool is `packages.Load` with `NeedTypesInfo`, renaming by object identity;
   a run costs a minute and a wrong guess costs nothing, because the build fails
   loudly and the change reverts by re-exporting the same name.

6. Run the strict configuration over the new packages only, where it is meaningful:
   ```
   golangci-lint run -c .golangci-strict.yml ./internal/numeric/... ./internal/toast/... ./internal/history/... ./internal/action/...
   ```
   These four are new code written during this work, so they can be held to the
   strict bar.

7. Verify that **every commit** on the branch builds, not only `HEAD`. The
   plan's central invariant is that any commit can be checked out and built, and
   checking `HEAD` alone never tests it:
   Walk the revisions in a scratch worktree, which reads and rewrites nothing:
   ```
   git worktree add -q --detach /tmp/verify upstream/main
   for c in $(git rev-list --reverse upstream/main..HEAD); do
       # -m --first-parent: git diff-tree prints NOTHING for a merge commit
       # without it, so a filter on "does this touch Go" skips every merge --
       # which is exactly the set where conflicts were resolved by hand and a
       # broken commit is most likely. Nine of this branch's commits were being
       # skipped that way, the three upstream merges among them.
       git diff-tree --no-commit-id --name-only -r -m --first-parent "$c" \
           | grep -qE '\.go$|^go\.(mod|sum)$' || continue
       git -C /tmp/verify checkout -q --detach "$c"
       (cd /tmp/verify && CGO_ENABLED=0 go build ./... && go vet ./...) \
           || echo "FAIL $c $(git log -1 --format=%s "$c")"
   done
   git worktree remove --force /tmp/verify
   ```
   Skip a commit that touches no `.go`, `go.mod` or `go.sum` — its result is the
   previous commit's. Use `go vet` and not only `go build`: `build` ignores test
   files, and the failure this catches is a helper deleted one commit before its
   replacement arrives.

   `git rebase --exec` would do the same job and rewrite every commit id doing
   it. Worse, this branch merges upstream rather than rebasing onto it, so a
   plain `git rebase` would flatten those merges and check a history that is not
   the one being reviewed. If a rebase is used anyway it needs
   `--rebase-merges`; the worktree walk needs nothing and changes nothing, which
   is what a check should do.

   A broken commit in the middle of a 300-file restructuring is not cosmetic: it
   breaks `git bisect` for whoever debugs a regression a year from now, and it is
   the first thing a maintainer notices on a branch that claims every step is
   green. The failure mode to watch for is a staged deletion travelling in
   somebody else's commit — `git commit` takes the whole index, not the paths
   just handed to `git add`.

### What Task 39 actually measured

The fear the task was written around did not happen, and the numbers are the
answer to it. Measured on `da85d539`, whole tree, default configuration,
`--max-same-issues=0 --max-issues-per-linter=0`:

| tree | issues | errcheck | gosec | unused |
|---|---|---|---|---|
| `origin/main` (165 upstream commits stale) | 389 | 308 | 80 | 1 |
| `upstream/main` | 375 | 294 | 80 | 1 |
| this branch | **276** | 206 | 69 | 1 |
| `--new-from-rev=origin/main` | **0** | | | |

So rename detection survives: if it had failed, the incremental job would have
reported the branch's whole 276 as new. It reports nothing, and the maintainer's
first impression is a green job rather than the red one with thousands of
entries the task was written to prevent.

The branch also **removes 99 findings** relative to `upstream/main`, which is
not a goal it set out with. They are the `_ =` and `#nosec` the waves added
where a rename made an old line new to `--new-from-rev` and the incremental job
demanded an answer for it. Every one of them is on a line whose behaviour did
not change; the count is a side effect of the linter's attention moving, not of
error handling improving, and the PR body should say so rather than claim
credit.

The strict configuration (`.golangci-strict.yml`) reports the same single
`unused` on both trees: the linters that graduated to whole-tree checking still
have no backlog, and the branch adds none.

**The ~2450 the task predicted was never measured**, and it is worth saying why
the guess was so far off rather than only that it was: it appears to have come
from a whole-tree run under a configuration that has since narrowed, and nobody
re-derived it after. A number carried through eleven phases without being
re-measured is the same kind of debt as a check that names a place.

### Required Interfaces and Contracts

- No `nolint` directive is added to work around rename detection.
- No lint configuration is changed. The two `.golangci*.yml` files are outside the
  scope of this plan.
- `gosec`'s `G115` findings must be zero in `internal/numeric` — Task 19 required
  the `#nosec` annotations to travel verbatim, and this is where that is confirmed.
- Every commit from `upstream/main` to `HEAD` builds and vets clean.

### Error Handling and Logging

Not applicable. This task produces numbers and a paragraph for the PR body.

### Tests

The lint runs above are the test.

### Acceptance Criteria

- Both counts are recorded.
- `golangci-lint run -c .golangci-strict.yml ./internal/numeric/...` reports no
  `G115`.
- The per-commit walk reaches `HEAD` without a failure.
- If rename detection failed, the PR body says so with the measured numbers.

### Verification

- `golangci-lint run --new-from-rev=origin/main ./... 2>&1 | tail -3`
- Expected result: a finding count that the PR body accounts for.

---

## Task 40: `/aif-docs` checkpoint

`docs/FILELIST.md` is generated by `scripts/filelist_update.sh` and no wave has
run it: the file still describes the flat `cmd/f4` of the base revision. Running
the script is the whole fix.

### Intent

Each move commit closed its own path references — that is a ground rule and an
unclosed reference is an unfinished move. But the restructuring changes what the
48 subsystem documents *describe*, not only the paths inside them. A document that
says "panels live in the flat package alongside the editor" is not fixed by
updating a path.

### Implementation Steps

1. Run `/aif-docs` and treat its findings as part of this commit rather than as a
   follow-up.
2. `AGENTS.md` needs more than a path sweep. Rewrite:
   - the Project Structure block, which still opens with
     `cmd/f4/  # the application: 687 files in one flat package main`;
   - the Documentation table row for `SPREADSHEET.md`, moved in Task 12;
   - the Agent Rules section on CodeGraph, whose advice — "`cmd/f4` is one flat
     `package main` of ~109k lines, so grep over it is slow and matches
     identifiers it should not" — was true and is now false. The graph is still the
     right tool for symbol questions; the *reason* changed.
3. `.ai-factory/rules/base.md`'s Module Structure section lists the pre-move tree
   and tells new code where to go. Update the list, re-derive the counts it
   quotes, and drop the transitional wording — after this branch the tree is not
   "being split", it is split.
   Then give the section a **File Placement** part, because this is the file the
   AI Factory skills read as project rules and it currently answers "what exists"
   without answering "where does mine go":
   - the package that owns the subject; no package owns it, create one; never
     `internal/app`, which wires and does not implement;
   - nothing new in `cmd/f4` — it holds `main.go`, the wiring tests, the four
     module-wide auditors and the Windows `.syso` files;
   - inside a package, `<topic>.go` and `<topic>_<aspect>.go`, prefix naming the
     topic and not the package (`panel/frame.go`, never `panel/panel_frame.go`),
     platform suffix last;
   - a test lives with its subject; a test spanning packages is hosted by the
     latest one and splits or uses `package X_test` — the rule Task 43 applied to
     61 of them;
   - resources travel with the package that embeds them, and a test reading them
     from disk guards against the empty set.
   Keep it short and point at `ARCHITECTURE.md` for the reasoning: `base.md` is
   the rule, the architecture document is the argument. Duplicating the argument
   in both guarantees they drift.
4. Walk the 13 of 48 top-level `docs/*.md` pages that mention `cmd/f4` and check
   each for a claim about *structure* rather than a path (`grep -rl 'cmd/f4' docs/`
   returns 20 because it also walks `docs/ISSUES/`, which is a historical record
   and is not rewritten here): which subsystem owns what, what is in one
   package, what a contributor must not couple.
5. `README.md:233`, `:237`, `:241` — the build and icon-generation instructions.
   Confirm Tasks 11 and 27 left them correct.

### Required Interfaces and Contracts

- Documentation describes the current state. No document explains the migration,
  compares "before" with "after", or references how the code used to be organised —
  that history lives in git.
- Counts quoted in prose are re-derived, not estimated: package count from
  `go list ./... | wc -l`, file counts from `find`.

### Error Handling and Logging

Not applicable.

### Tests

No automated test covers prose. The check is the grep sweep:

```
grep -rn 'flat package\|one flat\|687 files\|345 files' docs/ README.md AGENTS.md .ai-factory/
```

### Acceptance Criteria

- The grep above returns nothing outside `ARCHITECTURE.md` (Task 41 owns that
  file) and the plan artifacts under `.ai-factory/plans/`.
- `AGENTS.md`'s Project Structure block lists the packages that exist.
- Every count quoted in `AGENTS.md` and `rules/base.md` is re-derived.

### Verification

- `grep -rn 'cmd/f4/' docs/ README.md AGENTS.md`
- Expected result: only the build invocations (`go build ./cmd/f4`,
  `go generate ./cmd/f4`) and the `.syso` note.

---

### Note: `docs/FILELIST.md` is generated

`scripts/filelist_update.sh` used to write it from a `tree -a` of the working
directory, which is why the note above warned about `build/` and the tool state
directories: a walk has to be told what to leave out, and that exclusion list
goes stale in silence — it says nothing when a new ignored directory appears, it
simply lists it. It builds from `git ls-files` now, which answers the same
question the file claims to answer and needs no list at all. `.claude/` and
`.agents/` appear in it because they are tracked, which is the honest answer.

**Whether the file should exist is a separate question, and it belongs in the
PR.** `grep -rl FILELIST` finds the file, its generator and this plan — nothing
reads it. It described the flat `cmd/f4` of the base revision for eleven phases
and nobody noticed, which is the usual fate of a generated artefact that no
check regenerates. Propose deleting it and its generator as its own item in the
PR body; do not delete it here, because what belongs in the maintainer's
repository is the maintainer's call.

---

## Task 41: Rewrite `ARCHITECTURE.md` from target to fact

### Intent

Every other document needs updating. This one changes genre. It currently
describes a destination — `(move)` and `(extract)` markers on a tree that does not
exist yet, an extraction order, a migration policy, a table splitting files
between layer-0 and view-bound halves. Once the plan is executed, none of that
describes the repository; it describes the journey.

The project rule is explicit and applies to documents as much as to code
comments: text explains the **current** state, never the past. No "used to be X,
now Y", no account of how the code evolved. History lives in git. So this is not
"append a note saying it is done" — it is removing everything that describes a
transition and leaving a description of the result.

### Implementation Steps

0. **Locate the sections by heading, never by line number.** Start with
   `grep -n '^#\{1,3\} ' .ai-factory/ARCHITECTURE.md` and work from the offsets it
   prints. Every section below is named by its heading text for that reason: line
   numbers in a 559-line document drift with the first edit, and the sections in
   step 4 are bold paragraphs inside **Folder Structure**, not headings of their
   own — find them by their bold title.
1. **Folder Structure** (heading to the next one): strip every `(move)` and
   `(extract)` marker.
   Nothing moves any more; the tree is the tree. Keep the annotations that explain
   *why* a directory sits where it does — `sdk/` and `vfs/` being importable from
   outside the module, `embedded.go` being pinned by `//go:embed`,
   `rsrc_windows_*.syso` being linked only from the built package's directory,
   `internal/hideconsole` being a vendored fork.
1a. **The composition-root example is fiction and must be replaced, not
   annotated.** `ARCHITECTURE.md` shows `main` as

   ```go
   application := app.New(cfg, fs, host, term, left, right)
   if err := application.Run(context.Background()); err != nil { … }
   ```

   and repeats the constructor under **Dependency direction**. Neither exists:
   `cmd/f4/main.go` is `func main() { app.Main() }`, and Task 36 recorded why —
   4490 references to `vtui.FrameManager`, `config.App`,
   `keymap.GlobalHotkeysMgr` and `macro.MacroMgr` across `internal/`, so a
   constructor taking those things would list its dependencies rather than
   receive them. This document is handed over as a description of the built
   tree; leaving that example in it makes it wrong in its first paragraph about
   the composition root, which is the one paragraph a reviewer reads first.

   Replace both snippets with what the code does, and say in one line what the
   argument-taking form would require — the same 4490 — so the shape is recorded
   as a proposal rather than as a description. The proposal itself belongs in
   Task 44 step 2, not here.

1b. **Leave a check behind, or it drifts again.** The document lists packages and
   layers in prose, which cannot help going stale — it had six packages missing
   and four in the wrong layer when this task started, and nothing said so. Put
   the two commands that answer it into the Dependency Rules section itself: a
   `diff` of the package names it mentions against `ls -d internal/*/`, and the
   `architectureLayers` map it must agree with. A reader then asks whether it is
   current instead of reading it to find out.

2. Add the packages this plan created that the document does not yet name:
   `internal/action`, `internal/toast`, `internal/history`, `internal/numeric`,
   `internal/testutil` and `internal/paneltest`. Give the last two a line saying
   they are test scaffolding and no production file imports them.
3. **Overview** (first heading): "Two things are missing… `cmd/f4` holds 345 non-test
   files and ~109k lines in one flat `package main`" is false after Phase 10.
   Rewrite the paragraph to state what the layout *is* and what rule it expresses.
4. **Delete the sections that are scaffolding**, not description:
   - the bold paragraph "**`app` is two things, and only one of them is the
     root**" inside Folder Structure, with its file-split table. It is an
     instruction for performing the extraction.
   - the bold paragraph "**`sysinfo` keeps its own copy of the one numeric helper
     it needs**" — *keep the rule*, drop the justification framed as a migration
     decision. It is a live constraint: sysinfo is a leaf and must stay one.
   - the whole **Legacy vs New Code Policy** section: the extraction order, the
     one-subsystem-per-commit rule, "no rewrites inside a move commit", "a move is
     not done until the prose agrees". Scaffolding, all of it. What survives is the
     first bullet, reworded: new code goes into the module it belongs to, and if
     none fits, create the package.
5. **Keep unchanged** — these are permanent contracts, not migration aids:
   - **Decision Rationale**
   - **File Naming Inside a Package**, including the multi-type-file rule
   - **Dependency Rules** and the layer table, updated only with the new package
     names from step 2. It already places `internal/wincon`, `internal/ttyx`,
     `internal/netproxy` and `internal/hideconsole` at layer 0; Task 8 seeds the
     auditor's map with the first three so the two agree.
   - **Layer / Module Communication**
   - **Key Principles**, with principle 5's test-scaffolding paragraph rewritten to
     describe `internal/testutil` and `internal/paneltest` as they exist rather
     than as a plan
   - **Code Examples**
   - "Not every directory here is one module", inside Dependency Rules — six
     `go.mod` files is a standing fact
7. **Anti-Patterns** (last section): "**Adding to the flat package**" must be
   reworded. There is no flat package any more, but the rule it protects survives:
   a new feature belongs in the module it serves, and if none fits, in a new
   package — never appended to whichever package is nearest.
7. **Re-derive every number.** `345 non-test files`, `~109k lines`,
   `297 files of extensions`, `~700 Go files outside plugins`,
   `560 _test.go files`, `48 subsystem documents`, `133 files read AppConfig`,
   `37 call edges`, `42 audit keys`. Count them; do not adjust them by arithmetic.

### Required Interfaces and Contracts

- The document describes the repository as it is. A reader who has never seen the
  old tree must not be able to tell that a restructuring happened.
- No sentence contains "used to", "previously", "was moved", "after the
  extraction", "no longer", or a date.
- The dependency rules, layer table and naming convention keep their normative
  force — they are what the next contributor is held to.
- `cmd/f4/architecture_test.go` (Task 8) enforces four of the rules. Where the
  document states a rule the test checks, say so, so a reader knows which rules
  are mechanical.

### Error Handling and Logging

Not applicable.

### Tests

The auditor is the closest thing to a test of this document:

```
go test ./cmd/f4 -run '^TestArchitecture' -v
```

Every layer the document asserts must appear in the test's layer map, and vice
versa. A package in the document but not in the map is undocumented drift.

### Acceptance Criteria

- `grep -n '(move)\|(extract)' .ai-factory/ARCHITECTURE.md` returns nothing.
- `grep -niE 'used to|previously|no longer|after the extraction|migration' .ai-factory/ARCHITECTURE.md`
  returns nothing.
- Every package in `architecture_test.go`'s layer map appears in the document's
  folder structure, and vice versa.
- Every quoted count is re-derived.

### Verification

- `go test ./cmd/f4 -run '^TestArchitecture' -v` and a manual comparison of the
  layer map against the document's layer section.
- Expected result: the two agree package for package.
- `grep -n '(move)\|(extract)' .ai-factory/ARCHITECTURE.md`
- Expected result: no output.

---

## Task 42: Drop the migration baseline

### Intent

`.ai-factory/RESTRUCTURE_BASELINE.md` answered one question — "was this test red
before we started?" — for fourteen wave commits. With the last wave green, nothing
asks it any more.

This is the only task permitted to modify or remove that file.

### Implementation Steps

1. Confirm the last wave compared clean against it.
2. Decide, and state the decision in the commit message:
   - **delete** — `git rm .ai-factory/RESTRUCTURE_BASELINE.md`; the information it
     held is in the commit history of this branch; or
   - **keep** — because it records three pre-existing failures
     (`tools/icons`, `tools/wine_syscall_probe` on darwin/arm64, the inert
     `plugring_policy_test.go`) that outlive this work and that nobody else has
     written down. If keeping it, rename it to something that is not about a
     finished migration and move it to `docs/`.
   Prefer deleting: two of the three findings should by then be issues in the
   tracker, which is where they belong.
3. Whichever is chosen, make sure the three pre-existing findings do not vanish
   silently. If the file goes, they go into the PR body or into issues.

### Required Interfaces and Contracts

None. This is a repository-hygiene task.

### Error Handling and Logging

Not applicable.

### Tests

None.

### Acceptance Criteria

- The commit message states which option was taken and why.
- The three pre-existing findings are recorded somewhere that survives this
  branch.

### Verification

- `go test -timeout 25m ./...` and the five other module runs.
- Expected result: green everywhere except the three recorded pre-existing
  failures, which are unchanged from Task 1.

---

---

## Task 45: Write the pull request

**Say that the 42 renamed issue documents are renames.** Both trees hold 43
files in `docs/ISSUES/`; upstream keeps the `*_SOLUTION_REVIEW.md` names, phase 2
renamed them for their subject. Content-identical but for one line this branch
changed on purpose, and git resolves them as renames — but a reviewer scanning a
300-file diff sees 42 deletions next to 42 additions unless the body says
otherwise.

**And one item of its own: `docs/FILELIST.md`.** Nothing reads it, no check
regenerates it, and it described the base revision's tree for eleven phases
without anybody noticing. It is correct again and its generator no longer walks
the working directory, but a generated file that only a human remembers to run
will go stale again. Propose deleting both, and say what replaced the need: `git
ls-files` answers the same question on demand.

**The body needs a section for what the restructuring found rather than broke,
and it must be separate from the moves.** A reader of a 300-commit branch cannot
otherwise tell "we fixed what we broke" from "we found what was lying there" —
and only the second is an argument in favour of the work. What belongs there so
far, each with its evidence:

- **`internal/editor` raced its own teardown.** `EditorView.Close` cancelled the
  highlighting goroutine and did not wait for it, then called `BaseFrame.Close`,
  which writes the field that goroutine reads through `IsDone` between slices.
  The indexing goroutine two lines above is joined with `indexWG.Wait()`; the
  highlighter had no equivalent. Pre-existing, product-side, found by the race
  detector on run 34298174157 and fixed here.
- **`tools/icons`' own test never passed** (below).
- **`plugring_policy_test.go` asserted nothing.**
  `TestShippedCatalogMeetsItsOwnPolicy` read `plugring/index.yaml` relative to
  the working directory, which from `cmd/f4` is nothing, so every run reached
  `t.Skipf` and reported as passing. It resolves the path from its own file now
  and checks what it claims to. Same family as everything else this branch
  found: green because it had nothing to look at.
- **The `.lng` key that reaches the user as `{KeyBar.EditorAltF8}`** — upstream's
  gap, found by the sweep Task 46 added, and left as theirs to fix.

`internal/fileops`' race does **not** belong in that section: a test replaced
`vtui.FrameManager` under a goroutine it had started, which is this work's own
test hygiene, not a bug in f4. Keeping the two apart is the whole point of the
section.

**The migration baseline is gone, and its three findings resolved as follows**
— say this rather than leaving the reader to wonder what `RESTRUCTURE_BASELINE.md`
was:

- `tools/icons`' own test never passed, and passes now.
- `plugring_policy_test.go` reported as passing while asserting nothing, and
  asserts now.
- `tools/wine_syscall_probe` does not build on `darwin/arm64` — and that is not
  a failure. `rawGetpid` has no Go body because it is written in
  `probe_amd64.s`; the tool is a `windows/amd64` probe, it builds for that
  target, and CI never builds it for another. The baseline recorded it as a
  pre-existing failure; it is a single-target tool being asked the wrong
  question.

**One more line the body owes the reader**, recorded here so it survives to
Task 45: `tools/icons`' own test never passed. It read
`../../assets/icon/f4.svg` while the tool it tests read `cmd/f4/assets/icon/`,
one directory apart, and the module has its own `go.mod` so `go test ./...`
from the root never saw it. Fixed in the icons wave, which is what moved the
directory. That drops the branch's pre-existing failures from three to two.

### Intent

Six earlier tasks each end with "call this out in the PR body" and none of them
owns the body. A requirement everybody references and nobody writes is a
requirement that does not happen. This is where it is written, together with the
context a maintainer needs to judge a 300-file change he did not plan.

### Implementation Steps

1. **What and why**, in three or four sentences. `cmd/f4` held 345 non-test
   files in one flat `package main`; the compiler enforced no boundary anywhere
   in the application. Now it does.

2. **`Closes #505`.** The issue asked to tidy the repository root, which is
   Phase 2 of this branch. Say plainly that the PR does more than the issue
   asked, and why the rest belongs in the same change: the root cannot be tidied
   meaningfully while everything above it lives in one package.

3. **Answer the refusal in the issue thread on its own terms.** The bot
   maintainer declined it as "a broad repository-reorganization proposal without
   a narrowly defined bug or testable acceptance criteria", adding that doing it
   autonomously "would require a maintainer-approved project structure and
   migration plan". That is not "no", it is a list of what was missing — and the
   PR brings exactly those two things: `ARCHITECTURE.md` is the structure, and
   the archived bundle is the migration plan, with acceptance criteria on every
   task. Say so in one paragraph. It moves the PR from "here is my vision" to
   "here are the missing inputs; the decision is yours".

4. **Answer the `external/` question the issue asks.** No, and briefly why:
   `internal/` in Go is a compiler rule rather than a naming convention, and it
   has no counterpart. The public surface is whatever sits *outside* it — here
   `sdk/` and `vfs/`, which third-party plugins compile against. An `external/`
   would restate in a directory name something the language already enforces.

5. **Explain where the tree differs from the one sketched in the issue**, rather
   than diverging in silence. The sketch was the right direction; the boundaries
   came from measurement.
   - `tui/` (buffer, driver, event) — not created. The terminal engine lives
     outside this repository, in `vtui` and `vtinput`; there is nothing to put
     in it.
   - `desktop/` (screen manager, modal stack) — not created. No such thing
     exists in the code; `vtui` owns the window stack.
   - `vfs/` stayed in the root instead of moving under `internal/`: third-party
     plugins compile against it, and `internal/` would forbid that import.
   - `job/` became `fileops` and `keybind/` became `keymap`, named for what they
     actually hold.
   - Packages the sketch did not have — `term`, `media`, `plughost`, `sysinfo`,
     `i18n`, `theme`, `colorer`, `numeric`, `action`, `toast`, `history` — came
     out of the call graph over 345 files, not out of general principle.

6. **Where the reasoning lives.** `ARCHITECTURE.md` is the contract: layers,
   dependency rules, file naming, package ownership. The archived bundle is the
   plan that produced it. Both ship in this PR deliberately — a maintainer
   inheriting a restructuring needs to know why a file went where it went, and
   git history alone does not answer that.

7. **How it was built**, as a recommendation rather than a pitch. The chain is
   explore → plan (ultra) → improve → implement → verify → review →
   security-checklist → archive, from the AI Factory skills vendored in
   `.claude/skills/`. Two things are worth saying because they are what adopting
   it would actually buy: every artefact has one known location instead of loose
   markdown in the repository root — which is what #505 complains about — and the
   plan outlives the session that wrote it, so work continues across context
   resets and across people. Add that the bundle was verified four times and each
   pass disproved the previous one: routes measured by filename were wrong, the
   ordering criterion was inverted, `actions.go` could not move as a file, and
   154 tests had no assignment at all. That is the case for the method, and it is
   stronger than any claim about it.

8. **The graph tooling.** CodeGraph is wired in `.mcp.json` and answers
   caller/callee/impact questions over a package where grep is both slow and
   imprecise. Note that a fresh clone needs one `init` and that the index is
   git-ignored.

9. **Everything the earlier tasks asked to surface**, one line each: the README
   screenshot URL 404s until this merges into `main` (Task 11); the
   incremental-lint backlog number, if rename detection surfaced it (Task 39);
   the three pre-existing failures recorded in the baseline (Task 42); the
   structural review outcome and any follow-up proposals (Task 44).

10. **One line on why the rule matters more than the tidying.** `CI.md` appeared
    in the repository root while this branch was clearing it. That is not a
    complaint about the file or its author — it is the argument: a root stays
    tidy because something says where a file goes, not because somebody tidied
    it once. `ARCHITECTURE.md` and `rules/base.md` are that something, and they
    are in this PR.

11. **The function-typed package variables, and why their defaults are hostile.**
    A reader meeting `var Executable = func() (string, error) { return "", errNotWired }`
    in a diff will read it as an accident unless the body says otherwise. Four
    sentences: the dependency rules forbid a layer-0 package importing a layer
    above it, so where a lower package needs a function that moved upward, it
    declares a variable of the right signature and the composition root assigns
    the implementation — `internal/config` never learns that `internal/update`
    exists, and who implements what is known in one file, `main.go`. It is the
    "lower layer defines the interface" rule from `ARCHITECTURE.md` with a
    variable instead of an interface, because one function does not need one. The
    defaults refuse rather than work because a default that works turns forgotten
    wiring into silently wrong behaviour: `os.Executable` on a universal build
    returns the loader's path *successfully*, and f4 would read its ini next to
    `ld.so` and look to the user like "settings are not saved" rather than like a
    failure. A refusing default turns that into a loud one at startup.

    Keep the running list here so the body does not have to be reconstructed:
    - `history.SamePath` and the history config directory (Task 20)
    - `action.Localize` (Task 21, wired to `i18n.Msg` in Task 24)
    - `config.Executable` (Task 24)
    - the seven `TestMain` seams (Task 9)

12. **What was deliberately not done**: no behaviour change inside a move commit,
    no renamed Far-derived type, no further splitting of packages — that is
    proposed as follow-up with evidence rather than smuggled in.

13. **Re-measure the diff numbers in the same minute the PR is opened.** The
    body leads with a file and line count, and every commit moves it — the first
    draft was already stale by two commits before it was reviewed. The order is:
    commit everything, then measure, then paste, then open.

    ```
    git diff --shortstat -M upstream/main...HEAD
    git diff --name-status -M upstream/main...HEAD | grep -c '^R'
    git diff --shortstat -M upstream/main...HEAD -- ':!.claude' ':!.agents' ':!.ai-factory'
    ```

    A number in a PR body that is wrong on the first line is the cheapest
    possible way to lose a reader who was willing.

### Required Interfaces and Contracts

- The body is **prepared, not sent**. No `git push` and no `gh pr create`: when
  the work goes out is the user's decision and this task does not take it.

### Error Handling and Logging

Not applicable.

### Tests

None. The body is prose.

### Acceptance Criteria

- `Closes #505` is present.
- The refusal in the thread is answered, and the `external/` question with it.
- Every divergence from the issue's sketch is named and explained.
- All four carried-over items from step 9 appear.
- The workflow recommendation is concrete and says what the maintainer gains.
- Somebody who did not plan this change can read the body and know what to
  review first.

### Verification

- Preview the body (`gh pr create --dry-run`, or read the file).
- Expected result: a body a maintainer can act on. Nothing is pushed and no pull
  request is opened.

## Phase Risks and Mitigations

- **Risk:** the computed CI shards drop a package and a whole area stops being
  linted or raced, silently.
  **Mitigation:** Task 38's `diff /tmp/all.txt /tmp/sharded.txt` check proves every
  package is assigned exactly once, and the empty-shard guard at
  `build.yml:1336-1339` survives the rewrite.
- **Risk:** the incremental lint's rename detection fails and the PR opens red
  with thousands of pre-existing findings.
  **Mitigation:** Task 39 measures it locally before the PR and puts the numbers in
  the PR body.
- **Risk:** `ARCHITECTURE.md` is updated by appending "this is now done", leaving a
  document that narrates a migration.
  **Mitigation:** Task 41's acceptance criteria grep for exactly that vocabulary,
  and the project rule against past-tense explanation is stated in the intent.
- **Risk:** the baseline is deleted along with the only written record of three
  pre-existing failures.
  **Mitigation:** Task 42 step 3 requires them to land somewhere else first.

## Phase Completion Checklist

- Every Task 38-42 satisfies its acceptance criteria.
- No shard definition in `build.yml` names a package path.
- The PR body accounts for the incremental-lint finding count, the plugring
  catalogue URL change, and the branch-scoped README image URL.
- No document describes the migration; `ARCHITECTURE.md` describes the tree.
- `go test -timeout 25m ./...` plus the five other module runs match the Task 1
  baseline's green set.
- `index.md` task checkboxes 38-42 are ticked.

---

## Task 44: Review the finished tree before calling it done

### Intent

Every package in the target layout was chosen from a call graph measured while
the code still lived in one flat package. That measurement was good enough to
order the work, but a package only reveals its real shape once it exists and has
callers. This task looks at the result and asks whether the split went far
enough, not far enough, or in the wrong place — and records the answer instead of
leaving it to whoever notices first.

It changes no code. Splitting a package further is a separate pull request with
its own evidence; doing it here would enlarge a change that already touches the
whole tree.

### Implementation Steps

1. **Measure every extracted package as it now stands.** File count, and for each
   one the list of packages that import it:
   `go list -f '{{.ImportPath}} {{join .Imports " "}}' ./... | grep internal/`.
   A package nobody imports but `internal/app` is a candidate for merging back;
   a package imported by everything is a candidate for splitting.
1a. **Re-home whatever upstream added past Task 43's roster.** The roster names
   the 346 files of base revision `0cda22a7`; anything upstream added after it
   has no home there and lands wherever the merge puts it — a silent decision,
   because it compiles and the suite is green while the package may still be
   wrong.

   This was a task of its own, to run after the pull request merged, on the
   reasoning that the list is only complete once upstream stops moving under an
   open PR. Three things retired that reasoning. The branch is level with
   `upstream/main`. `cmd/f4` is five files, so a stray arrival there breaks the
   build rather than landing quietly. And every merge on this branch has placed
   its own arrivals as it went, which is the task being done incrementally. The
   residue — a file arriving between the last merge and the PR — is covered by
   merging once more immediately before opening it.

   The inventory:

   ```
   git ls-tree -r --name-only 0cda22a7      | grep '\.go$' | sort > /tmp/base.txt
   git ls-tree -r --name-only upstream/main | grep '\.go$' | sort > /tmp/up.txt
   comm -13 /tmp/base.txt /tmp/up.txt
   ```

   The comparison is of names, so it sees only what upstream **added**. A file
   upstream *renamed* does not appear at all, and if this branch renamed the
   same file differently the two drift apart in silence. Both halves of that
   happened here:

   - Upstream renamed three files over the distance
     (`git diff --name-status -M 0cda22a7 upstream/main | grep '^R'`), all
     markdown. Since the inventory filters `\.go$` they are invisible to it,
     which is why the Go answer above is complete.
   - **Forty-two documents exist under two names.** Both trees hold 43 files in
     `docs/ISSUES/` and the name sets barely overlap: upstream moved its
     root-level `*_SOLUTION_REVIEW.md` there keeping their names, phase 2 moved
     the same files there and renamed them for their subject
     (`ISSUE_165_CONPTY_SYNC_MARKER.md`). Compared by content, 42 of the 43 are
     byte-identical and the 43rd differs by one line — this branch's own
     `selfCommand` → `update.SelfCommand`. Nothing was lost; git resolves all
     of it as renames (41 × R100, one R098). Say so in the PR body, or the
     diff reads as 42 deletions beside 42 additions.

   So: check for renames before trusting a name comparison, and check by
   content when the names disagree. A file present under a different name is
   the case that looks exactly like a missing one.

   Nineteen files at the time of writing. Fifteen are outside the restructured
   tree — `internal/ttyx` (3), `plugins/archive`, `plugins/id3editor`,
   `sdk/f4plugin`, `vfs/hostfs`, `vfs/registry_vfs_windows`,
   `tools/vtui-screen`: their own packages and modules, untouched by the
   restructuring, each already where it belongs. Only what sits in `cmd/f4` or
   under `internal/` needs the question asked, which left four.

   Ask each one where its **subject** lives, not what its name suggests. Three
   were placed correctly by the merges that brought them:
   `external_editor_process_freebsd.go` and its test to `internal/editor`;
   `plugins_test.go` to `internal/plughost`, where `PluginManager` is;
   `local_language_files_test.go` to `internal/app`, where `initLang` and
   `InitHelpSystem` are.

   **The fourth was the check earning its place.** `edit_command_test.go` sat in
   `internal/app` while its subject, `parsePlainEditCommand`, is in
   `internal/panel`. The bridge was an export — and that function had exactly
   one caller outside its own package, the test itself. A symbol exported so a
   test can reach it from elsewhere is the rule "a test travels with its
   subject" broken and then papered over. The tell that the export was
   mechanical rather than decided: the doc comment above it still said
   `parsePlainEditCommand` in lower case, because the tool that renamed the
   function did not read the comment. The test is in `internal/panel` now, the
   function is unexported again, and the comment agrees with it.

2. **Propose the dependency-injection work with its price, and do not leave it
   as a question.** `internal/app` is the composition root by layer and by
   import rule, and it is not one by construction: it reaches for
   `vtui.FrameManager`, `config.App`, `keymap.GlobalHotkeysMgr` and
   `macro.MacroMgr` the way every other package does, 4490 times across
   `internal/`. `app.New(cfg, fs, host, term, left, right)` — the form
   `ARCHITECTURE.md` describes and Task 36 declined to build — becomes true only
   when those are threaded through instead.

   State the price rather than the wish. It has **no intermediate form**: a
   constructor that takes the arguments and a body that still reads the globals
   is worse than neither, because the signature then asserts something the code
   does not do. And it cannot start where it looks like it starts: the first
   thing it needs is a test of the startup path, and there is none —
   `main_test.go` does not exist, `Main` exits through `os.Exit` at seven points
   and returns nothing. So the order is: cover the entry point, then thread one
   global at a time, and the first one is `config.App` because it is read rather
   than mutated and 1575 of the 4490 sites are its.

1c. **Reconcile every file against the call graph — in two stages, because one
   stage does not work.** The obvious query is "which files reference another
   package more than their own", and run alone it is unusable: fifteen files on
   this tree, fourteen of them noise, in two kinds.

   The harmless kind is a file in `internal/app` calling `i18n.Msg` 314 times,
   or `actions_table.go` with 351 references into `internal/action`. Neither is
   misplaced — localisation is called from everywhere and the table *is* the
   registry's caller. No threshold separates those, because the count is right
   and the conclusion is wrong.

   The dangerous kind is **name collision**. CodeGraph attributes a reference by
   name, and 722 names in this tree have more than one bearer across 4247 nodes
   — `Close` has 203, `Read` 95, and Go's implicit interfaces are why. So
   `internal/plughost/application.go` and `internal/viewer/application.go` both
   came back pointing at `internal/app`: each declares its own `Application`
   interface, and neither imports `internal/app` at all. `keymap/farkeys.go`
   pointed at `sheet` with no f4 imports whatever.

   So: **the query proposes, the import list decides.** Take the candidates from
   the graph, then read each file's own imports. A file that does not import the
   package the graph named is answered, and answered in seconds.

   Run over the finished tree, that leaves exactly one:
   `internal/terminal/ttyx_session.go` — 72 lines, importing `internal/ttyx` and
   `vtui`, no reference into its own package, six into `ttyx`, callers in
   `terminal` (3), `media` (4) and `app` (1).

   **It stays, and the reason is the interesting part.** It is not an opener; it
   is a process-wide singleton with a policy: one `*ttyx.Session` behind a
   mutex, opened lazily, and stood down when `sess.Source().Trusted()` is false
   because everything built on the session draws over a window that was only
   guessed. A stateless opener belongs in the layer-0 library and would be
   reusable there. One that caches per process and refuses on a trust judgement
   is not: who holds the session, and on what terms, is the caller's decision,
   and moving it into `internal/ttyx` would put global state and a policy into a
   library whose whole value is having neither. Moving it would remove one
   `media → terminal` edge and buy that with a worse `ttyx`.

   State this in the PR body as a result, not an intention: every file was
   reconciled against the call graph, the one candidate was examined, and it
   stays for a stated reason.

2a. **Ask the split question where the parts have different callers.** The named
   candidate is `internal/media`: its `image_*`, `audio_*` and `video_*` families
   were measured as effectively unconnected before the move — one reference in
   total, `imageViewBackAttr`, which is a colour attribute and by then may live in
   `internal/theme`. If after extraction `panel` reaches only the image half and
   something else only the audio half, the boundary is real and worth a follow-up.
   If every caller uses all three, it is one package and stays one.
   Apply the same question to the largest results — `term`, `panel`, `app` — and
   to anything over roughly forty files.
3. **Ask whether `internal/terminal` should be `internal/terminal`.** The short name
   is taken: `golang.org/x/term` is used in five files, and where both are
   imported — `cmd/f4/main.go` — ours wins the bare name and the other needs an
   alias. The first alias tried was `xterm`, which names a real terminal
   emulator and reads, in terminal-compatibility code, as a compatibility check
   rather than as a package; it is `goterm` now, and that is a workaround, not
   an answer. `terminal` would need no alias anywhere, and the tree already
   holds `piecetable`, `textlayout` and `hideconsole`, so the usual "Go names
   packages short" argument is weaker here than usual.

   Not done during the waves: 66 imports and roughly 710 `term.<Symbol>`
   references, and a blind replacement breaks the very thing it is for, because
   `term.` also spells `x/term`'s `IsTerminal`, `GetSize` and `MakeRaw`. Isolated
   from the moves, on a settled tree, it is a mechanical change with a check
   that runs.

4. **Decide the two files whose home is genuinely arguable** rather than leaving
   them where the wave put them by default: `player_panel.go` (a panel over the
   media engine — media or panel?) and `sixel_layers.go` (graphics — media or
   term?). State the reason, not just the choice.
4a. **Close the `share_dialog.go` / `grabber.go` question with a written
   decision.** Phase 10 deferred both to "Task 44 step 2", and the renumbering
   turned step 2 into the dependency-injection proposal, so the question would
   otherwise be reachable only from `phase-10:149` and `HANDOFF:82`. Both files
   sit in `internal/app` with their tests, and neither is held there by need:
   zero references into `app` from either.

   Decide each on the edge it would create, not on its name, and record the
   answer here so a reader walking the steps arrives at it.

5. **Check that no new flat package appeared.** The failure this whole branch
   exists to undo is one package accumulating unrelated code. Verify no extracted
   package holds files from two unrelated subjects, and that `internal/app` holds
   wiring rather than features that found no other home.
6. **Verify the tree against `ARCHITECTURE.md` as rewritten in Task 41** — the
   layer table, the dependency rules, the file-naming convention. Where the code
   and the document disagree, one of them is wrong; say which.
7. **Review the suppressions the branch added, once, as a list.** Measured
   on the finished tree the branch reports 276 lint findings where
   `upstream/main` reports 375. That difference is not error handling
   improving. It is `_ =` and `#nosec` accepted **under pressure from a tool**:
   a rename made an old line new to `--new-from-rev`, the incremental job
   demanded an answer for a line whose behaviour had not changed, and the
   cheapest answer that let the wave continue was a suppression. Sixty-three
   of those, spread across eleven phases, each decided in passing while the
   attention was on moving files.

   `_ =` on an error is a real decision — "there is nobody here to tell" — and
   it is sometimes right; Task 38 closed a state file and a history write that
   way, with the reason written next to each. But a decision taken sixty-three
   times in passing is one that has not been taken.

   **Count them by comparing the trees, not the diff.** A diff over a branch
   that moved 600 files reports every line of a moved file as added:
   `git diff upstream/main...HEAD | grep -E '^\+.*(_ =|#nosec)'` returns 222,
   and most of those merely travelled with their file. The set difference is
   the number:

   ```
   count() { (cd "$1" && grep -rhoE '(^|[^A-Za-z_])_ = |#nosec' --include='*.go' . | grep -c .); }
   ```

   `upstream/main` holds 601, this branch 664 — **63 added**, of which 15 are
   `#nosec` (216 against 231). That is the list to review, and it is a third of
   what the diff suggests. Locate them by running the same grep over both trees
   into sorted files and taking the difference of the *lines*, since the same
   line may exist in both under different paths.

   For each, answer the only question that matters: is there anybody to tell?
   Where there is, handle the error. Where there is not, leave the suppression
   and say why beside it.

   What goes in the PR body is then honest and is an argument in favour of the
   work: "the branch added 63 suppressions under pressure from the incremental
   lint, all 63 were reviewed, M were kept with a stated reason." "We removed 99
   findings" is not an argument, and a maintainer who looks will read it as the
   opposite.

7a. **A test double that embeds a live implementation hides a missing method,
   and neither the compiler nor the test says so.** This is the only silent
   damage in the whole branch that left no trace at all — no build error, no
   failing assertion, no changed line. Copying `mockMetadataVFS` into
   `internal/dialog` with a regexp took its `type` block and left the two
   methods below it. The double embeds `vfs.VFS` and the field is populated
   with `vfs.NewOSVFS(t.TempDir())`, so `Stat` and `SetAttributes` resolved
   through the embedded value to the real filesystem. The package compiled, the
   test ran, and it hung on a two-second timeout — the only evidence was two
   seconds nobody would look at.

   Measured on the finished tree: **21** test doubles embed `vfs.VFS`, and **13**
   of those populate it with a real implementation. Each is one missing method
   away from testing the operating system instead of the double.

   The narrow fix is to copy types with the parser. The general one is that a
   double which embeds an interface should either assert it never reaches
   outward, or embed a struct of panicking stubs instead — the second is
   cheaper and fails on the first call rather than on a timeout. Decide which,
   once, for the thirteen.

8. **Record that the frame manager has a disciplined path that almost nobody
   takes.** `testutil.SwapFrameManager` gives a test a fresh manager and, in its
   returned closure, closes the frames and shuts that manager down;
   `vtui.FrameManager.Init` gives it a fresh screen on the shared manager and
   undoes nothing. Counted on the finished tree:

   | package | `FrameManager.Init` | `SwapFrameManager` |
   |---|---|---|
   | `cmd/f4` | 286 | 100 |
   | `internal/editor` | 161 | 10 |
   | `internal/panel` | 156 | 1 |
   | `internal/viewer` | 18 | 1 |
   | `internal/terminal` | 4 | 3 |
   | `internal/cmdline` | 4 | 0 |
   | `internal/dialog` | 3 | 2 |

   The helper exists because the direct call leaks, and it is taken in a
   minority of cases everywhere and in under one per cent in `internal/panel`.
   This is not a task for this branch — converting three hundred call sites is
   its own change with its own risk — but it is the first thing to propose as a
   follow-up, and the goroutine-leak failure recorded in `index.md` is the first
   bill for it.

9. **Write the outcome into the PR body**, in a short section: what was reviewed,
   what stays as is, and what is proposed as a follow-up with the evidence behind
   it. A reviewer should not have to ask whether the structure was thought about
   after it was built.

### Required Interfaces and Contracts

None. Read-only review.

### Error Handling and Logging

Not applicable.

### Tests

None added. `cmd/f4/architecture_test.go` (Task 6) already asserts the layer
rules; this task reads its result rather than extending it.

### Outcome

Run on the finished tree. Every number below is measured, and where a step's
prediction failed the measurement stands and the prediction is marked.

**Step 1 — the packages as they stand.** `internal/app` 234 files / 63212 lines,
`internal/terminal` 101 / 18631, `internal/panel` 80 / 41756, `internal/editor`
65 / 25280, `internal/media` 40 / 9152. Five packages have a single importer —
`sheet`, `vtvibe` and `paneltest` (`app`), `colorer` (`editor`), `wincon`
(`media`). None is a merge candidate: a subject with its own tests is a package
whatever the importer count, and merging any of them back would put a second
subject into the importer, which is the shape this branch exists to undo.

**Step 1a, verified a second way.** The re-homing above answers "did anything
arrive without a home"; it does not answer "does what arrived still run". The
seventeen test files that came in on merges were checked against
`go test -list '.*'` in the package each now lives in, function by function.
None is invisible — no file landed somewhere it compiles but is never
collected:

`internal/panel/edit_command_test.go`, `internal/app/local_language_files_test.go`,
`internal/dialog/attributes_mixed_test.go`,
`internal/dialog/settings_portable_test.go`,
`internal/editor/external_freebsd_test.go`, `internal/app/sort_groups_test.go`,
`internal/plughost/manager_lifecycle_test.go`,
`internal/ttyx/{keys_coverage,overlay_lifecycle,session_state}_test.go`,
`plugins/archive/sfx_test.go`,
`plugins/id3editor/{plugin_handle,plugin_paths}_test.go`,
`sdk/f4plugin/plugin_test.go`,
`vfs/hostfs/{hostfs_posix,hostfs_windows}_test.go`,
`vfs/registry_vfs_windows_test.go`.

One of them executes nowhere, and not through anything this branch did:
`external_freebsd_test.go` is `//go:build freebsd` and GitHub has no freebsd
runner. The only thing that ever compiles it is the `GOOS=freebsd` vet cell —
which is why that cell failing on a third-party dependency, rather than on our
code, mattered enough to fix.

**Step 2a — `internal/media` stays one package.** Counting package-level symbols
only, the image half and the audio half share **zero** references and image and
video share exactly **one** (`imageViewBackAttr`), so the plan's prediction about
the *contents* held. The plan's own caller test is what fails: `internal/panel`
reaches audio (10 references) and image (6). Every caller uses at least two
families, so there is no boundary behind the split, and a split without a caller
boundary is two packages that always travel together.

The first count was wrong and worth recording as a trap: counting *method* names
put seven image→video edges on the board that do not exist, because `Close`,
`ProcessKey` and `Show` are borne by unrelated types in both halves. The same
name collision that made step 1c need two stages makes a cohesion count need
package-level symbols only.

Cohesion measured the same way for the other clusters: `command_palette*` 13
files / 3024 lines, 11 outward edges / 6 inward — and the outward ones are frame
types and the action layer, which is its subject, not a second one; `sheet*` 4 /
1537, 4 / 3; `vtvibe*` 2 / 1062, 7 / 7; `pty*` 13 / 1729, 8 / 7; `frame*` 8 /
7992, 53 outward; `bootstrap*` 9 / 1870, 38 outward — it *is* the root, and that
number is what a composition root looks like.

**Step 4 — both named files had already moved, and one left a test behind.**
`player_panel.go` is `internal/panel/player.go`: a panel over the media engine
is a panel, and it imports `internal/media` the way any consumer does. The
production half of `sixel_layers.go` is `internal/terminal/sixel.go` and
`sixel_terminal.go`. The test did not follow it — `sixel_layers_test.go` stayed
in `internal/media` holding two subjects: two tests of the terminal's sixel
layering, and two of `blitInto`, which is `internal/media`'s own compositing
helper. It carried private copies of `mockPty`, `sixelEnv`, `newSixelEnv` and
`send`, and its own comment said so: "internal/terminal keeps its own; this copy
goes with the image tests when they leave for internal/media." The image tests
never left, because they were never image tests.

It is the same silhouette as `ParsePlainEditCommand` in step 1a, and the pair
is worth reading together: a separation papered over by copying instead of
moving. There, a function was exported so a test in another package could
reach it; here, a test carried private copies of four fixtures the target
package already declares. A test that needs a private copy of another
package's fixtures is saying where it belongs.

Split along the subjects: the two sixel tests and `sixelHalfBody` are
`internal/terminal/sixel_layers_test.go`, using the `sixelEnv` that package
already has, and the four helper copies are gone; the two `blitInto` tests are
`internal/media/blit_test.go`, beside `TestBlitIntoPlacesAndClips` which was
already there. A test file that needs a private copy of another package's
fixtures is stating where it belongs.

**Step 4a — both stay in `internal/app`, each for a different reason.**
`share_dialog.go` takes a `*panel.PanelsFrame` in three places, and
`internal/panel` imports `internal/dialog` already, so moving it to
`internal/dialog` closes a cycle. That is not a preference; the build would
refuse it. `grabber.go` is 437 lines importing `internal/terminal` and
`internal/keymap`, and all four of its top-level functions —
`NewGrabberFrame`, `OpenGrabber`, `handleForcedMouseSelectionEvent`,
`actionScreenGrab` — have **zero** callers outside `internal/app`. Moving it to
`internal/dialog` would buy one file's tidiness with a new `dialog → terminal`
edge on a package that has none, which is what Task 25 declined once already.
Both stay, and `internal/app` holding an action's own dialog is wiring, not a
feature that found no home.

**Step 5 — no new flat package.** The clusters above are the check: each is a
subject with its own callers, and the one package that reaches everywhere,
`internal/app`, reaches outward 38 times from `bootstrap*` because wiring is
what it holds.

**Step 6 — `ARCHITECTURE.md` and the tree agree.** The layer table diffed
against the tree returns nothing. The document was rewritten from the tree in
Task 41, which is why: it describes a fact, and it carries the check that says
so.

**Step 7 — the 63 suppressions.** Counted on the two finished trees with the
grep this step names: `upstream/main` holds 601 suppressions and this branch
664, so **63 added**, of which **15 are `#nosec`** (216 against 231). Both
halves of the method matter. Counting on the trees rather than on the diff,
because a diff between two points of history shows a line that was added and
later removed, and the question is what stands in the tree now. And counting
the *net*, because a line-by-line set difference of the same two trees reports
168 added lines: a suppression that travelled with its file and had a renamed
symbol in it reads as one removal and one addition, and neither is a decision
anybody took.

The largest groups are ten
`_ = File.Close()`, nine `_ = config.GetF4ConfigDir()`, and six `#nosec G204`
in `internal/panel/process_environment_panel_test.go`, all six on
`exec.Command` with the test's own fixture path. Reviewed: the `#nosec` lines
each carry a reason next to them naming why the input is not untrusted, and the
`_ =` lines are on a `Close` or a config-directory read in a path with nobody
to tell. They stay, and the PR body says so in those words rather than
advertising the 99-finding drop, which is not an improvement in error handling
and would read as one.

**Step 7a — the embedding trap is upstream's pattern, and the branch added
exactly one.** Counted on the finished tree: **38** test doubles embed
`vfs.VFS` anonymously, most of them populated with a live `vfs.NewOSVFS` or
`vfs.NewNullVFS`. `upstream/main` has **37** of them. The one the branch added
is `mockMetadataVFS` in `internal/dialog` — the copy that lost its two methods
to a regexp and silently reached the real filesystem. So the pattern is
inherited, not introduced, and the recommendation stands as a follow-up for the
whole tree rather than as a debt of this branch: a double that embeds an
interface should embed panicking stubs, so a missing method fails on the first
call instead of on a timeout.

**Step 8** is recorded in its own step above; **step 9** carries all of this
into the PR body.

### Acceptance Criteria

- Every extracted package has a recorded file count and caller list.
- `internal/media` has an explicit keep-or-split decision with the caller
  evidence behind it.
- `player_panel.go` and `sixel_layers.go` have a stated home and a reason.
- No package outside the composition root holds two unrelated subjects.
- The PR body carries the review outcome and any follow-up proposals.

### Verification

- `go list -f '{{.ImportPath}} {{join .Imports " "}}' ./...`
- `go test ./cmd/f4 -run TestArchitecture`

---
