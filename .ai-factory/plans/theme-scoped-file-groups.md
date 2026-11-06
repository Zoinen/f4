# Theme-scoped and universal file groups

Implementation of the user-approved plan. Work in the existing Qt worktree;
preserve prior changes. No commits, pushes or PRs.

## Tasks

- [x] Go model: IconTheme scope; immutable effective snapshots; reuse compiled predicates; universal-before-theme order; runtime theme independent of previews.
- [x] Version 3 persistence and in-memory v2 migration: explicit factory migration data, neutral generic defaults, per-theme catch-alls, stable universal extraction, backups/conflicts/comments.
- [x] Go editor: active-theme label/filter/scope, scope-aware add/duplicate/move, retained hidden drafts and committed-only associations.
- [x] Qt adapter: persisted theme in initial hello before first scene; optional small context notifications; no catalog retransmission; scope-aware fallback.
- [x] Regression coverage: theme round trips, independent features/six commands, migration/layering, preview/Apply/Cancel/conflicts, legacy highlighting/tree, 10,000 entries and Qt at 175%.
- [x] Update format/ExtUI documentation; build and smoke-check locally. Do not commit/push.

## Contract

Empty IconTheme is universal; Native ID is system. A frontend with no declared
theme uses generic. Universal rules precede only the chosen thematic set. Scope
affects every facet, not merely icons; per-theme icon references remain separate.
Inactive sets survive persistence and hidden drafts. Color styles stay independent.
Masks compile on document edits, never on theme changes or painting. Theme changes
refresh existing entries without re-reading directories or dropping panel state.
Cancel rolls back group previews, not the active application icon theme.

## Verification notes

CGO_ENABLED=0: all-package build, affected package suites, go vet and Windows
amd64 filegroups test compilation pass. The full short suite found missing
File groups category translations (fixed and rechecked for all bundled languages).
Its remaining failure is the unchanged HEAD VFS sudo-callsite auditor:
TestEveryElevatedCallChecksTheOperation, vfs/os_vfs.go:529. Reproduced separately;
neither the source nor the auditor was changed by this work. golangci-lint is
not installed; go vet and repository architecture/localization auditors pass.

Qt: controller suite 31 passed at 175%; icon suite 38 passed with native Cocoa;
focused gallery/icon precedence suite 5 passed; mixed-DPI physical raster test
3 passed (window DPR 1.75 / maximum screen DPR 2). Full Gallery suite is not
claimed: the existing ExternalCatalogMetadataPlanner crash is outside this task.
Live Settings filtering was checked through keyboard navigation. The test label
was removed after a live Apply; the profile is now v3 with its original
highlight.ini.pre-filegroups backup retained. Final startup leaves its hash unchanged.
