# Direct profiling of GalleryScrollAnimation

The real controller still misses refresh deadlines. Its cold run also cancels
motion four times when layout geometry changes. This is a repeatable correctness
and continuity problem that the earlier diagnostic hook did not exercise.
Native motion remains disabled in the normal app; constant 160 Hz is unfinished.

## Workload and provenance

Four fresh-process captures ran in this order: native, GUI, native repeat, GUI
repeat. Each contains two 10-second cold blocks and two 10-second warm blocks.
Both paths therefore have 80 measured seconds. Cold means the first scroll in
a fresh process, not an empty disk or thumbnail cache.

Conditions were the Downloads folder with 186 entries, Masonry density 152,
160 Hz display, DPR 1.75, 2194 x 1186 logical window, 30 ms wheel interval,
bouncing wheel direction, default 5000 us GC budget and the shared diagnostic
pixmap-cache limit of 2 MiB. General frame recording was disabled. Every
recorded wheel event reported an active window. Process IDs in each PresentMon
CSV match the corresponding capture, and each measured block contains enough
presents/display samples to reject an incomplete capture.

All eight stage identity sidecars identify one executable. There is no
`RtPanelHook`, dummy target or alternate camera implementation in its main.
The normal `GalleryScrollAnimation.cpp` was copied and instrumented, then
linked as an explicit object before the ordinary static Core archive. Linking
used no multiple-definition override; the original controller object was not
pulled as well. Its public API and QML type metadata remain the real ones.
The profile selects the actual QML controller before the input window begins.

The probe uses a fixed 65,536-record buffer with release/acquire publication;
it is pre-touched before input. JSON export happens after measurement. Native
frames record poses, job/revision IDs, coverage bounds and snapshot/application
wall scopes. GUI events record start, cancellation reason and finish. The probe
adds an identifier to private state and has nonzero recording overhead. These
are instrumented-host results, not exact uninstrumented production latencies.
No builds, tests, analysis or UI Automation observation ran during the input
windows. The ordinary working application stayed alive throughout.

## Display timing

A displayed interval greater than 9.375 ms is counted as long. This counts
intervals, not the number of refreshes lost: an approximately 18.75 ms interval
can span three 160 Hz refresh periods.

| Capture | Cold block 1 / 2 | Warm block 1 / 2 | Total long intervals |
| --- | --- | --- | --- |
| Native | 3 / 0 | 1 / 0 | 4 |
| GUI | 7 / 2 | 3 / 1 | 13 |
| Native repeat | 5 / 0 | 1 / 1 | 7 |
| GUI repeat | 3 / 1 | 2 / 0 | 6 |

Aggregates are 11 for native and 19 for GUI over 80 seconds each, but individual
runs overlap. These samples do not establish a reliable timing improvement.
The worst native display interval was 18.876 ms; the worst GUI interval was
18.735 ms. Native warm blocks still contain approximately 12.45-12.47 ms long
intervals. Zero spikes is not verified.

## Repeatable cold motion interruption

Both native cold captures started six jobs. Their first measurement block used
jobs 1 through 5; their second used job 6. All four interruptions are explicitly
recorded as `layoutBandsChanged()` cancellations with `commitPose=false`.
Warm captures add only jobs 7 and 8, one per measurement block.

In the first cold run, gaps between the last commit of an old job and the first
commit of its replacement were 25.734, 25.771, 19.473 and 19.410 ms. The repeat
had 32.122, 33.675, 19.500 and 33.894 ms gaps. These are native-job commit gaps;
they do not claim that no other GUI content was rendered during that time.

The first two original handoffs also have logical-coordinate discontinuities
of -20.136 and -46.336 pixels. The repeat has discontinuities of -18.256,
-21.835, zero and -7.505 pixels. Geometry changes can change coordinate spaces,
so these values alone are not an exact measurement of on-screen displacement.
The cancellation nevertheless discards the active trajectory and waits for a
subsequent input to start another job. Continuity must be verified against a
visible entry anchor when this is fixed.

This is not merely an unnecessary notification. `rebuildLayoutBands()` emits
the signal after its geometry index changes. In `rewrapMasonry()`, that signal
comes before content extent and the final viewport anchor have been updated.
Handling that signal alone is too early to translate the old camera into the
new layout's coordinate space. Removing the guard without rebasing would be
unsafe and could expose stale culling or poses.

## What is not limiting these runs

Every measured native block has zero coverage-limited frames and zero unready
anchor frames. Initial target-leading preparation is doing its job for this
workload; increasing its margins is not justified by these results. Up to 154
prepared roots and nine SVG gates were observed, so the capture exercised the
real vector presentation path as well as file/raster scenes.

In the first native capture, snapshot-scope medians were approximately
0.265-0.296 ms and the maximum was 1.082 ms. Native application-scope medians
were 0.018-0.023 ms, with a maximum of 0.739 ms. These are wall scopes, not
exclusive CPU costs. They do not alone explain the remaining warm long
intervals. GUI synchronization, allocation/collection and scheduling outside
these scopes remain candidates requiring attribution; none is declared the
cause from this capture.

The native matrix correction error stayed below 0.000126 logical pixels in
the first run. This is a matrix-consistency check, not complete scene/pixel
verification or a substitute for the rendered regressions.

## Next implementation requirements

Preserve genuine metadata-driven geometry updates while keeping motion alive.
Do not simply remove `layoutBandsChanged()` cancellation or postpone all correct
aspect ratios until scrolling stops.

1. Add a before/after geometry transaction around the full rewrap, including
   extent and anchor restoration. Begin from the committed native pose rather
   than its lagging GUI shadow, using the old geometry for anchor capture.
2. Rebase the running trajectory, current pose, prepared/culling regions and
   QML planned target into the new coordinate space together. Preserve the job
   and elapsed trajectory time; handle endpoint clamping when content shrinks.
3. Prevent queued GUI callbacks from publishing old-space poses while a new
   geometry revision is pending. Keep source/path, mode, resize and DPR safety
   boundaries until each has an appropriate transaction and tests.
4. Add a regression using real metadata/aspect-ratio updates while the native
   camera leads GUI state. Verify the visible entry's viewport phase, one job,
   no false running-state transition, SVG readiness/culling and final rendered
   physical-pixel alignment at 175%.
5. Repeat these captures after that fix, then attribute the residual warm
   spikes. Cursor/chrome, sticky headers, middle-button auto-scroll and broader
   workload verification remain necessary before enabling native motion by
   default or claiming the 160 Hz goal.

## Evidence and runtime state

`artifacts/modes-profile/real-scroll-findings.json` contains per-window display
statistics, native scopes, cancellation events and job transitions. Capture
prefixes are `real-scroll-native`, `real-scroll-gui`,
`real-scroll-native-repeat` and `real-scroll-gui-repeat`.
The archive manifest is
`profiler/experiments/real-scroll-controller-profile-2026-10-20/manifest.json`.
Raw executables/libraries are represented only by hashes.

Two comparison setup attempts failed before capture: an executable-copy sharing
lock after process termination and an unavailable nested-shell `Get-FileHash`.
The launcher now waits for the old process, avoids rewriting an identical Go
binary and computes hashes through the .NET API. Those failures are not
included as timing samples.

No production source change or new payload deployment was made in this step.
The normal worktree app remains the previously verified embedded build with
native motion disabled. No commit or push was made.

> AI assistance has been used to create this output.
