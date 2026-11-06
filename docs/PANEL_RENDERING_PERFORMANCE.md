[← Pixel grid](PIXEL_GRID.md) · [Back to README](../README.md) · [File fields →](FILE_FIELDS.md)

# Panel rendering: findings and maintenance guide

This records the useful results of the October–November 2026 Qt/ZoinGallery
investigation. It is a guide to preserving the improvements, not a claim that
all workloads now sustain 160 Hz. Historical reports describe the code and
instrumentation used in their own captures; a diagnostic candidate appearing
in a report does not mean that it became a production default.

The [checkpoint validation record](../profiler/reports/production-checkpoint-validation-2026-11-06.md)
lists passing checks, unresolved integration failures and incomplete suites.

At 160 Hz the display period is **6.25 ms**; at 60 Hz it is 16.67 ms.
Smoothness, responsiveness, and image readiness need separate measurements.
A regular stream of frames can show the same camera position while tile
coverage holds it back. A low average CPU cost can hide a long first-entry stall.

## Where the time went

| Layer | Finding | Practical consequence |
| --- | --- | --- |
| Alignment | The old frame callback enumerated leaves, walked ancestors, sorted by depth, and mapped scene coordinates even for unchanged UI. | Cache physical layout and ancestor relationships; invalidate changed branches. Do not optimize this as merely a slow `round()` call. |
| Input/motion | Wheel restarts repeatedly published stopped/running transitions and changed text/alignment state. Warm baseline handler median was 17.265 ms, p95 20.594 ms. | Preserve one gesture until settlement; do not persist/rebuild on intermediate restart notifications. |
| Captions | The measurement cache produced zero warm-scroll caption measurements in captured Grid/Masonry runs. | Keep it, but investigate other work when its miss counter is zero. |
| QML/scene construction | Entering delegates and generating SVG scene geometry could consume an entire frame or more; SVG creation reached 12.5–20.1 ms in historical traces. | Reuse bounded compatible visual objects and vector scenes; caching SVG bytes alone is insufficient. |
| Publication | An activation/title-only shell publication synchronously propagated for 12.835–13.149 ms without changing panel descriptors. | Use typed, narrow notification lanes; retain backing state without reprojecting the complete shell. |
| Qt renderer | Batch invalidation, temporary ordering containers, geometry uploads, and buffer allocation were separate costs. | Preserve ordering proofs and bounded storage reuse; measure allocation, upload and driver waits independently. |
| Tile coverage | Home/End repeatedly waited for intermediate prepared windows and restarted the remaining animation curve. | Request destination images at input time; keep a destination deadline independent of intermediate delegate readiness. |
| Cache/decode | More RAM did not remove delegate construction, Qt synchronization or GPU residency costs. A disk-cache hit still requires lookup/read/decompression/publication/upload. | Report which cache hit: source bytes, derived thumbnail, decoded image, vector scene, or GPU resource. |

Sources: [alignment/wheel checkpoint](../profiler/reports/scroll-performance-findings-2026-10-17.md),
[shell publication](../profiler/reports/masonry-shell-publication-cost-2026-11-03.md),
[renderer ordering storage](../profiler/reports/masonry-order-scratch-production-2026-10-31.md),
[warm Home/End](../profiler/reports/home-end-warm-cache-2026-11-04.md).
These measurements belong to different captures and must not be added together.

## Keep these production boundaries

### Layout, scrolling and pixel fitting

Layout calculation belongs to catalog, metadata, sizing and geometry changes.
Scrolling changes the common viewport/camera translation and bounded visibility
or priority queries. A first-viewport shortcut may defer remaining catalog work,
but scrolling must not turn that shortcut into repeated whole-layout work.
After deferred metadata arrives, retain the resulting full geometry and real
image proportions. Delegate materialization remains viewport bounded.

Use the shared cached physical-pixel service described in [PIXEL_GRID.md](PIXEL_GRID.md).
At rest, validate real text/image leaves through their complete ancestor
transforms at the actual DPR. Logical integer sizes are insufficient at 175%:
centering two whole-physical-pixel extents can still produce a half-pixel offset.
During animation, move the common scope rather than refitting every leaf.
Keep the configured text renderer; changing it per wheel event rebuilds work.

Native eligible Masonry motion uses a retained render-thread camera. Preserve
its hit-testing, clipping, scrollbar mapping, rewrap anchor and write-back
contracts. Software/other-mode fallbacks remain required. Hidden/minimized
windows stop and commit motion rather than accumulating hidden elapsed time.

### Stable objects and targeted updates

Reuse compatible delegates across folder entry/exit and viewport changes.
Reset source identity, selection, readiness and outgoing pixels when reusing
objects. Retention without those resets produces ghost tiles, stale previews
or incorrect hit targets. Keep visual overscan separate from metadata/decode
look-ahead; enlarging the latter need not instantiate more QML objects.

Preserve persistent model identity for compatible sparse catalog extensions.
A changed directory-preview descriptor is not automatically a new folder or a
reason to cancel all row preparation. Genuine source/revision changes must still
invalidate old work. See the [catalog-reset regression](../profiler/reports/masonry-directory-preview-catalog-resets-2026-11-03.md).

An unchanged notification is still potentially expensive: bindings, model
updates and synchronous signal delivery run downstream. Compare the semantic
change, not only whether a setter was called. Compact activation/title updates
must leave the registry's latest state correct for later-created consumers.

### Bounded rendering and image work

Keep resource reuse bounded and preserve Qt painter order. Stable interval,
batch-membership and removed-element checks are correctness preconditions,
not optional overhead. The retained ordering scratch avoids repeated temporary
container allocation and falls back to ordinary containers beyond its bounds.
Retired vertex/index buffer budgets are not total live GPU-memory budgets.

Vector surfaces remain vectors. Bound scene construction and upload work;
publish only complete, current-generation surfaces. Upload admission can delay
cold previews a few refresh intervals and does not bound every driver call or
the complete frame. Do not trade partial SVG rendering for a better timing number.

Thumbnail memory limits, persistent thumbnail resolution and viewer decoded
frames are different policies. The persistent cache keeps the larger useful
thumbnail, capped at 1024 px on its long side; resizing must not accumulate an
entry for every requested size. Retain true aspect metadata independently of
whether a particular thumbnail decode succeeded. See [FILE_FIELDS.md](FILE_FIELDS.md).

Active viewer and current viewport demand must supersede stale speculative
work. Neighbor decoding remains bounded and parallel. Do not solve queue
backlog by discarding keyboard auto-repeat or silently reducing the workload.

## Approaches that did not establish a general win

- Removing rounding alone does not remove ancestor traversal or QML invalidation.
- More cache RAM, more overscan, more concurrent construction, prewarming, or a
  larger request window can shift work into a critical frame or leave requests
  queued behind other work. Measure admission and readiness, not queue size alone.
- A direct caption delegate experiment did not convincingly improve coverage
  and stayed diagnostic. [Report](../profiler/reports/masonry-direct-caption-experiment-2026-11-03.md).
- Suppressing the other panel's overscan did not establish a benefit; an initial
  experiment even throttled the scrolling panel because shell activation was
  mistaken for motion ownership. [Report](../profiler/reports/masonry-inactive-overscan-experiment-2026-11-03.md).
- Backend, frame-latency, shader, buffer and driver experiments are controls,
  not universal settings recommendations. Moving a wait is not proof of removing it.
- Reducing a worst pause while worsening p95, or delivering fewer input events,
  is not sufficient evidence of a scrolling improvement.

## Evidence that prevents wrong diagnoses

The warm Home/End capture had 6.25 ms median present intervals while a stable
End still took 1,225–1,239 ms against a 150 ms animation budget. In two passes,
destination thumbnails were already in RAM but were requested only after
1,047–1,067 ms. There were 122/125 coverage constraints. This established a
motion/preparation coupling, not an SSD-bandwidth explanation. The report is a
pre-fix diagnosis; it does not certify the later deadline implementation.

An early-entry scheduler capture found a 62.524 ms display gap with the GUI
running for 62.132 ms and render waiting for 60.763 ms. GUI runnable-but-not-
scheduled time was only 0.014 ms. That capture points to GUI execution, not a
blanket claim that Windows scheduling or the GPU caused the gap. It cannot name
the responsible function. [Scheduler evidence](../profiler/reports/masonry-early-scheduler-2026-11-03.md).

In one readiness capture both construction slots were occupied for every one
of 154 distinct queued-blocker snapshots. Both slots were constructing needed
rows in 117 snapshots. Other snapshots included ready roots awaiting delivery.
Construction age includes suspended incubation and is not CPU time; improving
ready-root delivery alone cannot explain every hold. [Occupancy evidence](../profiler/reports/masonry-o176-coverage-root-occupancy-2026-11-03.md).

## How to validate the next change

1. Record the exact executable/Qt payload, branch, folder fingerprint and count,
   mode/density, window dimensions, actual DPR, physical refresh, foreground,
   graphics backend, power/session state and cache layer being warmed.
   On mixed-DPI desktops, verify the actual target window's screen and DPR;
   neither the primary screen nor the application-wide DPR proves its scale.
2. Keep cold entry, fully warm scrolling and source/decode attribution separate.
   Use wheel bursts, middle-button motion, held Page Down, Home/End, repeated
   folder entry/exit, resize, and hide/restore as distinct scenarios.
3. Preserve the requested input schedule. Record delivered/skipped events and
   pending work after input stops. Do not restart the observation window after
   the stall or omit coverage holds from acceptance.
4. Measure input handling, publication, polish/incubation, sync, renderer
   preparation, graphics upload, submission and displayed intervals separately.
   Measure requested-versus-visible camera displacement and destination time too.
5. Report p50/p95/p99/max, sample counts and frame-budget overruns. Inclusive
   QML spans overlap. Samples are not exact CPU milliseconds; wall-clock spans
   can include waits. Correlate with scheduler evidence before assigning blame.
6. Use the same workload for controls. Changed folder counts, focus loss, trace
   overflow or lost events remain explicit limitations. A 60 Hz/DPR 1.0 remote
   session is not representative evidence for 160 Hz/DPR 1.75.
7. Keep instrumentation out of clean cadence captures. UI Automation, animated
   observer overlays, desktop duplication, stackwalking and verbose file output
   can perturb results. Use the guards in [profiler/README.md](../profiler/README.md)
   and record observer controls separately; never terminate another client's helper.
8. Add a regression for the demonstrated fault. Verify real leaf transforms and
   rendered text/icons at 175%, plus painter order, clipping, stale-generation
   handling, cache bounds and hidden-window lifecycle where relevant.
9. Rebuild the embedded host and Go launcher under the portable policy whenever
   rendering code or Qt patches change. Verify the extracted child hash. A new
   launcher around an old payload is not a test of the change.

Avoid another open-ended optimization cycle: identify a measured mechanism,
make a bounded change, rerun its regression and representative flow, and record
what improved and what remains uncertain. No report in this collection proves
that the entire cold/warm application workload is consistently 160 Hz.

## See also

- [Physical-pixel layout and motion contracts](PIXEL_GRID.md)
- [Typed media fields, proportions and cache behavior](FILE_FIELDS.md)
- [Portable build and verification policy](PORTABLE_BUILD_POLICY.md)
