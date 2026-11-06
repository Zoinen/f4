# Masonry: retained cache capacity, foreground events and GUI input work

Constant 160 Hz remains **unachieved**. This turn fixes a demonstrated
retained-capacity problem in the diagnostic incremental cache and verifies it
natively. Foreground observations show that low wheel-event counts can occur
without focus loss. The renderer's earlier CPU saving remains visible, but
the new cold captures still contain missed intervals and large motion errors.
No production library/payload, normal application, commit or push is changed.

## Capacity fix, reproduced first

Repeatedly growing ancestor dependency sets to 512 entries and retiring all but
one leaves the previous prototype retaining 9,216 capacity units for a graph
whose live-graph bound is 2,176. The new regression fails on that exact previous
header before the fix. The first compile attempt stopped on a full D: drive;
that output is preserved separately and is not the regression result.

The fix trims a sparse QSet only when its capacity exceeds the larger of four
times its current size and Qt's measured single-entry minimum capacity. It
does so during graph insertion/removal, without adding a per-frame graph walk.
Per-graph capacity accounting tracks insertion, removal, moving an affected
set out during structural invalidation, rollback and reset. Retired path-vector
storage is released as before. Counters are aggregated after preprocessing.

The independent capacity-sum check now passes. Additional checks cover
structural retirement and partial capacity-admission rollback. The invariant
for stored dependency sets is:

`sum(set.capacity()) <= 4 * activeEdges + minimumCapacity * activeAncestors`.

This is a container-capacity bound, not an exact measurement of allocator or
driver bytes. Outer table and record capacities remain bounded by the existing
record/ancestor limits. Logical limits are unchanged: 16,384 records, 32,768
ancestor keys and 131,072 edges per renderer, eight renderer states per thread.

Verification passes: 14 helper results, nine recompiled legacy eligibility
results, three real native mutation results, 48 native scene tests, 27 shader
tests, three embedded-import results and the Windows system-DLL audit. Native
tests use actual DPR 1.75; no static UI layout is changed.

## Foreground diagnosis

The standalone native observer uses out-of-context foreground/desktop-switch
WinEvent hooks, not UI Automation or polling. Fixed-capacity records include
QPC time, event/current HWND and PID, executable path and callback cost. It
writes results only after capture and stops through its own named event.
A self-test observes both native test windows. A finite-duration watchdog and
owned-process cleanup are retained. This remains an intrusive diagnostic.

The first observed incremental-cache capture delivers only 1,480 wheel events,
all Qt-active, while the native foreground remains the Qt window throughout
both measurement windows. Startup activations of Git and the Android plugin
are recorded before the Qt window becomes foreground. They do not explain
that measured input-rate shortfall.

Four subsequent bounded-cache captures also show the same native foreground
PID/HWND throughout their measured windows, with zero foreground callbacks
inside those windows. The observer callbacks occur around startup/shutdown;
their maximum durations in these runs are 105.5–224.7 microseconds. The rejected
candidate B2 still delivers only 1,344 events, all active. Low input rate and
focus loss must therefore be investigated separately. Earlier rejected
focus-loss captures are not retrospectively reclassified.

## Cold comparison

Same diagnostic executable, dependency feature off/on; normal thumbnail
workers and unchanged baseline CPU/AV/resource options. Real Downloads,
191 entries, Masonry density 152, 3840×2160 / 160 Hz / actual DPR 1.75.
Fresh isolated application cache per process; OS cache not flushed. Two
ten-second wheel blocks. PresentMon GPU-duration tracking remains disabled
after its independently reproduced output failure; display tracking and all
original display/motion/coverage gates remain enabled.

Order: A1 control, B1 candidate, B2 candidate, A2 control. A1/B1/A2 pass capture
validity. B2 fails the unchanged 1,500-event minimum and remains rejected.
Active wheel counts: 1,531 / 1,518 / 1,344 / 1,541. No build, test, UI Automation
observer or offline analysis runs during gestures.

| Case | Block | Before-body median, ms | Preparation median/max, ms | Display max, ms | Gaps >9.375 ms | Motion max, physical px |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| A1 control | 0 | 0.3325 | 0.9715 / 7.9913 | 12.6413 | 6 | 89.212 |
| A1 control | 1 | 0.2942 | 0.9628 / 11.5795 | 18.8829 | 11 | 107.018 |
| B1 candidate | 0 | 0.1702 | 0.8319 / 8.3769 | 12.5102 | 6 | 85.861 |
| B1 candidate | 1 | 0.2021 | 0.9263 / 8.7954 | 18.2121 | 20 | 143.187 |
| B2 rejected | 0 | 0.1474 | 0.7254 / 6.2802 | 18.7736 | 6 | 163.578 |
| B2 rejected | 1 | 0.1801 | 0.7761 / 7.7812 | 18.7748 | 9 | 77.893 |
| A2 control | 0 | 0.3015 | 0.9320 / 12.3760 | 12.4980 | 5 | 77.834 |
| A2 control | 1 | 0.3490 | 1.1096 / 9.8532 | 28.3998 | 30 | 159.697 |

Coverage holds are zero in all blocks. Every block still fails the original
display and maximum-motion goals. This incomplete repeated matrix does not
establish a consistent tail improvement. The candidate's lower before-body
medians do not override its missed intervals.

B1 records 12 full graph resets, 27,015 individual path builds, 23,022 explicit
record retirements and 370 set trims. Peak graph storage: 2,639 ancestors,
10,621 edges and 176,128 set-capacity units, within the bound using measured
peaks. There are no overflows or capacity fallbacks. Totals include startup
and loading; retirements do not include records discarded by full resets.

## Reused GUI scheduler evidence

The previous current-renderer trace is reanalyzed for its GUI thread, using
its exact pinned executable/map and validated stack exports. This is the
unchanged GUI code but a different render-cache build/workload; the numerical
costs are not claimed to describe the new bounded candidate.

Across the two ten-second windows, GUI scheduling is 4,240.16/3,760.78 ms
scheduled, 51.18/46.63 ms runnable, 5,708.65/6,192.59 ms blocked, zero unknown.
Scheduled time includes interrupts/DPCs and is not pure application instruction
time. Of 7,924 GUI profile samples, four lack matching stack fragments.

The largest sampled self function is pointer-target discovery
`QQuickDeliveryAgentPrivate::eventTargets` (223 samples). Its joined callers
separate into 147 samples through modal-overlay filtering and 76 through
ordinary single-point event delivery. This identifies an additional target
discovery pass; it does not assign the entire GUI stall to hit testing.
Long GUI waits reach 19.2617 ms in `QWaitCondition::wait`, resumed through
`QSGThreadedRenderLoop::polishAndSync`, so render synchronization is another
direct route to delayed timers/input processing.

The upstream Qt 6.11.1 overlay source performs pointer-target discovery before
iterating stacked popups, and invokes that path for wheel/tablet events when
the overlay is visible. It is pinned as a reference only, not compiled into
this candidate. The installed project patches must be checked before any
replacement. [Qt overlay source](https://raw.githubusercontent.com/qt/qtdeclarative/v6.11.1/src/quicktemplates/qquickoverlay.cpp)

Next: reproduce the unnecessary overlay traversal with an empty-popup native
regression, preserve active modal/modeless popup behavior, and measure the
current GUI/input path. Continue the separate preparation/Map-wait work. The
full cold, verified-warm and quarter-height-middle acceptance matrix remains
required without lowering any gate.

## Storage recovery and evidence

D: filled before the regression object could be written. In-place compression
initially could not obtain working space. A complete lossless gzip backup of
the old `masonry-modes-stall.etl` was created on C: and its decompressed SHA256
verified before temporarily releasing the D: source space. The two inactive
crash dumps were compressed, the original trace was restored to its original
D: path, and all restored/compressed file hashes were checked. The original
evidence paths are present, the verified backup remains, and about 1.25 GB was
recovered. The rescue script and state record document each step.

Frozen source, failed/passing tests, foreground events, captures, GUI analysis
and storage proofs:
`profiler/experiments/masonry-cache-capacity-and-foreground-2026-10-29`.
Large binaries, old GUI stack exports and the rescue backup are externally
hashed. Only the exact diagnostic launcher tree was stopped. Normal worktree
PID 43936 was verified alive. No commit or push was made.

AI assistance has been used to create this output.
