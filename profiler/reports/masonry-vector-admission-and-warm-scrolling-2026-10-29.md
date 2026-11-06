# Vector admission and verified scrolling — 2026-10-29

## Outcome

Fixed the vector-source admission condition that prevented the real-content
warm-up tour from completing. Two verified warm captures now cover all 191
Downloads entries at 3840×2160, 160 Hz, DPR 1.75. Warm middle scrolling passes
the existing strict gate. Warm wheel scrolling and cold/loading scrolling do
not. Constant 160 Hz without spikes remains an open goal. No diagnostic renderer
variant or portable embedded payload is promoted, and no commit or push is made.

The preceding investigation is recorded in
`masonry-verified-warm-tour-2026-10-29.md`. Its zero accepted warm runs describe
that earlier experiment, not these subsequent captures.

## Admission fix and verification

`MasonryLayoutThumbnails.cpp` uses shared vector classification, including source
metadata and MIME for opaque virtual identities. An SVG source may enter the
existing priority/cancellation queue without bitmap dimensions. Unknown-size
vectors use positive preview bounds as a queue ticket; the vector source supplies
its own authoring viewport. Unknown-size bitmaps retain their existing guard.
No bitmap fallback, static layout change, animation rounding, cache enlargement,
wire-protocol change or new setting is introduced.

`vectorSourceAdmissionDoesNotRequireBitmapDimensions` covers five panel modes,
POD/facade paths, SVG, SVGZ, MIME classification and bitmap rejection: 30 vector
cases fail before the fix; the fixed fixture passes all 40 cases (42 including
initialization and cleanup). Three related native-renderer integration functions
pass (five including initialization/cleanup), covering SVG resize retention,
sparse metadata completion and catalog reorder. Tests run at actual DPR 1.75.

The broader session selection records 40 passes and one failure:
`recycledPreviewNeverLoadsVectorAsBitmap(masonry)`. The same failure occurs in
the matched control: an old photo URL remains where the test expects an empty
URL. It remains unresolved and is not reported as passing.

The prior reset/metadata vector-classification fixes remain included. Their
six-case regression failed before and passes after. The precise earlier layer
that lost known dimensions is still unproven; this fix removes that bitmap
metadata prerequisite from vector admission.

Both diagnostic hosts pass four embedded-import checks and the system-DLL-only
audit. These are explicit diagnostic host overrides, not production packaging.

## Live measurements

All captures preserve real content, normal rendering and the original 100-entry
buffer pool. No UI Automation observer, build, test or analysis runs during the
gestures. Each block is ten seconds. Miss means a display interval over 9.375 ms.
Motion limits remain unchanged: maximum 2 physical pixels and zero coverage
holds; middle scrolling also requires native velocity error at most .002 px.

| Condition/block | Preparation p99 / max ms | Display max ms | Misses | Motion max physical px |
| --- | --- | --- | --- | --- |
| Warm wheel 0 | 2.0538 / 4.2868 | 6.5893 | 0 | 6.206832 |
| Warm wheel 1 | 1.8617 / 2.5737 | 6.3669 | 0 | 2.107170 |
| Warm middle 0 | 1.7154 / 2.9243 | 6.4059 | 0 | .446655 |
| Warm middle 1 | 1.6530 / 2.2703 | 6.3777 | 0 | .396601 |
| Cold wheel 0 | 3.6256 / 6.2994 | 18.7538 | 7 | 88.235074 |
| Cold wheel 1 | 3.4988 / 6.8411 | 12.5192 | 4 | 84.649415 |

All blocks have zero coverage holds. The middle strict analyzer exits zero; the
wheel strict analyzer exits one. Wheel display timestamps are all Composed Flip,
SyncInterval 1, flags 0, tearing disabled. Whether residual warm wheel motion
error reflects timestamp variation or actual displayed motion requires further
independent evidence; the gate is not relaxed.

Warm cases are `real-scroll-vector-admission-wheel-20261029-wheel-masonry` and
`real-scroll-vector-admission-middle-20261029-wheel-masonry` (fresh caches u/v).
The normal overlapping warm-up tour visits all 191 entries at 15 waypoints.
Both record zero pending bitmap requests and zero bitmap stores, misses or frame
count changes during measurement. This proves tour completion and bitmap-cache
quiescence; it does not directly count every SVG source read/generation during
the gesture. The wheel has 1,858 active inputs; middle has 1,014 active checks,
zero inactive checks and pointer displacement one quarter of window height.

Cold case `real-scroll-vector-admission-cold-r2-20261029-wheel-masonry` uses
fresh cache x, normal `setPanelContentY` persistence, and no warm-up tour. It
passes capture guards and records 196 loading observations. The preceding cold
attempt (cache w, without r2) is invalid: disk exhaustion truncated its JSON at
33,554,432 bytes. It contributes no performance conclusion.

## Remaining costs and limits of attribution

Cold preparation peaks are distributed across render-list construction, uploads,
material preparation and work before the renderer body. For example, 6.8411 ms
contains render lists 1.7646 ms, uploads 1.7076 ms, pre-body 1.4730 ms and alpha
materials .8572 ms. No individual recorded RHI call of at least 50 microseconds
lies inside that preparation span. This does not establish that the thread was
continuously scheduled; a scheduler capture is needed for that distinction.

For the 5.5549 ms preparation frame, uploads occupy 4.0372 ms. Per-upload marks
account for 3.7233 ms in vertex/index unmap (pool selection, resource creation
when needed, and enqueueing/copying QRhi buffer updates),
.1058 ms geometry assembly and .0029 ms CPU storage mapping across 98 uploads.
The largest preparation frames contain 152–160 uploads. `frame-cost.json`
preserves the joins and largest upload records. This locates wall time in the
Qt unmap path, not an isolated update-enqueue cost, proven single driver call
or proven CPU hotspot.
GUI dispatch samples are absent in this capture, and frames without a sync
marker are represented as null, not invented zero-duration sync work.

The slowest recorded RHI calls over the cold gestures include Present 9.2175 ms,
UpdateSubresource 1.577 ms, staging CreateTexture2D 1.46 ms and staging fallback
Update 1.2217 ms. These are thresholded samples, not complete API CPU accounting.
The next useful attribution separates QRhi enqueue/allocation work, scheduling
and total frame timing before choosing another optimization.

## Reproduction and artifact handling

Build/test scripts are `build-vector-admission-regression-20261029`,
`build-vector-admission-fix-20261029`, `verify-vector-admission-modes-20261029`
and `build-cold-vector-admission-20261029` under `artifacts` (.py/.cmd pairs).
Analysis scripts are `analyze-vector-admission-warm-20261029.py`,
`analyze-vector-admission-cold-20261029.py` and
`analyze-vector-admission-frame-cost-20261029.py`. Receipts and native test logs
are in `vector-admission-20261029` / `cold-vector-admission-20261029`.

Disk recovery compressed old profiling JSON losslessly and removed only five
inactive Go launchers verified byte-identical to the preserved normal launcher.
The recovery manifest records hashes and reclaimed 820,608,000 bytes. Native
hosts, unique binaries and raw traces were retained. The normal worktree app
remained alive (launcher PID 43936 at the last check). Completed diagnostic
variants must be stopped by exact executable path before using the shared
profiling command file for another variant.
