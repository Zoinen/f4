# Lazy video badges and remaining scroll stalls — 2026-10-19

## Result

Constant 160 Hz without spikes remains unachieved. This iteration makes progress:
video badge subtrees are instantiated only for successfully decoded video
thumbnails; regression and rendered-image checks pass; the portable worktree
application is rebuilt and running. A six-run clean comparison does not establish
a reliable cadence improvement. New sampled traces identify both GC and native
preparation/render work in remaining misses. No commit or push was made.

## Implementation and regression evidence

GalleryEntryPreview previously created the play badge even on ordinary files.
Its subtree now comes from one shared GalleryEntryVisualFactories.videoBadge
factory, through the existing typed GalleryPreviewLoader. Readiness uses the
existing videoThumbnailReady predicate. Removing video readiness destroys the
badge; restoring readiness recreates it. No GC settings, ownership, source
requests, prefetch priority or native rendering-library changes are promoted.

The moved factory accesses its row through preview.entry. An Item surface absorbs
Loader sizing, while its child badge preserves its independent circular extent.
Before that wrapper, the regression measured badge widths 252.5714 versus
49.7143 expected (landscape), 80 versus 28 (portrait), and 142.2857 versus 49.7143
(square). These failed results are retained. The final tests check all three
aspects, the 76 logical-pixel diameter cap, actual badge/circle/glyph/caption
scene-space physical origins and unit transforms at DPR 1.75, and rendered output.
The five-mode no-badge regression first failed on the eager implementation;
it now also verifies that video metadata alone does not create a badge.

The transition regression exposed a separate rendering bug: thumbnail geometry
could be suppressed while its sibling shader still painted the image. The
shared shader factory now follows thumbnailItem.sceneVisible and excludes
vector sources. The new pixel regression failed before that fix and passes
after it: the thumbnail centre changes from the image to the underlying
selection surface, its caption remains visible, and revealing restores the
original pixel. Native culling means Qt isVisible alone cannot prove drawing.

Older transition assertions needed correction for existing asynchronous shared
component creation and physically rounded layout: tests wait for the committed
GalleryComponentHost.item, reject retired branches, expect no Grid backdrop,
and bound aspect error by one physical pixel per axis. Failed intermediate
logs are archived separately, including the actual painted suppression failure.

Selected QtTest totals on each of Direct3D11 and OpenGL at actual DPR 1.75:
27 layout engine, 37 modes, 7 session, 3 embedded-import, and 6 video/transition
cases pass (totals include setup/cleanup). This is not a claim that the full
suite passes; the previously reported unrelated legacy Details logical-coordinate
expectation remains unresolved. Captures were inspected, including play glyphs,
small captions and suppressed/visible thumbnail states.

## Retained objects

Separate native census captures use Downloads, 186 entries, density 152,
160 Hz, DPR 1.75. Census traversal runs after the input measurement windows.

| Measurement | Before | After |
| --- | ---: | ---: |
| All retained QObjects | 27,243 | 25,742 |
| Named badge roots | 269 | 4 |
| Named non-video badge roots | 264 | 0 |
| Named badge subtree QObjects | 1,544 | 28 |
| QQmlComponents | 258 | 264 |

Named badge counts cover active-presentation names, not every unnamed preview
in the application. Pool/lifecycle snapshots differ slightly, so the complete
1,501 QObject reduction is not an exact per-badge allocation attribution.
The shared factory adds definitions per retained panel; component count alone
does not measure the saved subtree/context/binding work.

## Clean timing comparison

Fresh processes run A1/B1/B2/A2/A3/B3. A is the shared-fallback implementation
before lazy badges; B includes lazy badges and the shader suppression fix.
Each run contains two four-second boundary-bounce wheel windows, wheel interval
30 ms, actual 160 Hz and DPR 1.75, same Downloads entries/density and 5 ms GC
budget. Frame recording, census, WPR and UI Automation are disabled. No builds,
tests or offline analysis overlap these captures. Each run has one Composed:
Flip swapchain. Input activity/motion are validated; caption measurements and
top-level model/snapshot signals are zero. Both displayed interval endpoints
must lie inside an input window.

| Run | Display gaps >9.375 ms | Maximum gap, ms | Block p99, ms |
| --- | ---: | ---: | --- |
| A1 | 5 | 12.5846 | 6.3421, 6.2809 |
| B1 | 4 | 12.5007 | 6.3407, 6.2957 |
| B2 | 4 | 18.7461 | 6.3465, 6.2996 |
| A2 | 3 | 12.4975 | 6.3096, 6.3218 |
| A3 | 4 | 12.5043 | 6.3156, 6.2901 |
| B3 | 3 | 12.5048 | 6.3218, 6.3065 |

A totals 12 long intervals over 24 seconds, B totals 11. The one-event
difference and B's 18.7461 ms worst interval do not establish improvement.
Near-6.25 ms p99 values can coexist with visible rare spikes. The object
reduction and correctness fixes are verified independently of cadence.

## Sampled attribution and next work

A separate current-build cold/warm capture uses bounded GC/root, native
preparation and incubation observers plus WPR sampling and PresentMon.
It is diagnostic evidence, not a release timing comparison. Same engine/PID
is retained for cold/warm windows. Exact raw QPC matching has 900 ns offset
spread and all observer arrays remain below their bounds. GUI/render CPU
durations overlap and must not be added as serial work.

Warm marking visits fall from 142,123 in the preceding shared-fallback trace
to 139,493; recursive QObject descendant visits fall 26,560 to 25,077.
There is one warm collection in both. Recorded warm maximum MarkDrain is
5.4294 ms and sweep 4.7611 ms; significant GC latency remains. Cold collection
count differs (three versus two), so total cold marking counts are not a
per-collection speed comparison.

Selected exact producer intervals associated with display misses:

| Stage / ending QPC ns | Producer ms | Display ms | GC union ms | GUI running ms | Render running ms | GPU ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Cold / 146439802253300 | 15.4979 | 12.5574 | 8.3200 | 14.7439 | 4.2229 | 0.4237 |
| Cold / 146439825784500 | 14.7837 | 18.7347 | 2.4264 | 10.8688 | 6.7237 | 0.4969 |
| Warm / 146460232973400 | 6.7528 | 12.5029 | 0 | 1.8678 | 5.6488 | 0.5509 |
| Warm / 146461703289600 | 11.3948 | 12.4774 | 0 | 5.2260 | 3.8898 | 0.4720 |

The first warm miss has no incubation and only 0.4855 ms native work. Render
samples include QSGAreaAllocator, batch invalidation and D3D resource staging.
The second warm miss has no incubation/GC, but native work union is 4.0538 ms,
including 3.2825 ms nearby preparation, 0.4242 ms properties and 0.2526 ms row
preparation. Nested scope durations are inclusive, not additive.

The next investigation should separate retirement/release, row binding, and
retained-vector maintenance inside prepareNearbyDelegates. Its nominal 2 ms
timer starts after retirement/release and is checked only before complete rows;
the whole operation therefore has no demonstrated 2 ms bound. Establish actual
phase costs and scheduling relative to render deadlines before changing limits.
Keep useful prefetch, retained thumbnails and successful-only image navigation.
Render resource/scene maintenance and atomic GC remain independent constraints;
do not replace this evidence with an arbitrary GC constant or blanket rounding.

## Portable rollout and archive

The normal static host passes the system-DLL audit and embedded-only QML import
test. Payload regeneration, Go extraction/materialization tests and launcher
build succeed with CGO_ENABLED=0, GOWORK=off and system GOCACHE. Only this
worktree's launcher process tree is replaced. Direct --gui=qt launch, staged
launcher equality, extracted-child host equality and empty startup stderr are
verified. Canonical D:/Code/f4/f4.exe is untouched.

The SHA-256 manifest and runtime identities are in
profiler/experiments/lazy-video-badges-2026-10-19. It retains source snapshots,
failed and final checks, rendered captures, six clean summaries, census and
compact sampled attribution, scripts and binary hashes. Executables, libraries,
raw ETW/PresentMon CSVs, build outputs and generated payloads are excluded.

AI assistance has been used to create this output.
