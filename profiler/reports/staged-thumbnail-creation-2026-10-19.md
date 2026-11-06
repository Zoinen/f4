# Incremental thumbnail renderer creation

## Implementation and result

GalleryPreviewVisualHost now uses the native GalleryComponentHost. It admits
one pending QQmlIncubator per host, retains the published item until its
replacement is Ready, and cancels superseded work. Incubation uses the engine's
existing controller, which is confirmed present in both candidate captures.
Each pending root is parented and sized before its bindings run, then culled
until publication. The factory's typed parent/preview contract stays intact.
The pending incubator is native state rather than another QML Loader/context.

Cancellation, active-state changes, component errors and destruction clear
pending work. Generation checks prevent an older completion from overriding a
new request. Callback lifetime protection allows the consumer to destroy its
host directly from loaded(). Published roots retain their host parent until
deferred destruction, preserving parent-dependent bindings during retirement.

The previously measured blocking renderer-assignment callback has a much
smaller tail. Clean scrolling still misses refreshes; no reliable whole-scene
cadence improvement or constant 160 Hz is established. Creation work now runs
through Qt incubation. The callback reduction does not mean that all of that
creation CPU work disappeared.

## Regressions and verification

thumbnailReplacementStagesWithoutBlankingReadyItem first fails on the original
Loader: changing sourceComponent asynchronously makes item null instead of
preserving the ready item. The fixed test installs a manually controlled
incubation controller, pauses after the replacement attaches to the host, and
confirms that an actual rendered frame still contains the old red rectangle.
The incomplete green replacement never paints. A third blue request cancels it
and becomes the sole published renderer when incubation advances.

The test verifies resizing both roots while incubation is paused, cancellation
on deactivation and host destruction, and immediate host destruction from the
loaded callback. It checks the actual rendered leaf's scene origin and unit
transform at DPR 1.75. Rendered captures verify the red/blue handoff on Direct3D
and OpenGL; the same regression passes with the software renderer.

An intermediate candidate detached the outgoing root before deleteLater. The
broader suite exposed TypeError warnings when shared factories read their
parent's preview. An extended regression using a parent-dependent binding and
QTest::failOnWarning reproduces this. Culling the retiring root while retaining
its parent fixes it. The final selected hardware logs contain no TypeError,
ReferenceError, binding-loop or invalid-assignment warnings. Existing font and
PinchArea warnings remain.

Direct3D and OpenGL each pass 37 selected Gallery cases, eleven mode cases and
four resource-only import/shader cases. These include all five panel modes,
actual image/shader leaf alignment, SVG surfaces and failure/cancellation,
cross-row factory ownership, recycling, hidden fallback work and resting
scroll alignment. The final extended lifecycle test also passes separately on
Direct3D, OpenGL and software. Both native red/blue captures and the current
real-panel SVG/caption captures were inspected.

The final ownership regression also rejects a non-visual component. It first
finds an unused QtObject still parented to the host after rejection. Cleanup
now holds the incubator's QObject root across clear(), then deletes that root
even when it is not a QQuickItem. The regression verifies Error status, no
published item and no retained rejected child. This final error-path correction
does not change the successful-renderer path used in the scrolling captures.

An initial test invocation omitted the required Basic style and could not load
QtQuick.Controls.Windows; its failure was unrelated to the candidate. The
proper style is set by the final scripts. An attempt to start a test while its
executable was still being linked hit Windows' file lock and produced no test
result. The completed build was awaited before the successful run. Neither
attempt is used as verification evidence.

## Clean display comparison

Fresh-process A1/B1/B2/A2 runs compare the shared-thumbnail-factory baseline
against this change. Both use the original QtQml library and combined QtQuick
renderer, normal GC settings and nearby preparation, the same Downloads
directory with 186 entries, density 152, extent 4101.142857, active 160 Hz
display and DPR 1.75. Input is a real Qt wheel event every 30 ms. Caption
measurements are zero. No QML profiler, WPR, UI Automation observer, census,
build or tests run inside these intervals.

Displayed intervals require both endpoints to lie within the same measured
scroll direction. Each direction lasts approximately four seconds.

| Run | Down display p99 / max ms | Down >9.375 ms | Up display p99 / max ms | Up >9.375 ms |
| --- | ---: | ---: | ---: | ---: |
| A1 | 6.3379 / 12.5093 | 3 | 6.2782 / 12.5023 | 1 |
| B1 | 6.2928 / 12.5047 | 2 | 6.2812 / 12.5075 | 1 |
| B2 | 6.3223 / 12.5128 | 3 | 6.2833 / 6.3756 | 0 |
| A2 | 6.3223 / 12.5099 | 4 | 6.2986 / 6.4375 | 0 |

Candidates have six long intervals versus eight controls. This small difference
does not prove a reliable cadence improvement. All four runs are retained.
Median scene batch counts are 367, 365, 366 and 366 respectively; the scene
complexity is comparable, while the rendered regressions independently verify
correct presentation. These counts alone do not prove pixel equivalence.

## Correlated callback attribution

A separate QML/GC capture on the candidate uses the same logic observer and
default-budget phase-instrumented QtQml library as the earlier shared-factory
capture. Only ranges wholly inside each native direction window are counted.
Nested ranges and their GC overlap are handled as unions. The candidate's
QML/native clock spread is 50 ns, maximum uncertainty 200 ns; every recorded
QML wheel start matches its native handler window.

For GalleryEntryPreview.qml commitComponent at line 282:

| Metric | Shared-factory baseline | Incremental host |
| --- | ---: | ---: |
| Calls during scrolling | 281 | 278 |
| Median non-GC inclusive ms | 0.0040 | 0.00295 |
| p95 non-GC inclusive ms | 0.4206 | 0.0124 |
| Maximum non-GC inclusive ms | 1.5520 | 0.0240 |
| Total non-GC inclusive ms | 36.3369 | 1.2010 |

Both maximum inclusive durations equal their non-GC values. The expensive
synchronous creation tail has left that assignment callback. Ordinary QML
binding ranges elsewhere still reach 0.8754 ms, and incubation remains real GUI
work governed by Qt's controller. This is attribution evidence, separate from
the clean display results above.

The candidate has 1274 native frame periods, median 6.2354 ms, p95 7.2415 ms,
p99 8.3491 ms and maximum 12.8265 ms. Five exceed 9.375 ms. Three overlap GC
states; two do not. The worst interval overlaps 8.5185 ms of GC-state work.
Event-loop mark draining reaches 5.2649 ms. These are observed native callback
periods with profiling overhead, not clean displayed intervals. GC remains a
confirmed major target, and its atomic operations are not bounded by this
renderer change.

## Portable rollout

Windows system-DLL audit and both resource-only QML import/shader gates pass.
The host payload is regenerated and embedded into a CGO_ENABLED=0 launcher using
the system Go build cache. Payload validation, extraction, reuse, concurrency,
corruption and embedded-host lookup tests pass. The worktree launcher is
replaced after these checks and launched directly with --gui=qt, with diagnostic
overrides cleared. The extracted child's hash is verified against the normal
host. Only the worktree process tree is terminated; the canonical checkout is
untouched.

Normal host SHA256: 7461c30bdb23f0b90dfdeb82c69a47ca220e141a04634bf60aaef91d3676187a.
Launcher SHA256: 2050c16636b66f7c24d2e2d0e22534fe6c9422903dd25e3b8acd4c27f08eedc9.

Compact measurements, joined attribution, rendered images, expected failing
and passing tests, source snapshots and reproduction scripts are stored with a
SHA256 manifest in profiler/experiments/staged-thumbnail-creation-2026-10-19.
Executables, raw QTD/CSV traces and build output stay outside that archive.
No commit or push. The constant-160-Hz goal remains active and unachieved.

> AI assistance has been used to create this output.
