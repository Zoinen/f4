# Coherent recycled-row commits — 2026-10-19

## Result

Recycled previews now reopen only after their new identity, indexes, geometry,
presentation and visibility are installed. The verified portable worktree build
is running. Selected checks pass on Direct3D11 and OpenGL at actual DPR 1.75.
Diagnostic GC-free preparation costs improve, and a clean alternating trial has
15 baseline versus 10 candidate long display intervals. Refresh misses remain;
constant 160 Hz without spikes is not achieved. No commit or push was made.

## Reproduction and implementation

The preceding diagnostic work identified snapshot and index notification as
substantial native preparation costs. GalleryDelegateItem::setVisualRow guarded
the identity replacement, but immediately reopened the preview before the layout
published final row geometry. The following index notification and layout changes
then retriggered the active preview's bindings.

The new recycledPreviewReopensAfterRowGeometryCommit regression observes actual
identity-guard releases while scrolling 300 entries through recycled slots. It
requires at least one observed reassignment and compares each reopening's item
geometry against the committed index geometry. Before the fix, examples include
row 160 reopening at y=8798 instead of 6286.29 and row 161 at y=6 instead of
6286.29. The failed output is preserved. Initial harness runs also exposed a
missing Basic style environment and the helper's explicit DPR 1 setting; those
are corrected to use Basic and the actual window DPR. The final geometry check
has no tolerance that could mask an old-slot position.

GalleryDelegateItem gains nested beginVisualUpdate/endVisualUpdate boundaries.
An identity change still closes the existing guard before snapshot publication,
but its release waits for the outer update to end. Both visible-row commits and
nearby preparation use a QScopeGuard through identity, visual, geometry and
visibility publication. Standalone setVisualRow keeps its original behavior;
same-entry changes do not close the preview. Old-entry pixels are still cleared
before replacement bindings run. No new QML layout, GC tuning, ownership change,
prefetch reduction or source acquisition path is introduced.

The native header changes class size, so affected test objects are rebuilt
against the new ABI before relinking. Older diagnostic core archives are not
linked into the new host. A fresh phase library is generated from current
sources and preserves the matching native ABI.

## Verification

On each backend, selected QtTest totals pass: 27 layout engine, 41 modes,
7 session, 3 embedded-only imports and 7 interaction/video cases. Totals include
setup/cleanup. The expanded cases cover recycled geometry publication, nearby
ready delegates, retained SVG scenes, nested guards, standalone identity reset,
same-entry metadata changes, all five modes, thumbnail replacement, transition
suppression, and physical leaf origins/unit transforms at 175% scale.

Rendered captures are retained and inspected, including small Details text,
icons, play glyphs/captions and transition surfaces. This does not claim the full
test suite is green: the previously documented legacy Details logical-coordinate
expectation remains unresolved and is not silently relabelled as passing.

## Clean display comparison

A is the verified lazy-badge host before this native change. B adds the coherent
commit boundary. Fresh processes run A1/B1/B2/A2/A3/B3 with two four-second
boundary-bounce wheel windows each, 30 ms wheel input, Downloads 186 entries,
density 152, actual 160 Hz and DPR 1.75, and the unchanged 5 ms GC budget.
Native frame recording, object census, WPR and UI Automation are disabled. No
build/test/offline analysis overlaps capture. All runs have one Composed: Flip
swapchain, active input and validated motion, zero caption measurements and
zero top-level model/snapshot notifications. Display intervals require both
endpoints inside the measurement window.

| Run | Display gaps >9.375 ms | Maximum display gap, ms | Block p99, ms |
| --- | ---: | ---: | --- |
| A1 | 4 | 12.5034 | 6.3429, 6.3325 |
| B1 | 4 | 12.5045 | 6.3419, 6.3103 |
| B2 | 4 | 12.5128 | 6.2911, 6.3048 |
| A2 | 6 | 12.5043 | 6.3790, 6.3032 |
| A3 | 5 | 12.6069 | 6.3589, 6.2719 |
| B3 | 2 | 12.5008 | 6.2920, 6.2841 |

A totals 15 long intervals over 24 seconds, B totals 10. This is encouraging
and consistent with reduced preparation work, but the small event counts and
remaining misses do not prove stable improvement across folders or constant
160 Hz. Similar near-6.25 ms p99 values still hide individual missed refreshes.

## Native phase comparison

Separate native-only cold/warm diagnostic captures use bounded preparation,
incubation and GC scopes with PresentMon. The before capture is nearby-notify;
after is row-commit-phases, built under a distinct immutable host/map name.
The latter explicitly measures endVisualUpdate/reopening. Both retain process
and engine identity across cold/warm windows; no forced collection is used.
All arrays remain below their caps. These are instrumented samples, not the
clean cadence results above, and are not exclusive scheduled CPU measurements.

The comparison selects measurement-window nearby calls that prepare rows,
contain a replacement snapshot and have zero GC overlap. Costs are normalized
by each call's prepared-row count. Whole-call cost includes shared retirement
and maintenance overhead; binding+layout+reopen sums disjoint scopes and excludes
nested snapshot/index scopes to avoid double counting.

| Stage / metric, ms per prepared row | Before median | After median | Before p95 | After p95 |
| --- | ---: | ---: | ---: | ---: |
| Cold whole nearby call | 1.0205 | 0.7049 | 1.7756 | 1.1331 |
| Warm whole nearby call | 1.0240 | 0.64375 | 1.64885 | 1.0801 |
| Cold binding + layout + reopen | 0.8150 | 0.5597 | 1.4728 | 1.0186 |
| Warm binding + layout + reopen | 0.7884 | 0.5338 | 1.3713 | 0.8275 |
| Cold index notification | 0.2508 | 0.0245 | 0.4401 | 0.04495 |
| Warm index notification | 0.2754 | 0.02645 | 0.4903 | 0.17095 |

Before cold/warm sample counts are 192/204 calls; after 193/208. This is not a
paired per-row microbenchmark; row mix and OS scheduling vary. Reopening has a
real cost and is included: after warm median 0.1297 ms per prepared row, p95
0.3238 ms, maximum 1.4205 ms. The speedup is not inferred merely by moving work
outside the previously measured bindVisual scope.

## Remaining work

GC still creates single-frame overruns. The new cold 7.3012 ms nearby call
contains 6.0258 ms GC, including a collection triggered inside layout. A new warm
6.5344 ms call has 5.6753 ms GC nested inside snapshot publication. Other calls
have no GC: a cold 4.0304 ms call includes 1.3357 ms release work and 0.9465 ms
preview reopening across two rows. The nominal row-loop timer does not bound
the full operation or an individual synchronous notification.

Next inspect allocations and repeated notifications that remain during parked
slot retirement/reassignment, with the coherent boundary now in place. Keep
the correctness regression and include the whole commit when measuring. Atomic
GC and the independently observed render-thread resource/scene costs remain
constraints; another arbitrary GC or row-count limit is not evidence of a fix.

## Portable rollout and evidence

The normal static host passes the system-DLL audit and embedded-only QML imports.
The payload is regenerated; Go extraction/materialization tests and the launcher
build pass with CGO_ENABLED=0, GOWORK=off and the system Go cache. The worktree
launcher starts directly with --gui=qt. Staged launcher equality, extracted-host
equality and empty startup stderr are verified. Canonical D:/Code/f4/f4.exe is
untouched. Runtime identity and binary hashes are retained.

Archive: profiler/experiments/coherent-row-commits-2026-10-19. The SHA-256 manifest
covers sources, generators, diagnostic scope data, final/failed tests, rendered
captures and all six clean summaries. Before notification scopes are also
preserved in nearby-preparation-parts-2026-10-19. Executables, libraries, raw
ETW/PresentMon CSVs, build outputs and generated payloads are excluded.

AI assistance has been used to create this output.
