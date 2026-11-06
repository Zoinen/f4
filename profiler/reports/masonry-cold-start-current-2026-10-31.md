# Current-graph cold-start scrolling after observer isolation

## Outcome

Cold startup still has visible stalls. Two fresh-cache controls recorded one
and eleven raw long display intervals respectively. A short CPU/scheduler trace
shows substantial GUI work across early presentation gaps and a separate blocked
render-thread span. No production optimization is promoted by this study.
Constant 160 Hz remains unachieved.

The preceding turn removed an animated Computer Use observer and obtained clean
warm wheel/middle results. This turn keeps that observer absent and establishes
a reproducible cold-cache path, rather than extrapolating the warm result.

## Exact scope

Each case launches the current graph with a new empty gallery cache, configured
through the existing startup-only StorageLocations API before QML construction.
The host rejects an absent, nonabsolute, already-used or nonempty cache root.
The command is queued before launch; input starts once the Downloads catalog is
visible and active, with the viewport at zero. Hashing, script preparation and
collector startup occur before the application launches. The host requests its
own focus before timing; Computer Use is not reconnected.

All input observations retain Downloads' 192 entries, Masonry density 152,
actual DPR 1.75, 160 Hz and foreground focus. No full-folder tour precedes a cold
capture. Source decoding continues while scrolling: the initial cache is
partially populated, rather than empty at the instant of input. The filesystem's
OS file cache is not flushed.

These are **early application-startup plus first-scroll stress tests**. Initial
viewport previews can still be loading. They do not replace a separate test of
entering Downloads in an already-initialized application. Keep these startup
failures and add that folder-entry check; do not replace them with an easier
warm measurement.

The native animation/render policies are unchanged. The diagnostic host adds
startup cache isolation, an input fixture and bounded read-only timing/motion
probes. Host SHA-256:
`95218f7c4045bcd08cdf9ecb7f86506118d8b33cde8074aae7fcd303ae1fe267`.
The existing import-isolation QtTest target passes its three checks; the host's PE
subsystem and system-only DLL audit pass. It is not embedded or distributed as
a new portable production payload. The ordinary running app remains unchanged.

## Fresh-cache controls and attribution run

| Case | Cache frames before → after | Pending before → after | Render p99 / max ms | Raw display max ms | Raw long intervals / drops |
| --- | ---: | ---: | ---: | ---: | ---: |
| cold-a1 | 29 → 117 | 15 → 0 | 2.938505 / 6.715 | 12.4655 | 1 / 0 |
| cold-a2 | 3 → 120 | 24 → 0 | 2.913736 / 10.683 | 56.2645 | 11 / 1 |
| cold-p2, early tracing | 2 → 118 | 25 → 0 | 3.050200 / 16.6522 | 56.2376 | 7 / 0 |

Long means >9.375 ms. All cases have zero coverage holds. Maximum camera-matrix
float error stays below 0.000241 physical pixels. These are not wheel-curve or
optical-motion acceptance results. Raw interval failures and every dropped row
are retained, including first-interval boundaries.

An original admission check incorrectly required the final frame count to equal
or exceed a prior warm tour's 119. The controls themselves show counts of
117/120/118, and cold startup does not require a completed warm tour. The corrected
check requires an empty root before QML, no preceding tour, correct input scope,
and observed increases in frames, stores and misses during scrolling. The original
cold-a1 rejection remains in its own receipt. Ten admission regressions pass,
including incomplete-tour source work, cache reuse, wrong root/viewport, focus
loss and wrong DPR. No frame-time, motion, coverage or presentation limit changed.

## Boundary accounting

The 56.2376-ms interval in p2 crosses the measurement start. The preceding present
is at -45.147 ms and the next at +7.222 ms. It must not be called 56 ms of CPU work
caused by a scroll event. The full raw interval remains visible in the result;
this is a startup boundary, not a discarded outlier.

There are also definite post-start gaps. Across the 20.6953-ms interval beginning
at +7.222 ms, the GUI thread is scheduled for 20.140 ms and the render thread for
2.980 ms. Across the next 13.5354-ms interval, GUI scheduled time is 12.841 ms.
Scheduled time includes interrupt occupation; it is not exclusive instruction
time. The samples provide the application-work evidence below.

## Early CPU/scheduler attribution

The startup-only kernel capture begins immediately before launching the core
and stops after five seconds plus trace rundown. It does **not** cover all
30 seconds of input. The remaining portion is retained but cannot receive CPU
attribution from this trace. Sampling and collector shutdown are intrusive;
p2 is not a clean comparison arm.

The trace has 1,185,021 events, zero lost events/buffers, and successful readers.
Its profile-prefix fallback is independently checked against xperf: all 421
selected thread/IP/timestamp records match. A separate symbol-only link has
identical .text and .pdata bytes/RVAs to the captured host. The symbol-only
executable is never run. ICF aliases, unknown symbols and absent/ambiguous stack
joins remain explicit; samples are not exact function durations.

GUI samples across the early gaps include:

- ExternalCatalogRowsTransaction validation/publication and child directory
  preview catalog completion.
- Masonry visual snapshot updates and thumbnail request planning.
- QML object/incubator setup and binding notifications.
- Variant-association element wrappers and incremental GC marking/sweeping.

This points to cold GUI catalog/property churn as a candidate for measurement
and coalescing. It does not prove that every listed path is redundant or that
one alone accounts for the full interval. Directory preview selection is already
bounded to 16 donors; do not "optimize" it by assuming an unbounded child catalog
or by dropping existing folder previews.

A separate frame at +1284.527 ms has a 16.6522-ms render-signal range. The render
thread is scheduled for only 3.5758 ms inside that range and blocked for 13.0764
ms. The long wait has recorded state 5/reason 28 and only 0.0068 ms of ready-to-run
delay. This trace does not contain context-switch caller stacks, so the exact
blocking call is unproven. Treating the whole range as expensive renderer CPU
would send optimization in the wrong direction.

## Preserved setup failure and storage changes

`cold-p1` stops at the disk-space guard before launching a Qt instance. Its
collector output and failure are retained. Cleanup initially tried to open a
trace that had never started; that harness error is corrected and covered by a
regression. A second regression verifies that cleanup cannot stop a foreign
kernel session.

Closed historical traces were compressed with streaming before/after SHA-256
checks. Several completed build inputs were copied to an owned archive, verified,
and their original paths preserved as symlinks. This includes the patched Qt
archives and the ZoinGallery build archive, not the installed SDK. Their logical
bytes are unchanged. No user file or raw trace is deleted, and no external app,
VM, affinity or Windows graphics setting is changed.

## Next work and remaining acceptance

Quantify repeated cold catalog/snapshot publications before changing batching or
introducing another cache. Preserve thumbnail/viewer priority, cancellations,
fresh metadata, native SVGs and folder previews. Obtain an exact blocked-render
caller if that wait is targeted; do not infer it from a neighboring sample.

The broader goal still requires cold folder-entry validation in an initialized
app, complete wheel curve/physical-motion checks, rendered-pixel verification,
and sustained normal-window no-spike delivery. The previous warm controls remain
useful but do not establish these requirements. No commit, push, production
rollout or goal-completion claim is made.
