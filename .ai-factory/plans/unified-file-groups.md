# Unified File Groups implementation

Accepted design: the user-provided plan in this chat, 2026-10-18.
Testing: yes. Documentation: yes. Diagnostics: opt-in vtui.DebugLog; never log command contents.

## Decisions

Pure-Go domain and Settings UI; Qt is an optional renderer/catalog adapter.
Independent highlighting, icon, sorting and association facets. Shipped INI
defaults plus profile-local highlight.ini field overrides; stable group IDs.
Preserve legacy mask syntax and association command states on migration.
Several named command sets per group, all six slots executable through actions.
Native/plugin icons win unless the group's off-by-default override flag is set.
Live reversible appearance/sorting preview; commands remain committed until Apply.
No user configuration writes before Apply. Preserve unknown INI data and comments.

## Tasks

- [x] 1. Shared pure-Go models, layered/lossless document, compiled matching and tests.
- [x] 2. Safe legacy migration, transactional store, factory sources and tests.
- [x] 3. Highlighting/sorting/association runtime integration and six action slots.
- [x] 4. Go Settings editor, nested associations, icon themes, reset and match preview.
- [x] 5. Reversible live preview, revision conflicts, failure and lifecycle tests.
- [x] 6. Qt data-driven icon catalogs, transport and centralized precedence.
- [x] 7. Terminal/Qt regression coverage, 175% rendering, performance, docs, build/run (known pre-existing gallery-suite crash recorded below).

## Delivery boundaries

Keep Go foundation/UI changes separable from Qt adapter changes for upstream.
Do not commit, push or create a PR without a further user request.
Do not include existing screenshots, binaries, diagnostics or QA artifacts.

## Verification so far

- CGO_ENABLED=0: filegroups, settings, theme, panel, app and both SDK packages pass.
- Full internal/app short suite: passes (88 seconds; editor UI drain can wait 30 seconds).
- Architecture audit and go vet for changed domain/UI/SDK packages: pass.
- Windows amd64 pure-Go filegroups test binary cross-compiles.
- At 175% scale: icon provider 37 passes/1 platform skip; document surfaces 70 passes/3 fixture skips; focused gallery icon priority tests pass.
- A full gallery suite run crashed in viewerOwnsEscapeAndZoom; focused icon/appearance checks pass. Do not describe the full Qt suite as green.
- Isolated portable-profile GUI smoke: File groups opens, Add/edit works, Escape cancels the unapplied edit; no normal user profile modified.
- Final build and repeated domain/Settings/theme/SDK tests pass; go vet, architecture/docs checks and git diff --check pass.
- Final 175% run: icon provider 37 passes/1 platform skip; document surfaces 70 passes/3 fixture skips; focused gallery tests 5 passes.
- The gallery crash stack is ExternalCatalogMetadataPlanner::collectCatalogRows(), identical to the pre-existing 2026-10-17 macOS crash report; gallery submodule left unchanged.
- Final isolated GUI smoke confirms File groups renders and Escape closes without applying; isolated process then closed. Normal running app/profile untouched.
- 10,000 predicate matches: 3.3 ms, zero allocations per pass (Apple M5 Pro).
- Verification warnings: golangci-lint is unavailable; no roadmap artifact/linkage exists. Full gallery suite is not green due to the independently confirmed pre-existing crash.
