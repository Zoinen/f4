# Verified warm route and SVG admission — 2026-10-29

## Outcome

No warm performance run is accepted. A real-content warm-up tour repeatedly
stops on two unresolved SVG previews; it does not hide them or declare the route
warm. Consequently there is no new claim of constant 160 Hz, no accepted warm
wheel comparison, and no accepted quarter-height middle-scroll result.

Progress in this investigation consists of a bounded warm-route harness,
regressions for its admission gate, a reproduced and fixed vector-classification
inconsistency, and a live source trace locating a further thumbnail-admission
failure. The full cold/loading, warm wheel, middle scrolling, motion-error and
fidelity requirements remain in force. No optimization or new portable payload
is promoted from these observations.

## Warm-route harness

`VerifiedWarmTourMethods.inc` visits overlapping viewport positions from top to
bottom and back while retaining normal content, source priorities, animation,
clipping and rendering. It records entry-index coverage and visible-preview
readiness. Each waypoint requires zero pending thumbnail-cache requests, stable
source/store/frame counters and visible identities for at least 500 ms, and a
displayed frame after that stable interval began. Success requires coverage of
every catalog entry. The observer stops before the measured gestures.

The bounds are 30 intervals and 60 seconds for the tour. This folder has 191
entries. Actual display checks require 3840×2160, 160 Hz and DPR 1.75. The
existing `warmScene` visibility-changing experiment is explicitly forbidden.
No larger cache, forced retention or reduced content is used.

The first tour assigned `layout.contentY` directly. Several waypoints reverted
to zero because this bypassed the normal saved-viewport update. The harness now
invokes the same `setPanelContentY(value, true)` method as the scrollbar, and
requested and observed positions agree in the subsequent logs. This correction
is to the diagnostic procedure; the first attempt is not evidence of a newly
introduced production scrolling defect.

The gate also distinguishes unresolved/loading vectors from explicit source
failure with ordinary file-icon presentation. Nine gate tests pass, including
quiet-time resets, content changes, missing swaps, unresolved vectors, explicit
failures and clock ordering. Four embedded-import checks and the system-DLL-only
audit pass for the rebuilt diagnostic hosts.

## Rejected live attempts

All are actual-display diagnostics with a fresh application gallery cache:

- `real-scroll-verified-warm-tour-wheel-20261029-wheel-masonry`
- `real-scroll-verified-warm-tour-wheel-r2-20261029-wheel-masonry`
- `real-scroll-verified-warm-tour-wheel-r3-20261029-wheel-masonry`
- `real-scroll-svg-classification-wheel-20261029-wheel-masonry`
- `real-scroll-svg-classification-source-trace-20261029-wheel-masonry`

The first attempt reaches waypoint 8 but has invalid intermediate positioning.
Later attempts consistently stop at waypoint 5, after covering 141 entries,
at contentY 2918.285714285714. At timeout: 42 visible rows, 20 expected previews,
18 ready previews, zero definitive failures, zero pending bitmap-cache requests.
The two remaining rows are 142 and 143. Their preview rectangles intersect the
viewport, so this is not the earlier hypothesis of caption-only intersection.
Both expose `isVector=true`, `vectorState="loading"`, no vector source and
ordinary fallback icons. They must not be counted as loaded or failed merely
because the bitmap-cache pending count is zero.

PresentMon sessions are uniquely named by case. The first two rejected attempts
needed explicit cleanup of their owned sessions; the harness now stops its
owned session gracefully on failure as well as success. No other application's
trace session is stopped. These attempts contain no accepted measurement
windows and must not be included in performance aggregates.

## Separate classification regression and fix

External catalog append parsing recognizes SVG sources despite an incoming
`isImage=false` raster hint. Full reset and metadata enrichment lacked that
normalization. The new
`vectorClassificationSurvivesExternalRasterHint` regression covers SVG, SVGZ
and SVG MIME detection through both paths. All six cases fail against the
unchanged gallery library and pass with the fix (8 QtTest passes including
setup/cleanup).

`ExternalCatalogResetTransaction.cpp` now recognizes the vector after assembling
its authoritative source descriptor, before retaining previous state.
`ExternalCatalogMetadata.cpp` does the same after source reconciliation, before
classifying source changes and scheduling work. Directories remain excluded;
existing definitive failure state and revision handling are preserved. No
raster fallback, new format, wire change or new source read is introduced.

The checked-in regression is in `tests/GallerySessionTest.cpp`; a small standalone
fixture extracts those exact test bodies for the diagnostic build. Of 41 existing
vector/metadata cases, 40 pass at actual DPR 1.75 with native rendering and Basic
controls. One `recycledPreviewNeverLoadsVectorAsBitmap(masonry)` assertion fails
identically in the unchanged control and the candidate: an old bitmap URL is
still present where the test expects an empty URL. That failure remains recorded
and unresolved; it is not counted as passing coverage. An earlier run omitted
the Basic/native test environment and is retained separately, not used as the
final coverage result.

The classification fix is necessary for consistency, but **does not resolve the
two live previews**. Do not report the live SVG problem fixed from the focused
classification tests alone.

## Source trace: actual remaining admission failure

The final attempt enables media timing for attribution only, not performance
acceptance. The filtered trace is summarized in `live-source-findings.json`.

| Source | Bytes | Materialization | Applied metadata | Later thumbnail plan skips |
| --- | ---: | ---: | --- | ---: |
| `PlayerControl+FileIcons_Dark++_nb_pro_5+.svg` | 64,977 | 2.0326 ms, successful | 1024×1024 | 14 |
| `PlayerControl+FileIcons_Dark++.svg` | 1,500 | 1.2612 ms, successful | 812×9 | 14 |

Both metadata passes complete successfully and are applied to the main `f4-left`
session before the skips. Other sessions reject those broadcasts by namespace,
as intended; those rejections are not failures of the main source request.

The later planner events show `modelIsImage=true` but `knownWidth=-1`. Initially
there is no facade (`facade-deferred`); subsequently an image facade exists and
is classified as an image but its width remains -1 (`facade-not-decodable`).
There are 10 deferred and 18 non-decodable events across the two sources.
Refresh, snapshot-refresh and materialization-pending flags are false. No
thumbnail-admission, image-read or source-decode event is observed for either
source. Thus this is not a slow materialization or a slow SVG parser in flight.

The exact place that loses or fails to propagate the applied size is still
unproven. A direct model KnownImageSizeRole read versus the cached brick/facade
would distinguish the remaining possibilities. Do not infer the responsible
model layer from the cached-brick trace alone.

The next regression should exercise a visible SVG with unavailable bitmap
dimensions. `MasonryLayout::planThumbnailForIndex` currently requires known size
before facade materialization and again before requesting a source. Vector
source acquisition should not depend on raster dimensions: the source supplies
its own size. Prove that case, then admit only recognized vector sources through
the existing bounded viewport/priority queue while retaining bitmap safeguards.
Recheck the live files, complete the full tour, and only then measure warm wheel
and quarter-height middle gestures. The source-size propagation issue must also
remain visible in diagnostics rather than being dismissed as a parser failure.

## Reproducibility and current application

The normal worktree launcher PID 43936 and extracted Qt host PID 119460 remain
alive. Only worktree-specific diagnostic launchers are stopped. No canonical
binary is overwritten, and no commit or push is made. The classification source
fix is present in the checkout and the diagnostic host; a portable production
rollout is not claimed.

The experiment snapshot preserves the harness, rejected attempts, source trace,
analysis, baseline/candidate tests and source changes. Compiled outputs are
external hashed evidence. D: space was recovered through transparent NTFS
compression of old profiling JSONs; their content and paths are unchanged.

AI assistance has been used to create this output.
