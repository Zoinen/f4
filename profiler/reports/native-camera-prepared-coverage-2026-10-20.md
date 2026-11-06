# Prepared-content safety for native scrolling

The native scrolling experiment now checks individual expected cells before
advancing its camera. Preparing leading delegates toward the current input
target removed all observed safety-limit hits in two unthrottled 40-second
captures. The second capture still missed two display refresh intervals.
Constant 160 Hz without spikes is not achieved.

## Why the previous coverage metric was insufficient

A bounding union of prepared delegates can contain the entire viewport even
when a cell in one column is missing. A prepared tile in a neighboring column
can extend the union beyond that missing cell. Measuring only the union's top
and bottom therefore underestimated camera outruns.

`MasonryLayout::viewportMotionCandidates` queries the layout's existing spatial
index in content coordinates. It does not apply the active camera correction
used for pointer hit testing. The diagnostic snapshots candidates in a bounded
three-viewport window during synchronization, when GUI objects are locked.
Prepared nodes are matched by their assigned view index and facade readiness.

`GalleryViewportMotionRange` subtracts missing expected cells from the allowed
contiguous camera-offset interval around the GUI pose. Missing cells above and
below the viewport constrain motion in the corresponding direction. A missing
cell at the anchor invalidates the interval; empty layout space and cells in
other columns outside the viewport do not. The render thread consumes only
the resulting numeric range. It does not query GUI objects on native frames.
When the diagnostic reaches a limit, it rebases the animation segment at the
limited position, preventing accumulated time from causing a catch-up jump.

With this limit enabled but the ordinary 48-logical-pixel delegate margin,
the live camera was constrained in 32, 20, 15 and 14 frames across the four
measurement blocks. The maximum deferred displacement was 28.651 logical
pixels. The anchor was ready in every sampled frame. Display timing alone
showed zero missed refreshes, yet camera samples included additional stationary
transitions. This is why zero missed presents alone cannot establish smoothness.

## Leading preparation

`GalleryViewportMotionState` now publishes the GUI input target as well as the
committed render position. While a native camera is active, the Masonry
materialization plan extends its delegate window toward that target, capped
by the existing metadata look-ahead. Metadata/decode demand is unchanged.
The normal path, without an active native camera, retains its original plan.

The extension is consumed on the next delegate/layout commit. A production
controller still needs to prepare its initial target window explicitly when
arming motion, and to handle target-only changes without assuming a GUI pose
change will always trigger a commit. The experiment continues to redirect an
ordinary logical animation to a dummy target; it is not that controller.

The safety limit remains enabled in the measurements below. No input target
or delay is allowed to bypass the cell-readiness check.

## Measurement conditions and results

Windows D3D11, Qt 6.11.1, 160 Hz display, DPR 1.75, 2194 x 1186 window,
Downloads with 186 entries, density 152, and wheel commands every 30 ms.
Each case has two 10-second cold and two 10-second warm blocks. The normal
5000 us GC budget is preserved. The general frame recorder is disabled;
bounded native samples and PresentMon supply the evidence. No builds, tests,
analysis or UI Automation observation ran during a capture.

A display interval over 9.375 ms is counted as a missed-refresh interval.
This counts long intervals, not the total number of refreshes lost within them.
Safety-limit hits are render samples constrained by missing-cell coverage.

| Case | Cold display misses | Warm display misses | Safety-limit hits, all blocks |
| --- | --- | --- | --- |
| Ordinary margin, safety enabled | 0 / 0 | 0 / 0 | 32 / 20 / 15 / 14 |
| Target-leading preparation | 0 / 0 | 0 / 0 | 0 / 0 / 0 / 0 |
| Same executable, GUI updates at least 16 ms apart | 0 / 1 | 0 / 1 | 0 / 0 / 0 / 0 |
| Repeat target-leading preparation, unthrottled | 0 / 0 | 2 / 0 | 0 / 0 / 0 / 0 |

The first unthrottled target-leading run had display p99 values 6.308, 6.301,
6.326 and 6.327 ms, with a worst display interval of 6.681 ms. The repeat's
warm p99 values rose to 8.618 and 8.645 ms, with a worst interval of 12.226 ms.
No cause is assigned to that timing variation from these measurements alone.
Both unthrottled runs have one stationary transition per block, corresponding
to the initial pose, and no coverage constraints. Snapshot eligibility does
not prove every descendant has submitted its pixels; rendered checks remain
necessary, especially during source/layout replacement.

Throttling is not selected: there is no demonstrated display-timing benefit,
and the same-binary 16 ms variant increases median GUI/native pose lag to about
71-72 logical pixels. The unthrottled leading run's medians were about 29-39
pixels (repeat 33-45). Throttled lag reached 188.368 logical pixels. Pointer
queries use the committed native pose, but chrome and lifecycle integration
remain unfinished.

Leading preparation has a measured cost. The first unthrottled run's snapshot
scope medians were about 0.257-0.262 ms and maxima up to 0.723 ms; native
application scopes had medians 0.007-0.009 ms and maxima up to 0.315 ms. The
repeat snapshot maximum was 0.814 ms. Snapshot timings are deduplicated by sync
ID. These are instrumented wall-clock scopes, not exclusive CPU measurements;
do not sum overlapping scopes or infer the cause of a missed refresh from a
maximum alone.

## Verification and rollout status

The selected coverage/materialization tests pass: seven checks including
setup and cleanup, zero failures or skips. They cover a missing cell inside a
misleading prepared union, a missing anchor, gaps, both directions, bounded
target preparation and the unchanged ordinary materialization contract.

The camera hit-test regression also verifies that the bounded content query
matches layout cell intersections and stays independent of the active camera.
The selected hit-testing, raster and SVG camera tests pass on D3D11 and OpenGL
at DPR 1.75: five checks including setup/cleanup per backend, no failures or
skips. Native camera images are compared with settled GUI references, and
actual caption, secondary-caption and raster leaves have scene-position and
unit-transform assertions. The current rendered SVG capture was inspected,
including the small secondary text and thin authored white stroke.

The freshly relinked static import-isolation test passes (three checks, no
failures or skips). The current diagnostic host passes the Windows import
audit. This is not a full-suite or packaged-launch claim. The known previous
Details visual-contract failure remains outside the selected test set.

No embedded payload was deployed; the normal application has not been opted
into the native camera. Remaining work includes a production motion controller,
initial preparation, cancellation/settlement, transactional rebasing for
source/layout changes, hidden tabs, resize/DPR changes, software fallback and
chrome/cursor coordination. The remaining refresh spikes still need attribution
and broader repeated verification. No commit or push was made.

Evidence is indexed by
`profiler/experiments/native-camera-prepared-coverage-2026-10-20/manifest.json`.
Capture names are `rt-panel-coverage0`, `rt-panel-coverage-lead`,
`rt-panel-coverage-lead16`, and `rt-panel-coverage-lead-repeat`. The last three
use the same executable; each capture has a build-identity sidecar. The exact
prototype source for the first safety-only case is retained separately and
its hash is verified against that capture's sidecar.

> AI assistance has been used to create this output.
