# Lazy layout diagnostics and retained QObject census — 2026-10-18

## Result

The previous goal turn removed hidden thumbnail fallback requests and initial
fallback objects. This turn measures retained native objects, then removes a
different producer: disabled layout-diagnostic rectangles retain a border QObject
in every instance. They now materialize their Rectangle only while visible.
Controlled tests prove the disabled/visible/disabled lifetime; a live native
census confirms the removed children. Clean frame results remain mixed and
constant 160 Hz remains unfinished.

`GalleryLayoutDebugZone.qml` becomes a Loader, keeping zone, color, geometry and
stacking behavior. An explicit `borderColor` replaces the implicit Rectangle
border object; the folder-preview customization is updated to use it.
`GalleryLayoutDebugPaint.qml` owns the actual Rectangle and border and is listed
in the existing static QML module. Its URL resolves in the defining QML file,
including when the caller uses an inline context. No per-frame rounding or
layout arithmetic changes are introduced. Debugging remains available and is
off by default.

## Native census

An artifact-only diagnostic walks QObject children and visual item children from
the window and engine, deduplicating pointers. Class-name QML suffixes are
normalized. It counts reachable native objects, not all JS heap allocations or
every unparented script-owned object. The scan and JSON construction occur only
after the measured frame arrays have been copied, with frame capture inactive.
Clean comparisons disable the scan entirely. No production observer is added.

Both census captures use Downloads, 184 entries, density 152, 30 ms actual wheel
events, active 160 Hz, DPR 1.75, D3D11 and normal GC settings.

| Reachable objects | Before | After |
| --- | ---: | ---: |
| All counted QObjects | 32350 | 31058 |
| Diagnostic zone roots | 1211 | 1207 |
| Children retained under diagnostic zones | 1211 | 0 |
| QQuickPen instances | 1911 | 698 |
| QQmlComponent instances | 4025 | 4013 |

There are slightly different materialized rows, so the full 1292-object
difference is not all attributed to this patch. The stronger invariant is one
unnecessary child per disabled diagnostic before, versus none after. This is
about 1200 fewer objects at this working-set size, without replacing each border
with another eagerly constructed component. QQmlComponent counts remain a larger
retained-graph target for further investigation.

## Tests and pixel verification

The new test fails on the old implementation with three retained QQuickPens
when zero are expected. The final test verifies no borders while disabled,
creation of all three diagnostic region types when enabled, preserved default
and overridden fill/border colors, whole physical origins/extents and unit
transforms at DPR 1.75, a rendered-color sample, and destruction when hidden.

The existing all-mode diagnostic test now examines actual Rectangle descendants,
not only Loader wrappers, and captures each presentation. It uses the real
window DPR and waits for three rendered frames with stable origins and settled
motion before checking the leaves. The first draft exposed its legacy fixture's
explicit DPR 1 (a y edge measured 10.5 physical pixels); a subsequent immediate
check caught an in-progress layout transition at x 11.5. Correcting the fixture
and waiting for rest resolves both, with no production rounding added.

The host's import-isolation test now enables this lazy component and verifies
the paint child loads with only embedded import roots. A gallery-only fixture
does not perform the host's complete QtQml resource bootstrap, so import isolation
is tested in the proper host target. The new standalone diagnostic test still
checks its own full lifecycle and geometry.

- Gallery suite: 34 passed D3D11 and 34 passed OpenGL at 175%.
- Mode/motion suite: 13 passed, including all five debug presentations, Grid
  captions, fallback opacity-node preservation, five wheel modes, resting
  alignment and Columns text.
- Host import isolation and shader-resource audit: 4 passed on both backends,
  now including activation of the new paint resource.
- Static Windows system-DLL audit passes.
- Embedded payload generation/materialization/reuse/concurrency/corruption/lookup
  tests pass after packaging.

The rendered three-region fixture and actual Masonry screenshot are inspected,
including thin borders and small captions. Other mode captures remain in the
workspace. Test development also catches an initially caller-relative URL and
the folder-preview border customization before deployment. A Qt moc issue with
the inline fixture body is isolated behind Q_MOC_RUN; its test slot remains
registered and the executable runs the full body.

## Clean displayed cadence

Four fresh processes use the same renderer, workload, scale, default GC budget
and 30 ms wheel cadence. No QML profiler, WPR, tests, build or census scan runs
during these comparisons. All windows stay active. Display intervals include
both endpoints only inside the same direction window.

| Run | Down p99 ms | Down max ms | Down >9.375 ms | Up p99 ms | Up max ms | Up >9.375 ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| A1 | 6.2978 | 18.7515 | 3 | 6.3030 | 6.3387 | 0 |
| B1 | 6.3515 | 12.5399 | 6 | 6.2796 | 12.5100 | 3 |
| B2 | 6.2754 | 12.5063 | 2 | 6.2698 | 6.3096 | 0 |
| A2 | 6.3149 | 12.5150 | 4 | 6.3175 | 12.4931 | 4 |

Both versions total 11 long displayed intervals. B1 is worse than A1 by count;
B2 is better than A2. No consistent frame-time gain, reduced GC pause duration,
or constant-160-Hz result is claimed. This change removes verified retained
objects but substantial component/binding work remains.

## Rollout and evidence

The static host is rebuilt, its compressed payload regenerated and a portable
CGO_ENABLED=0 launcher built with the system Go cache. The live diagnostic restores
panel state before replacing only the worktree's own launcher tree. The normal
app runs directly with `--gui=qt`; diagnostic overrides are cleared. The extracted
host matches the new build and startup stderr is empty. Canonical F4 is untouched.
No commit or push is made.

- Host SHA256: `9B879D99F1E5220A0AF6A9A66F2509E6C97EFB3772E626C727C6EEB99589A335`
- Launcher SHA256: `A83CA4B29AD4927C1718B298A6F076816088E255681E2B9994D77612613D1131`

`profiler/experiments/lazy-layout-debug-2026-10-18/` contains native census/frame
captures, all four strict display summaries, expected-failure/passing tests,
rendered evidence, scripts and a manifest. Raw PresentMon traces, binaries and
build outputs remain untracked in artifacts. Deployment identity is recorded in
`artifacts/lazy-debug-normal-identity.json`.
