# Text batch fragmentation during Masonry scrolling

The 160 Hz target remains unmet. The frame budget is 6.25 ms. This investigation
uses the fresh QML binaries rebuilt after discovering stale diagnostic logging;
it does not reuse the earlier logging-contaminated attribution as clean evidence.

## Workload and method

Windows, Qt 6.11.1, Direct3D 11, 160 Hz, DPR 1.75, Downloads with 186 entries,
left Masonry at density 152, right Details. Native material timing uses bounded
buffers. A separate owner census walks the item tree only after measured input
has finished, while the GUI thread is blocked for synchronization. Census runs
are diagnostic and are not clean cadence comparisons.

Artifacts are under `artifacts/modes-profile/`, with prefixes `fresh-qml-wide`,
`fresh-materials`, `batch-bounds`, and `bounded-details-160`.

## Findings

- The material timing capture records approximately 162 merged text batches,
  84–85 texture batches, and 20 thumbnail shader batches per active frame.
  Timings are inclusive; nested scopes must not be summed as independent work.
- A post-scroll census finds 162 text batches but only 11 compatible text
  material groups. Text materials have blending enabled, without NoBatching or
  full-matrix flags. Material uniqueness does not explain the fragmentation.
- The right Details panel accounts for 86 text batches. Each preview is clipped
  independently, giving its graphics a distinct batch root. Qt's alpha batching
  stops at the intervening root, even though successive row labels have the
  same material and do not overlap.
- Masonry has a different barrier: each caption background occurs immediately
  before its text. The next background overlaps its own caption, preventing that
  caption from being merged into a preceding text batch. This remains unfixed.
- Disabling non-vector preview clips diagnostically changes the right Details
  panel from 86 text batches to 5. The initial diagnostic runs have differing
  scroll extents and warmup state; their frame timings are not a valid A/B result.
- Fresh long attribution also records atomic GC sweep phases around 6–7 ms and
  mark-drain work up to 6.83 ms. Reducing text batches does not solve those stalls.

## Narrow implementation

Details raster thumbnails and fallback icons already fit their preview slots.
Remove only their redundant outer clip. Keep the viewport clip and retain SVG
preview clips, since SVG geometry may exceed its viewBox. Other compact modes
retain their existing behavior; Masonry keeps its existing shader masking.

Regression `detailsBoundedPreviewsDoNotSplitTextBatches` failed on the original
per-preview clip before the fix. It checks loaded raster thumbnails and colored
fallback icons, every visible named text/image leaf's scene origin and unit
transform at DPR 1.75, and exact rendered equivalence with the old clips restored.
It uses Segoe UI for primary and secondary text. Direct3D/OpenGL results and
captures are recorded under `artifacts/bounded-details-{backend}-175/`.

## Verification and clean cadence comparison

The narrowed production change gives the same census result: Details text batches
86 to 5, and total text batches 162 to 81. The matched diagnostic snapshot has
46 raster-branch Masonry caption text batches and 9 vector-branch caption batches
remaining. No global layer cache, new rounding traversal, or Qt threshold change
was added.

Clean immutable hosts were alternated A1/B1/B2/A2. Each run measures two 10-second
wheel windows, giving 40 seconds per version. Every run has the same 186 entries,
4101.142857 logical-pixel scroll extent, density 152, actual 160 Hz/DPR 1.75,
zero caption measurements/model-change signals, active-window input, and actual
motion/reversals. Native per-frame probes and material census are disabled.

| Run | Version | Display intervals >9.375 ms | Worst interval (ms) | Window p99 (ms) |
| --- | --- | ---: | ---: | --- |
| A1 | Before | 3 | 12.4966 | 6.3223 / 6.3057 |
| B1 | After | 4 | 18.7288 | 6.3160 / 6.3145 |
| B2 | After | 10 | 12.6203 | 6.3132 / 6.3690 |
| A2 | Before | 12 | 18.9245 | 6.3950 / 6.3480 |

15 long intervals before versus 14 after is not a convincing cadence improvement.
The implementation removes demonstrated redundant rendering work; steady 160 Hz
is still unproven and contradicted by remaining long intervals.

Six selected mode regressions and the embedded-only QML import check pass on each
of Direct3D 11 and OpenGL at 175% (8 mode-test results and 3 import-test results per
backend, including fixture setup/cleanup). This is not a claim that the entire
test suite passes. The existing legacy Details logical-coordinate assertion is
outside this selected set and remains unresolved.

Rendered screenshots were inspected, including small extension and size text.
The Windows system-DLL import audit passes. Embedded payload regeneration,
Go embedding/extraction tests and CGO-disabled launcher build pass with the system
Go cache. The worktree's normal Qt app was restarted without a host override;
its extracted host matches the built host SHA-256
`509c4b470b89b3171bb4f8f7e31faf9d9fc9011800c2e1bc1bdebfb50aa893ca`.
Startup stderr is empty. Neither the canonical checkout app nor other worktree
apps were terminated.

## Remaining work

Investigate a safe shared caption drawing order in Masonry, preserving
selection, overlap behavior, clipping and the physical pixel grid. GC and other
remaining frame spikes still require independent fixes and live verification.

The earlier incremental collector prototype crashed in live testing and is not
part of this implementation or the normal build.

> AI assistance has been used to create this output.
