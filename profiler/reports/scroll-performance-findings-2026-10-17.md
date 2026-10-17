# Scrolling performance findings before the pixel-grid migration

This is a checkpoint of the investigation and existing implementation, before
the planned migration to cached physical-pixel layout. The migration is not
implemented here. Some optimizations and correctness fixes are complete; rapid
wheel scrolling and first-pass SVG rendering still have unresolved costs.

## Measurement context and limits

Recent live captures use `C:\Users\xs\Downloads`, 183 entries, a 160 Hz display,
and actual window DPR 1.75. The frame budget is 6.25 ms. Earlier reports have
different densities, neighboring panels, and sometimes 182 entries; their
numbers are historical observations, not controlled comparisons with every
later run. First traversal means a fresh presentation with the normal disk
cache retained, not an empty source/thumbnail cache.

PresentMon display intervals describe displayed cadence. Native frame-swap
timestamps describe submission pacing. QML traces and ETW sampling are separate
attribution passes: instrumentation, setup, PNG captures, and CPU analysis can
distort timings. Inclusive QML totals overlap and must not be added together.

Raw traces and diagnostic executables remain local build artifacts. The
supporting Markdown reports below preserve measurements and trace identifiers.

## 1. Alignment does more work than the arithmetic requires

Source inspection of `qt/host/src/ScenePixelAlignment.cpp` confirms a per-window
`afterAnimating` callback which, on every frame:

- allocates a temporary list for all registered alignment objects;
- walks their ancestors to calculate depth;
- stable-sorts the list;
- maps every registered item through the scene and calculates its correction.

Unchanged chrome and dialogs participate while unrelated content animates.
Gallery delegate bindings also construct ancestor dependencies and map visual
leaves; `MasonryLayout::positionViewport()` recalculates ancestor corrections
when the scroll offset changes. Document run corrections incorporate scrolling
into individual text-leaf bindings.

These are verified source-level repeated-work patterns. Their individual CPU
costs have not all been isolated in the latest rapid-wheel capture. It would be
incorrect to attribute the full delay to `round()` itself. Traversal, binding
invalidation, notifications, text rebuilding, and scene changes are the larger
mechanisms to remove or measure.

At DPR 1.75, an integer logical coordinate is not sufficient for physical
alignment. Multiples of four map to integer physical coordinates, but centering
can reintroduce halves: a 16-unit child centered in a 28-unit parent starts at
6 logical units, or 10.5 physical pixels. Cached physical edges and explicit
centering are needed; merely restricting source dimensions is insufficient.

## 2. Rapid wheel input repeatedly ends and restarts motion

`GalleryViewportController::animatePanelScrollTo()` restarts the 150 ms
`NumberAnimation` for every wheel event. Restarting publishes a stopped and a
running state. Those states drive viewport persistence/finalization, alignment
suspension, and `GalleryMotionText` renderer changes for native text.

The diagnostic host delivered real `QWheelEvent`s through the window, using
120-unit vertical deltas at a requested 30 ms interval, for 900 ms in each
direction. These were not continuous synthetic animation sweeps.

| Capture | Delivered events | Motion-state changes | Handler median | Handler p95 | Handler maximum |
| --- | ---: | ---: | ---: | ---: | ---: |
| `wheel-before-fast-masonry` | 54 | 108 | 15.143 ms | 18.530 ms | 24.694 ms |
| `wheel-before-warm-masonry` | 60 | 120 | 17.265 ms | 20.594 ms | 27.388 ms |

Both captures used DPR 1.75 and 160 Hz, with zero caption measurements. The
handler measurements include synchronous event dispatch and its consequences;
they are not isolated timings of rounding, text rendering, or one QML function.
The repeated transitions are directly observed; the exact division of their
cost between downstream consumers remains an attribution task.

The wheel fix was interrupted in favor of planning the new grid architecture.
The checkpoint retains the unfinished
`wheelBurstKeepsMotionActiveUntilSettled` regression. It must not be described as
a completed or passing wheel fix.

## 3. Native text has its own rounding

The exact Qt 6.11.1 source, in
`qtdeclarative/src/quick/scenegraph/shaders_ng/textmask.vert`, applies
`floor(xformed.xyz * dpr + 0.5) / dpr` after the model-view transformation.
`qsgdefaultglyphnode_p.cpp` also aligns native glyph geometry while generating
it. Therefore, fractional parent translation does not make native text move at
fractional physical-pixel positions.

The current `GalleryMotionText` temporarily uses Qt text rendering during motion
to avoid this quantization, restoring the original renderer at rest. Repeating
that transition on every wheel notch is avoidable work. The upcoming migration
will instead keep the user's selected renderer unchanged: native text may move
in whole-pixel steps, while images and the common viewport move fractionally.
This is an agreed future behavior, not implemented in this checkpoint.

## 4. Caption measurement, delegate churn, and SVG scenes are different costs

The caption cache is effective: recent warm Grid and Masonry captures record
zero caption measurements while scrolling. Caption measurement is not supported
as an explanation for the latest rapid-wheel delay.

Earlier traces did identify synchronous SVG scene construction and entering-row
object churn. A single VectorImage creation reached 12.5–20.1 ms across modes;
a Grid frame included 54.46 ms constructing vector scenes. Caching an SVG source
does not cache its generated scene geometry.

Existing changes retain compatible previews and bounded delegate working sets,
and reuse a bounded cache of direct vector scenes by immutable source revision.
They preserve source leases, discard stale completions, clear outgoing pixels,
and retain vector rendering. Warm Grid scene-reuse captures contain zero
VectorImage creation events. These changes reduce churn but do not alone prove
160 Hz rendering; the Columns reuse report explicitly records a worse p95
despite a smaller worst pause.

Masonry now separates raster/ordinary and vector drawing branches. Stable
native drawing gates avoid unnecessary opacity and clip-node changes. Rejected
per-row clip changes reduced some preparation intervals but increased ordinary
rendering costs; they are not part of the product change.

## 5. Qt drawing-order accounting caused expensive rebuilds

The exact Qt 6.11.1 batch renderer reserved geometry drawing orders in every
ancestor root on insertion but returned them only to the immediate root on
removal. Repeated subtree changes exhausted ancestor reservations and forced
expensive complete rebuilds.

The retained dependency patch returns orders through the matching ancestor
chain. The nested-branch regression failed with the original library on its
first cycle: ancestor available orders fell from 1024 to 1004. The patched
library preserves the initial count and identical rendered pixels over eight
cycles. The CI patch and recipe-cache changes preserve this fix reproducibly;
experimental symbol overrides and generated static libraries are not committed.

Partial batch rebuilds must not be called complete rebuilds. Diagnostic flags
observed in the investigation distinguish tagged-root, complete-list, and batch
rebuilding. Attribution details and superseded experiments remain in the
Masonry investigation report.

## 6. Backward jumps were stale viewport restoration

The original restoration guard deferred reconciliation during the wheel's
NumberAnimation but did not protect middle-button scrolling. Late metadata or
content-height changes could restore an old saved offset during an active
gesture. Captures included downward movement followed by resets such as
1270.38 to 0.

The retained fix defers reconciliation for middle gestures too, clears queued
initial restoration, and persists the final offset before deferred work runs.
A quarter-screen pointer displacement regression failed before the fix with
Masonry 572.533 to 0. Both queued and late-update variants now pass. Checks
which counted only stationary frames had missed these reversals; monotonicity
is a separate required assertion.

The earlier low-speed middle-scroll fix also allows fractional viewport
translation while animating and restores physical-pixel alignment at rest.
Nearest-neighbor icon sampling and native glyph snapping had otherwise made
small movements visibly quantized.

## 7. First-pass SVG rendering still exceeds the budget

The final quarter-screen middle-scroll first-pass capture has no reversals,
but its native frame gaps reach 27.509 ms and render CPU reaches 14.600 ms.
Down/up display-interval p99 is 12.501/6.303 ms, with 14/1 intervals above
9.375 ms.

The corresponding warm capture has no reversals, median display intervals of
6.25 ms, p99 6.290/6.300 ms, and no intervals above 9.375 ms. This proves good
cadence for that warm workload, not uninterrupted 160 Hz in every case.

ETW samples from slow first-pass bursts include
`QSGBatchRenderer::Renderer::uploadBatch`, `Renderer::unmap`,
`QD3D11Buffer::create`, and buffer copies. Native dirty-node tracing associates
cold full rebuilds with SVG scene changes. This supports geometry allocation
and upload as remaining costs; it does not establish that a single allocation
policy alone causes the slowdown.

Early SVG branch/clip placement, a 16 MiB reuse pool, geometric buffer growth,
and initially dynamic buffers did not materially improve cold display cadence.
Those experiments are excluded from the product.

## 8. The observer affected measurements

The computer-use observer contributed Windows UI Automation work on the GUI
thread. Disconnecting that observer removed the measurement overhead without
disabling application accessibility. Earlier observed runs must be separated
from later clean captures. Builds, tests, verbose tracing, and capture setup
also must not overlap headline performance measurements.

## Checkpoint and verification status

The checkpoint includes existing gallery caching, scene/batch, alignment,
middle-scroll fixes, dependency patching, host worktree-title correction, and
their tests. It intentionally precedes the new dirty-scope/physical-layout
migration. Both repositories use branch
`zoin_branch/scroll-performance-checkpoint-20261017`; the parent commit pins the
gallery commit.

The gallery checkpoint is `2dcba89` (`chore(gallery): checkpoint scrolling
performance work`). It includes the six new C++/QML files referenced by the
existing build changes, not only modifications to already tracked files.

Historical verification is scoped to the changes and builds in each report:

- The latest middle-scroll targeted runs report native modes 25 passed,
  software modes 24 passed, renderer/gesture lifecycle 6 passed, and isolated
  imports/production-host startup 4 passed, including fixture setup/cleanup.
- The earlier full native Modes run reports 148 passed, three failed, one
  skipped. Remaining failures are two Icons reorder-anchor cases and the
  Details 49-row reset budget (44.339 ms against 16 ms). They are not fixed here.
- The newly added wheel-burst regression was linked and run for this checkpoint
  on the native renderer at DPR 1.75. All five mode variants fail: eight notches
  cause 15 motion-state changes and seven offset commits instead of maintaining
  one active gesture. QtTest reports 2 fixture passes and 5 failures. The local
  result is `artifacts/checkpoint-wheel-regression-175.txt`. No product fix was
  applied and the full suite is not claimed green.
- The Qt dependency patch tests were rerun for this checkpoint: 12 passed.
  Whitespace/error checks passed in both repositories.
- Earlier portable DLL-import audits and embedded payload/extraction checks
  passed as documented. This checkpoint itself changes documentation and Git
  history, not the application implementation or its embedded payload.

## Supporting reports

- [All five modes: initial 160 Hz investigation](downloads-scroll-160hz-2026-10-15.md)
- [Preview/delegate reuse and its tradeoffs](preview-reuse-2026-10-16.md)
- [SVG scene retention](svg-scene-cache-2026-10-16.md)
- [Masonry render batches, Qt patch, measurements, and test limits](masonry-investigation-2026-10-17.md)
- [Low-speed middle scrolling](middle-autoscroll-2026-10-17.md)
- [Quarter-screen reversal fix and remaining first-pass stalls](middle-autoscroll-first-pass-2026-10-17.md)

AI assistance has been used to create this output.
