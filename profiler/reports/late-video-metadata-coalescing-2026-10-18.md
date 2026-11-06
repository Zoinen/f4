# Coalesce late cached-video geometry updates

## Change and regression

A cached video thumbnail completion carries isCached in request.info. When its
dimensions refine existing metadata, the old handleImageReady path sent that
information through handleImageInfo, which published CachedMetadataBatchRole.
Masonry therefore flushed its layout synchronously inside imageReady, using
the path intended for atomic startup metadata restoration.

The new path applies the same authority, revision, completed-field merging and
dimension handling, preserving isCached. Its ImageInfoBatchState explicitly
disallows the cached-restoration flush. Late geometry joins the existing
coalesced layout update. The actual metadata restoration signal and its single
cached range remain unchanged.

lateVideoThumbnailDoesNotFlushCachedMetadata fails before the fix for the
cached-pixels case. Both decoded and cached-pixels cases pass after the fix.
The expanded fixture attaches a real MasonryLayout and checks that its revision
does not change inside imageReady, then advances after event processing with
the correct portrait aspect ratio. The focused session selection has nine
passing results, including cached startup batches, completed-field preservation,
portrait catalog reset and failure fallback.

Before-fix evidence: `artifacts/video-metadata-before-175.txt`.
After-fix evidence: `artifacts/video-metadata-layout-after-175.txt`.

## Clean displayed-frame measurements

All runs use fresh processes, the committed normal Qt renderer, Downloads with
184 entries, density 152, initial extent 4161.142857, actual DPR 1.75 and 160 Hz.
Wheel delta 120 repeats every 30 ms for four seconds in each direction.
PresentMon and basic frame hooks run without source observers, CPU sampling,
UI Automation, compilation or tests. The interval renderer prototype is not
used in these pairs.

| Run | Down display p99 | Down maximum | Down intervals >9.375 ms | Up display p99 | Up intervals >9.375 ms |
|---|---:|---:|---:|---:|---:|
| Baseline A1 | 18.7685 ms | 25.0104 ms | 35 | 18.7505 ms | 30 |
| Candidate B1 | 25.1139 ms | 43.8059 ms | 38 | 6.4816 ms | 6 |
| Baseline A2 | 31.2781 ms | 87.4868 ms | 43 | 12.5087 ms | 17 |
| Candidate B2 | 24.9752 ms | 32.7758 ms | 26 | 6.3419 ms | 5 |

Capture stems are `video-meta-baseline-a1`, `video-meta-candidate-b1`,
`video-meta-baseline-a2`, `video-meta-candidate-b2`, followed by
`-wheel-masonry`, under `artifacts/modes-profile`. Each has native JSON,
PresentMon CSV and a display summary. Metadata confirms the intended window
is active, actual DPR and workload match, and caption measurement count is zero.

Both return-scroll comparisons improve. Cold downward results remain variable:
the first candidate is worse, the second better. Two pairs do not establish
a robust cold-scroll improvement, and none proves constant 160 Hz.

## Callback attribution after the change

A separate instrumented run, `video-meta-fixed-probe160-wheel-masonry`, uses
the saved interval renderer to compare the callback chain with the preceding
diagnosis. It is not a clean frame-rate comparison. In the active window,
onImageReady peaks at 3.6247 ms and metadata dataChanged at 3.563 ms, compared
with previously observed 6–8 ms deliveries. Coalescing does not remove every
synchronous cost: handleLightweightMasonryDelta still peaks at 3.5519 ms, and
a separate visual snapshot update reaches 6.2153 ms. These remain investigation
targets alongside first-time SVG geometry uploads and full renderer rebuilds.

## Verification notes

Native renderer regressions pass four results at actual DPR 1.75. The focused
Masonry selection initially passes five results for cached geometry, complete
metadata rows and video field columns. Rendered inspection then finds that
the column fixture can pass geometry checks with missing row glyphs: an empty
font-family override resolves to unavailable MS Sans Serif in this environment.
The fixture now selects an installed Windows font explicitly and asserts
visible text pixels in every tested field, in addition to existing scene-space
origins and unit-transform checks. The final focused selection passes five
results in `artifacts/video-metadata-final-pixels-175.txt`. The capture at
`artifacts/video-metadata-pixels/gallery-video-fields-175.png` is inspected:
all nine field values and the smaller row labels render. The earlier blank
capture is not sharpness evidence.

Resource-only QML imports and compiled-host startup pass four results. Tagged
embedded payload/materialization tests pass, and the Windows import audit
exits successfully. The rebuilt host SHA256 is
`39dd3e3485b04d23febd8e9685f1cd7744cf32effa0bce28bce541538e9942d3`.
The generated payload and staged CGO-disabled launcher contain that host.
The worktree launcher is replaced and restarted with --gui=qt and no diagnostic
override. Its extracted child has that exact hash, identifies the current
branch in its title, and has empty startup stderr; identity is recorded in
`artifacts/video-metadata-portable-identity.json`. Cross-platform release
verification is not claimed.

AI assistance has been used to create this output.
