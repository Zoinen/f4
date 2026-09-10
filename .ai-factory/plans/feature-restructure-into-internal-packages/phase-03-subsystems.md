# Phase 3: Self-Contained Subsystems Under `internal/`

Plan: [index.md](index.md)
Tasks: 16-17
Depends on: Phase 2

## Objective

Six directories that are already proper Go packages with an acyclic dependency
graph move from the repository root to `internal/`, where the compiler keeps them
module-private. No package is split, no identifier is renamed, no file gains or
loses a line of logic — only import paths change.

This phase is deliberately placed before the `cmd/f4` waves: it is the largest
purely mechanical change in the plan, it touches no `package main` file except
its import block, and getting it out of the way means the wave commits contain
nothing but their own subject.

## Current-Code Evidence

| Path | Go files | Importing files | External deps |
|---|---|---|---|
| `piecetable/` | 6 | 51 | none (leaf) |
| `textlayout/` | 3 | 5 | `f4/piecetable` |
| `sheet/` | 6 | 4 | none |
| `fusefs/` | 13 | 6 | `f4/vfs` |
| `vtvibe/` | 9 | 4 | `f4/vfs` |
| `luaplug/` | 10 | 2 | none |

Verified: across all six, the only intra-module imports are
`github.com/unxed/f4/piecetable` and `github.com/unxed/f4/vfs`. None imports
another of the six, and none imports `cmd/f4` (it could not — `package main`).
The move is therefore a pure path rewrite with no ordering constraint between the
two tasks.

`internal/hideconsole` is **not** part of this phase and must not be touched. It
is a vendored fork of `github.com/ebitengine/hideconsole` with its own `go.mod`
and its own module path, substituted through `replace` at `go.mod:187`. Its module
path is not ours to rewrite.

## Files to Change

| Path | Action | Required change |
|---|---|---|
| `internal/piecetable/`, `internal/textlayout/`, `internal/sheet/` | create (move) | From the root |
| `internal/fusefs/`, `internal/vtvibe/`, `internal/luaplug/` | create (move) | From the root |
| every file importing one of the six | modify | Import path only |
| `docs/VTVIBE.md` | modify | 34 path mentions |
| `docs/` (piecetable, textlayout, fusefs pages) | modify | One file each |
| `AGENTS.md` | modify | Project Structure block |
| `.ai-factory/rules/base.md` | modify | Module Structure list |

---

## Task 16: Move `piecetable`, `textlayout` and `sheet`

### Intent

Three subsystems with no filesystem dependency: the piece table backing the
editor, the text layout and wrapping engine over it, and spreadsheet mode. They
are leaves or near-leaves and are consumed only from `cmd/f4`.

### Implementation Steps

1. `git mv piecetable internal/piecetable`
2. `git mv textlayout internal/textlayout`
3. `git mv sheet internal/sheet`
4. Rewrite the import paths across the module. The three old paths are
   unambiguous prefixes, so a scripted rewrite is safe:
   ```
   grep -rl 'github.com/unxed/f4/\(piecetable\|textlayout\|sheet\)' \
     --include='*.go' . |
     xargs sed -i '' \
       -e 's|github.com/unxed/f4/piecetable|github.com/unxed/f4/internal/piecetable|g' \
       -e 's|github.com/unxed/f4/textlayout|github.com/unxed/f4/internal/textlayout|g' \
       -e 's|github.com/unxed/f4/sheet|github.com/unxed/f4/internal/sheet|g'
   ```
   (`sed -i ''` is the BSD/macOS form; on GNU `sed` use `sed -i`.)
   `textlayout` imports `piecetable`, so its own files are rewritten by the same
   pass.
5. `gofmt -l ./internal ./cmd ./plugins` and fix any import block the rewrite left
   unsorted. `goimports` grouping matters here: the paths move from the
   third-party group into the same group as the existing `internal/` imports.
6. Sweep the prose: `grep -rn 'piecetable/\|textlayout/\|sheet/' docs/ README.md AGENTS.md .ai-factory/rules/base.md`
   and update every hit — one `docs/` page each for piecetable and textlayout, the
   `AGENTS.md` Project Structure block, and the Module Structure list in
   `rules/base.md`.

### Required Interfaces and Contracts

- Package names are unchanged: `package piecetable`, `package textlayout`,
  `package sheet`. Only the import path changes.
- No exported identifier is renamed. Far-derived names in particular stay as they
  are.
- `internal/textlayout` → `internal/piecetable` is the only edge among the three,
  and it stays. Both are layer 0/1 per the dependency rules.
- After the move, `sdk/` and `vfs/` must still not import any of them — Task 8's
  auditor rule 1 covers this.

### Error Handling and Logging

None. A missed import path is a compile error.

### Tests

`_test.go` files move with their packages automatically — they are inside the
moved directories. No test is rewritten.

```
go test ./internal/piecetable/... ./internal/textlayout/... ./internal/sheet/...
```

### Acceptance Criteria

- `ls piecetable textlayout sheet` all fail.
- `grep -rn 'github.com/unxed/f4/piecetable"' --include='*.go' .` returns nothing
  (note the closing quote: it excludes the new `internal/` path).
- `gofmt -l .` returns nothing.
- The suite matches the Task 1 baseline.

### Verification

- `CGO_ENABLED=0 go build ./...`
- Expected result: exit 0.
- `go test -timeout 25m ./...`
- Expected result: identical to `.ai-factory/RESTRUCTURE_BASELINE.md`.
- `go test ./cmd/f4 -run '^TestArchitecture'`
- Expected result: `ok` — the graph is still acyclic and nothing imports upward.

---

## Task 17: Move `fusefs`, `vtvibe` and `luaplug`

### Intent

The remaining three subsystems: FUSE mounting, the vtvibe session/provider layer,
and the Lua plugin engine. `fusefs` and `vtvibe` sit on `vfs`, which stays public
and is unaffected; `luaplug` is a leaf that `internal/plughost` will own as its
Lua transport.

### Implementation Steps

1. `git mv fusefs internal/fusefs`
2. `git mv vtvibe internal/vtvibe`
3. `git mv luaplug internal/luaplug`
4. Rewrite the import paths, same shape as Task 16 step 4, for the three paths.
5. `gofmt -l ./internal ./cmd ./plugins` and fix import grouping.
6. **`docs/VTVIBE.md` is the largest single documentation edit in this plan** — 34
   path mentions. Do it here, in the commit that creates the drift, not in the
   Phase 11 docs pass. A surviving reference is an unfinished move.
7. Sweep the rest: `grep -rn 'fusefs/\|vtvibe/\|luaplug/' docs/ README.md AGENTS.md .ai-factory/rules/base.md`.
8. Check the build-tag files survived: `fusefs` carries per-OS files, and a
   `git mv` of a directory preserves them, but confirm the count is unchanged
   (`find internal/fusefs -name '*.go' | wc -l` = 13).

### Required Interfaces and Contracts

- Package names unchanged: `package fusefs`, `package vtvibe`, `package luaplug`.
- `internal/fusefs` → `vfs` and `internal/vtvibe` → `vfs` stay. `vfs` remains a
  root-level public package; nothing about it changes in this task.
- `internal/luaplug` keeps its FFI arrangement untouched. The race workflow's
  comment at `build.yml:1395` notes that luaplug can now run its FFI tests under
  the detector — that stays true, and the `packages` scope picks up the new path
  automatically.

### Error Handling and Logging

None.

### Tests

```
go test ./internal/fusefs/... ./internal/vtvibe/... ./internal/luaplug/...
go test -race ./internal/luaplug/...
```

The race run is worth doing separately here: `luaplug` is the one moved subsystem
that CI exercises under the detector.

### Acceptance Criteria

- `ls fusefs vtvibe luaplug` all fail.
- `find internal/fusefs -name '*.go' | wc -l` returns `13`;
  `internal/vtvibe` `9`; `internal/luaplug` `10`.
- `grep -c 'cmd/f4\|vtvibe/' docs/VTVIBE.md` shows no stale root-level path.
- `gofmt -l .` returns nothing.

### Verification

- `CGO_ENABLED=0 go build ./...`
- Expected result: exit 0.
- `go test -timeout 25m ./...` and `go test -race ./internal/luaplug/...`
- Expected result: identical to the Task 1 baseline.
- Cross-compile spot check, because `fusefs` is the most platform-dependent of the
  three:
  ```
  for t in linux/amd64 darwin/arm64 windows/amd64 freebsd/amd64 openbsd/amd64 \
           netbsd/amd64 dragonfly/amd64 illumos/amd64 solaris/amd64; do
    GOOS=${t%/*} GOARCH=${t#*/} CGO_ENABLED=0 go build ./... || echo "FAIL $t"
  done
  ```
- Expected result: no `FAIL` line.

---

## Phase Risks and Mitigations

- **Risk:** the scripted `sed` rewrite also matches a longer path that happens to
  start with one of the six names (e.g. a hypothetical `sheetview`).
  **Mitigation:** all six old paths are followed immediately by `"` or `/` in a Go
  import; verify with
  `grep -rn 'github.com/unxed/f4/\(piecetable\|textlayout\|sheet\|fusefs\|vtvibe\|luaplug\)[a-z]' --include='*.go' .`
  before running the rewrite — it must return nothing.
- **Risk:** `internal/hideconsole` is swept up as "another subsystem to move".
  **Mitigation:** it is named in this phase's evidence section as explicitly out of
  scope; its `go.mod` and the `replace` at `go.mod:187` make any move a two-file
  change, which is a different task nobody has asked for.
- **Risk:** `docs/VTVIBE.md`'s 34 mentions are partially updated and the document
  describes two different trees.
  **Mitigation:** grep the document for the old path after editing and require
  zero hits before committing.
- **Risk:** a platform-tagged file in `fusefs` is dropped by an interrupted move.
  **Mitigation:** the file-count check in Task 17 step 8 and the cross-compile loop
  in Verification.

## Phase Completion Checklist

- Every Task 16-17 satisfies its acceptance criteria.
- Six directories have left the repository root; `internal/hideconsole` is
  untouched.
- No `.go` file outside `internal/` references the old paths.
- `docs/VTVIBE.md`, the piecetable/textlayout/fusefs pages, `AGENTS.md` and
  `.ai-factory/rules/base.md` describe the new locations.
- `go test -timeout 25m ./...` matches the Task 1 baseline.
- `index.md` task checkboxes 16-17 are ticked.
