# Integrate native batch retention into the portable Qt recipe

## Change

`ci/qt_quick_stable_intervals.py` contains eleven exact before/after source
fragments for Qt 6.11.1, derived from the previously measured renderer.
`ci/patch-qt-dependencies.py` embeds the fragments in the exported Conan recipe
after both existing batch-order and blocked-opacity patches. Source preparation
loads the renderer once, verifies each anchor occurs exactly once, applies the
replacements in memory, then saves the completed result. An unknown or duplicate
anchor fails before that save. The exported recipe needs no helper file or
artifact directory at source-preparation time. All six native-package cache
keys in the workflow now include the replacement data.

The implementation is the previously tested stable-interval/batch-retention
renderer, without a new global-padding default, buffer handoff, initial-buffer
memory-policy change, or GPU-upload budget. Native buffers keep their original
batch ownership. Its behavior and current displayed-frame measurements are in
[the lifecycle report](retained-renderer-lifecycle-2026-10-18.md).

The expanded native regression is now enabled in its normal source file rather
than only archived. The target's default offscreen CTest environment skips
hardware-renderer assertions; the native results below explicitly enable them.

## Regression and source reproduction

The new recipe tests initially fail because integration is absent. Full-source
execution then catches a replacement-data escaping issue that the first
synthetic fixture missed: Python nonraw literals consumed a C++ preprocessor
line continuation. Raw literals preserve it, and the fixture now asserts that
continuation explicitly. All 19 dependency-recipe tests pass afterward.

The recipe tests cover self-contained export, Python syntax and idempotence,
ordering after both predecessor patches, missing/ambiguous predecessor blocks,
strict source composition, and unknown/duplicate source-anchor rejection.
They run with the standard library and do not require Conan or a Qt checkout.

A separate local source gate executes the generated source body with actual
Conan load/save/replace functions against the full Qt 6.11.1 renderer. Its
output is byte-for-byte identical after newline normalization to the measured
prototype source. That recipe output is compiled with the same static MSVC
contract and used for the normal host link. No change is made to the installed
Conan package in place. The local CMake imported-library override points at
`artifacts/qt-stable-intervals-recipe/Qt6Quick.lib`.

The first multi-target build encounters the existing malformed long response
file for ZoinGallerySessionTests. Building the host and changed native test
object separately, followed by the existing response-file linker helper,
completes successfully. The running last working app remains alive throughout
the build. This does not claim the unrelated build-response issue is fixed.

## Verification

Using the library compiled from recipe output:

| Check | Result |
|---|---|
| Dependency recipe tests | 19 pass |
| Native batch/order/lifetime tests at DPR 1.75, default Windows backend | 10 pass |
| Same native tests, OpenGL | 10 pass |
| Focused SVG session tests, all five panel modes included, DPR 1.75 | 26 pass |
| Focused video-field rendered-pixel and metadata-row tests, DPR 1.75 | 4 pass |
| Embedded-resource imports and compiled-host QML startup | 4 pass |
| Windows system-DLL audit | exit 0 |
| Tagged embedded payload/materialization/selection tests | pass |

Logs are `stable-recipe-after.txt`, `stable-recipe-native-175.txt`,
`stable-recipe-native-opengl-175.txt`, `stable-recipe-svg-175.txt`,
`stable-recipe-modes-175.txt`, `stable-recipe-imports.txt`, and
`stable-recipe-portable-audit.txt` under artifacts. The SVG suite retains the
existing authored-animation pause warning; all listed cases pass. This is
focused verification, not a passing claim for every repository test or every
supported OS/driver. Prior independent fixture failures remain documented in
the preceding GUI-fix reports.

## Portable worktree rollout

The static host is packaged into the generated compressed payload. The Go
launcher is built with CGO disabled, GOWORK off, the embedded-host tag, and the
system Go cache `C:/Users/xs/AppData/Local/go-build`. Tagged payload tests pass
before replacing the running worktree binary.

New host SHA256:
`a6a5aa0c5951a58621278e55de0396ec85fdddb8f2583ae74ac4ff2e749629b4`.

Staged and installed worktree launcher SHA256:
`9fb4ea50a4fa86422ac0fa1b2bc6354f52274fd452119a7997c7f615c0d7440f`.

Only the process tree belonging to `D:/Code/f4-zoin/f4-zoin.exe` is terminated.
The staged binary is copied after termination, its hash is checked, then it
launches directly with `--gui=qt`. Profiling, Qt scale, renderer-backend,
order-padding and diagnostic-host overrides are absent. The extracted child
has the exact new host hash and title `f4 [zoin_branch/cached-pixel-grid]`;
startup stderr is empty. Evidence is
`artifacts/stable-recipe-normal-identity.json` and the corresponding normal
startup logs. No canonical checkout binary is replaced.

The frame measurements supporting retention are unchanged because the recipe
source matches the measured candidate. They show reduced spikes, not the
6.25 ms end state: candidate downward display p99 remains around 12.5 ms and
maximum around 25 ms. The 160 Hz goal remains active. First-time large SVG
geometry uploads remain the next separate renderer bottleneck to address.
Cross-platform release gates have not been exercised in this Windows turn.

AI assistance has been used to create this output.
