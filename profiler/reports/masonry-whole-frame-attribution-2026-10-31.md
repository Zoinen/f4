# Masonry: synchronization and whole-frame CPU attribution

Constant 160 Hz is still unachieved. This analysis identifies a substantial
additional target outside rendering preparation: `ScrollJob::postSync` performs
repeated delegate enumeration, SVG descendant searches and transform mapping
inside scene synchronization, while the GUI thread is blocked. It accounts for
381 of 692 synchronization CPU samples in the ordinary-driver trace and
408 of 883 in the driver-threading experiment. These are statistical sample
shares, not exact elapsed-time percentages or proof that it causes every spike.

The next experiment should reuse scene membership and geometry records across
unchanged synchronization passes, with explicit invalidation. No optimization,
portable payload rollout, commit or push was performed in this step.

## Evidence and boundaries

This is offline analysis of two existing cold Downloads captures: 192 entries,
Masonry density 152, actual DPR 1.75 and 160 Hz. They already contain both GUI
and render-thread scheduler events and sampled stacks. No fresh live capture,
UI Automation activity or observer was introduced. The second trace enables
the diagnostic D3D11 threading flag; it is a separate configuration, not an
identical causal control. Neither trace has PresentMon displayed-frame data.

| Capture | Frames | Synchronization p50 / p99 / max (ms) | Preparation p99 / max (ms) | Render recording max (ms) |
| --- | ---: | --- | --- | ---: |
| coldresidual | 3201 | 0.4272 / 1.3912 / 2.9032 | 2.2766 / 6.8655 | 2.0557 |
| driverwait | 3192 | 0.4770 / 2.4544 / 4.5986 | 3.7344 / 8.1291 | 4.3316 |

Only 1603 / 1611 frames respectively include a new GUI synchronization pass.
Frames without one are retained in whole-frame analysis; optional sync markers
are not invented. GUI scheduled time inside synchronization is zero in both
captures. Render-thread scheduled time in the worst synchronization spans is
2.9032 / 4.5959 ms, respectively: this is CPU work rather than a long blocked
render-thread wait.

The largest render-thread frame-end interval is 12.5148 / 17.3993 ms. Those are
**not displayed intervals** and do not establish a display or motion gate result.
Both intervals are retained without relabeling frame completion as scanout.

The ordinary-driver worst interval contains 0.8419 ms sync, 6.8655 ms preparation,
2.0557 ms recording and 0.8100 ms submission. Its preparation contains the
previously identified NVIDIA Map wait. Another 11.2769 ms interval contains
2.0543 ms sync and 2.2929 ms preparation. There is no single preparation-only
explanation for all long frames.

## What synchronization is doing

Synchronization profiles (inclusive function counts require a joined stack):

| Call path, inclusive | coldresidual | driverwait |
| --- | ---: | ---: |
| `ScrollJob::postSync` | 381 / 692 | 408 / 883 |
| `QQuickWindowPrivate::syncSceneGraph` | 680 | 867 |
| `visitVectorGates` under `postSync` | 140 | 148 |
| `QQuickItem::mapRectToItem` | 102 | 94 |
| `QQuickWindowPrivate::updateDirtyNodes` | 294 | 451 |

Inclusive rows can overlap and must not be added together. Some samples within
the signal-marker interval are outside `syncSceneGraph`; a missing function in
a stack is not classified as an unjoined stack.

The current [scroll implementation](../../third_party/ZoinGallery/GalleryScrollAnimation.cpp)
rebuilds roots, vector gates and clip lists at every `postSync` call. It scans
surface branches and children, uses QObject casts, builds a prepared-index set,
maps delegate rectangles through the item hierarchy, recursively enumerates
vector descendants, and maps their rectangles again. This is work on unchanged
scene membership as well as on actual mutations. Geometry correctness and
coverage readiness still require refreshes when relevant state changes.

The most expensive ordinary-driver sync span has three samples and none is
inside `postSync`. The next two have one each. Therefore the aggregate result
supports targeting a substantial recurring cost, **not** claiming that caching
this function will eliminate the worst spike. Dirty-node processing and scene
churn remain separate contributors.

GUI profiles also show Masonry delegate commits, QML bindings, pixel-grid work
and mouse hit-testing. Scheduled GUI and render time overlap on separate cores;
their durations cannot be added to produce frame latency. This analysis does not
establish mouse hit-testing as the dominant critical-path delay.

## Pacing and data quality

The saved Qt render-loop source places `beforeFrameBegin` before
`QRhi::beginFrame`, followed by synchronization. The roughly 5 ms median
before-sync span includes normal swapchain pacing. GUI inactivity there is not
evidence of a 5 ms application CPU operation. Exact driver wait attribution still
requires the corresponding stacks and scheduler state; the analyzer does not
subtract all waiting from an asserted display budget.

Both traces report zero lost events, lost buffers, decode errors and processing
errors. The sampled-profile prefix fallback was independently checked against
xperf: 431 / 402 reference records match. Raw packet hashes and code-identical
symbol-map receipts are validated before analysis. ICF aliases stay explicit.

Frame-window stack joins are 5077 render / 5455 GUI in coldresidual, with three
unjoined samples each; driverwait has 7340 render / 6279 GUI, with two / seven
unjoined. These totals cover whole frame spans, not just synchronization.
The notify-dispatch probe records **zero entries**, so no dispatch latency claim
is made from that array. This does not prove dispatches were cheap or absent.

All 6393 frames have scheduler-state conservation and no unknown state in the
analyzed spans. A second output verifier checks 90 distinct selected frames:
ordered markers, exact span duration, state sums, whole-frame partitioning and
sample/stack membership. It passes. The compact evidence is in
[the frozen experiment](../experiments/masonry-whole-frame-attribution-2026-10-31/manifest.json).

## Next implementation and verification

1. Measure `postSync` membership discovery, mapping and coverage steps separately
   with a bounded native probe. Keep the probe cost visible.
2. Replace repeated vector-descendant discovery with a bounded registry or cached
   ordered membership. Refresh for delegate rebind/reparent/destruction, Loader
   replacement, source revision and scene-graph invalidation. Do not keep stale
   QSGNode pointers across invalidation.
3. Reuse geometry only when its coordinate-space and ancestor-transform revisions
   match. Preserve native camera motion, vector readiness, clipped retained scenes,
   coverage holds, exact resting pixel alignment and scroll-stop handoff.
4. Add mutation regressions against direct traversal, verify 175% rendered output,
   then run the original cold, verified-warm and quarter-height middle-scroll
   acceptance workload. Keep the existing display/motion thresholds.

> AI assistance has been used to create this output.
