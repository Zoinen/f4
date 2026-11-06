# Masonry: preprocessing eligibility reuse

## Outcome

Constant 160 Hz without spikes remains **unachieved**. This turn implements and
tests a flat cache of preprocessing eligibility, then captures eight clean
cases: four cold rapid-wheel cases, a verified-warm wheel pair and a
quarter-height middle-button pair. The preceding goal turn made progress through
bounded vector-owner resolution; its tested implementation is enabled in both
arms here.

Cold before-renderer median work falls from .2273–.2734 ms to .1917–.2257 ms.
Warm middle before-renderer medians fall from .1257/.1395 ms to .0318/.0346 ms.
Tails remain mixed. Cold controls have five display intervals over 9.375 ms;
candidates have ten. All cold and warm wheel blocks fail the unchanged maximum
two-physical-pixel motion gate. The candidate passes both strict warm-middle
blocks in this run; the control passes one and misses a refresh in the other.
This finite comparison does not establish a population-level display win or
regression, nor a hard frame-time bound.

No production library, embedded portable payload or normal application is
replaced. No commit or push. The cache remains a diagnostic candidate.

## Reason and implementation

The earlier scheduler/CPU trace finds 367 preprocessing self samples in
`QSGNodeUpdater::isNodeBlocked`. Every eligible node repeats an ancestor walk,
even when relevant scene state is unchanged. The earlier per-ancestor hash
cache added too much bookkeeping and remains rejected.

This experiment changes the caller in `QSGRenderer::preprocess`. It preserves
the existing QSet snapshot, its iteration order, callback calls, and the
`m_nodes_dont_preprocess` deletion guard. A flat vector stores each snapshot
position's node pointer and blocked result. Reuse requires matching root,
updater, revision, snapshot size and node pointer at that exact position.
Otherwise the original `isNodeBlocked` executes. There is no per-node hash
lookup, ancestor mapping, sorting, pixel rounding or reordering of callbacks.

At most eight renderer records per render thread and 16,384 logical entries per
record are accepted. Excess renderer count, excessive snapshot size and nested
use of the same renderer fall back to the original query. Vector/container
allocation overhead is not claimed to equal logical entry payload exactly.
No source lease, bitmap, GPU resource or owning node reference is retained.

`QSGRootNode::notifyNodeChange` advances a thread-local revision before any
renderer callbacks for subtree-blocking, opacity, node addition/removal,
preprocessing flags or force-update changes. Matrix, geometry and material
changes alone do not change eligibility. Qt's existing dirty-state contract and
the application SceneTransformNode's explicit blocking notifications are
preserved. A relevant notification during preprocessing immediately forces
fresh queries for subsequent entries and prevents publishing that pass's cache.
Deletion-guard skips also prevent cache publication. Wraparound invalidates
stored revisions, including active scopes.

Root additions/removals clear the renderer record even when delivered directly
through the renderer API. Updater replacement and renderer destruction release
the record. Replacement during an active pass invalidates and retires the
storage at scope exit, preventing the scope from keeping a freed State pointer.
Diagnostic counters aggregate locally and publish per scope, rather than using
atomics at every ancestor step or root notification.

The feature flag is `F4_PROFILE_PREPROCESS_ELIGIBILITY`. The final executable
strongly overrides only the original node/renderer archive objects; no SDK file
is overwritten and no class layout or public/wire protocol changes. Renderer
source starts from the already-tested Qt curve-fill preprocessing patch, not
unpatched upstream code, so the existing no-op-fill filtering remains present.
Original sources, their hashes and reversible replacements are preserved.

## Correctness and lifecycle tests

Nine helper results pass: stable reuse, root exclusion, mid-pass invalidation,
iteration-order mismatch, different roots/updaters, snapshot changes, skipped
deleted slots, ignored visual changes, revision wraparound, nested same-renderer
fallback, renderer/entry capacity and active-updater replacement retirement.

The native fixture runs inside Qt's actual preprocessing loop. It chooses a
later node from the actual randomized QSet snapshot, avoiding an assumed hash
iteration order. It changes blocking and opacity, reparents a callback into and
out of a blocked subtree, destroys/reconstructs a later callback at the exact
same address, and changes UsePreprocess while the snapshot is active. It checks
callback counts, original snapshot deferral and actual rendered pixels. A
separate afterRendering observation records the first affected pass, so an
extra automatic frame cannot conceal an incorrect same-pass callback.

The fixture passes three results at actual DPR 1.75, including explicit
cleanup: zero live renderer-cache slots after window destruction. It checks
the geometry item's physical origin (14,14) and unit scene transforms; the
rendered red geometry leaves have integer physical edges. Blocked, visible and
opacity captures were inspected. With the feature off, every behavioral/pixel
check passes and only the explicit final assertion that eligibility reuse ran
fails.

Additional final checks pass:

- 48 native scene results in each arm, zero failures/skips.
- 27 shader results and three embedded-only QML import results.
- Three native vector-owner topology results with both caches present.
- Windows system-DLL-only dependency audit.

No static product UI layout changes. These native tests verify D3D11 at actual
175% scale, not other backends or packaged deployment. The normal application
and its portable payload remain unchanged.

Two setup failures are retained honestly. The initial helper build used a
member named `slots`, which Qt's keyword macro consumed; it was renamed. The
first native control fixture incorrectly required an unblocked subtree's
callback to run in the current snapshot. The original renderer instead adds it
for the next snapshot. That expectation was corrected, while stale-callback,
deletion, reparenting and reuse checks remain. Neither was a production crash.

## Capture conditions

Actual Downloads, all 191 entries, Masonry density 152, actual 3840×2160 at
160 Hz and DPR 1.75. Each case contains two ten-second gesture blocks.
`artifacts/f4-qt-host-preprocess-eligibility-20261029.exe` is used in all arms.
The eligibility flag is the sole arm variable. The earlier AV private-heap,
thumbnail decoder-device reuse and bounded vector-owner resolution diagnostics
remain enabled in both arms. Converter-resource reuse and the conversion lane
are absent. The original 100-entry asynchronous geometry pool, renderer budgets,
pixel/motion logic, media handling, vector content and input paths remain.

No build, test, offline analysis, ETW scheduler sampler or UI Automation observer
runs during gestures. Display, content, focus/input, resource bounds, feature-use
and error guards pass. Owned diagnostic instances are stopped by exact launcher
path between cases. OS file caches are not flushed.

Cold cases use normal cold scrollbar setup and fresh caches zzv/zzw/zzx/zzy in
A1/B1/B2/A2 order. Warm cases use fresh caches named
`cold-gallery-cache-20261029-eligibility-{warm,middle}-{control,candidate}`.
Their normal overlapping tour visits every one of the 191 indices, with a
settled presentation at each point. All four warm cases record zero pending
bitmap requests, new bitmap stores, bitmap misses and decoded-frame count
changes during measurement. This proves the tour and bitmap quiescence; it does
not independently count every SVG source read/generation. Middle displacement
is exactly one quarter of window height and inactive checks are zero.

Lossless NTFS compression of older completed scheduler exports recovers capture
space. Before/after hashes match; unique traces are not deleted and the disk
guard is not lowered. The compression receipt is preserved.

## Cold rapid-wheel results

Cases: `real-scroll-preprocess-eligibility-{a1,b1,b2,a2}-20261029-wheel-masonry`.

| Arm | Block | Before-body median ms | Preparation median / max ms | Display max ms | Intervals >9.375ms | Motion max physical px |
| --- | ---: | ---: | --- | ---: | ---: | ---: |
| A1 control | 0 | .2707 | .7075 / 7.2590 | 12.4921 | 2 | 42.312 |
| A1 control | 1 | .2273 | .6370 / 6.2197 | 6.8305 | 0 | 9.556 |
| B1 cache | 0 | .2211 | .6269 / 4.7395 | 12.5171 | 2 | 100.114 |
| B1 cache | 1 | .1924 | .5770 / 6.2649 | 12.4906 | 4 | 99.786 |
| B2 cache | 0 | .2257 | .6370 / 7.5649 | 12.4056 | 1 | 74.209 |
| B2 cache | 1 | .1917 | .5734 / 9.4805 | 12.7460 | 3 | 5.215 |
| A2 control | 0 | .2734 | .7082 / 5.5766 | 12.5208 | 1 | 69.825 |
| A2 control | 1 | .2591 | .6922 / 6.8579 | 12.5220 | 2 | 111.749 |

All have zero coverage holds. Before-body p99 and full preparation tails remain
mixed: this saving in medians is not a claim that the largest CPU work is fixed.
One control geometry unmap reaches 1.7189 ms; candidate unmaps in these clean
captures remain below .40 ms. No scheduler trace attributes those particular
events, so no lock or driver-causation claim is made from this comparison.

Process-cumulative counters, including startup:

| Arm | Reused answers | Original walks | Relevant notifications | Peak logical entries | Live records |
| --- | ---: | ---: | ---: | ---: | ---: |
| A1 | 0 | 1,151,592 | 0 | 0 | 0 |
| B1 | 399,614 | 728,617 | 62,148 | 402 | 1 |
| B2 | 381,777 | 709,240 | 74,094 | 396 | 1 |
| A2 | 0 | 1,112,509 | 0 | 0 | 0 |

No capacity bypasses occur. Roughly 35% of candidate queries reuse eligibility.
Live invalidated-pass counts are zero: notifications here occur outside active
preprocessing, while the separate native fixture verifies mutations inside it.
The final snapshots precede process termination, so created=1/destroyed=0 is
expected; explicit window-destruction tests independently verify cleanup.

## Verified-warm wheel and middle results

Cases: `real-scroll-preprocess-eligibility-{warm,middle}-{control,candidate}-20261029-wheel-masonry`.

| Gesture / arm | Block | Preparation median / max ms | Display max ms | Intervals >9.375ms | Motion max physical px | Strict pass |
| --- | ---: | --- | ---: | ---: | ---: | --- |
| Wheel control | 0 | .4478 / 2.3992 | 6.4224 | 0 | 2.9046 | No |
| Wheel control | 1 | .4453 / 2.8251 | 6.4100 | 0 | 2.9163 | No |
| Wheel cache | 0 | .4271 / 2.5518 | 6.4000 | 0 | 2.3929 | No |
| Wheel cache | 1 | .4069 / 2.1440 | 7.0957 | 0 | 13.3629 | No |
| Middle control | 0 | .4301 / 2.5155 | 6.4225 | 0 | .4970 | Yes |
| Middle control | 1 | .4057 / 2.6129 | 12.5045 | 1 | 17.8128 | No |
| Middle cache | 0 | .2968 / 2.4156 | 6.4734 | 0 | .6359 | Yes |
| Middle cache | 1 | .3026 / 2.2590 | 6.4100 | 0 | .4685 | Yes |

Coverage holds are zero throughout. Native middle velocity error is at most
.0007284 px for control and .0009807 px for candidate, below the unchanged
.002 px limit. Both candidate middle blocks satisfy all current display, motion,
velocity and coverage checks. A single control middle miss does not establish
that this cache fixes every warm-middle stall.

Warm wheel medians change only modestly. Middle before-body medians change from
.1395/.1257 ms to .0318/.0346 ms, but their p99 values remain similar. Cumulative
warm-wheel candidate counts are 444,568 hits / 843,311 walks, peak411 entries;
middle candidate counts are 871,908 hits / 379,505 walks, peak384 entries. These
include the warm-up tour and startup and are not pure measurement-window rates.

## Next evidence needed

Keep the cache as a tested CPU candidate with its conservative invalidation and
lifecycle regressions. Full fidelity cold and warm-wheel gates still fail.

The warm wheel result separates a remaining issue from large preparation cost:
its candidate preparation never exceeds 2.5518 ms, all display intervals remain
below 9.375 ms, yet the motion gate fails. The worst candidate block1 pair has a
**5.3967 ms displayed interval but a 6.25 ms animation-clock step**, producing
13.3629 physical pixels of error. A control worst pair has 6.4016 ms versus
6.25 ms, producing 2.9046 px. The analyzer independently checks that the recorded
curve/clock explains the rendered camera, and excludes retargets/geometry rebases.
This does not establish whether the displayed-time variation is actual visible
jitter or timestamp/observation variation.

The next useful capture should compare independent graphics/vsync timing with
these PresentMon and animation timestamps before changing the animation clock
or relaxing any gate. Cold video-conversion context contention and distributed
renderer work remain separate unresolved paths. A favorable median or the two
passing middle blocks do not complete the original goal.

Sources, source hashes/reversible hooks, tests (including retained setup
failures), receipts, all eight capture packets/presentation CSVs, native images
and analysis are frozen under
`profiler/experiments/masonry-preprocess-eligibility-2026-10-29`. Large files are
losslessly compressed and external binaries/objects are hashed. The normal
worktree application remains alive with its original binary hash; all owned
diagnostic instances are stopped. The goal stays active.

AI assistance has been used to create this output.
