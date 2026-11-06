# Scalar preview geometry and on-demand cursor origin

Date: 2026-10-19. Goal: constant 160 Hz without spikes. Completion remains unproven.

## Implementation

GalleryDelegateItem exposes previewX/Y/Width/Height directly from the existing native QRectF, with the same single post-commit notify signal. No duplicated cached rectangle or coordinate rounding is added. GalleryEntryDelegateBase preserves effectivePreviewRect for embedders, but local preview, folder, and caption placement consume scalar effective geometry. Existing compact/noncompact arithmetic is preserved. Hidden preview debug zones do not bind their dimensions while disabled.

GalleryCursorController reads the native viewportSceneOrigin only inside the alignment operation, rather than maintaining a point binding on every scroll frame. The native resting pixel-grid phase is preserved; animation alignment suspension is unchanged. No GC constant or dependency default changes.

## Regression and rendering checks

previewScalarsPublishOneCommittedRectangle failed before the implementation because scalar geometry was absent. It passes after the implementation and verifies atomic publication, no redundant notification for an identical rectangle, fractional values, and clearing the rectangle. Direct3D and OpenGL each pass 15 selected mode cases, 14 gallery cases, and 4 static import/shader cases at DPR 1.75. These include cursor motion/rest, deferred thumbnail replacement, all SVG panel modes, hidden fallback behavior, folder previews, and caption alignment. Rendered masonry and Details images were inspected, including the small caption and secondary size text. The portable host DLL audit passes.

## Separate allocation census

This instrumented capture is for counts, not timing. Both runs use continuous actual wheel input, Downloads 186 entries, density 152, 160 Hz, DPR 1.75, and budget 5 ms. The census includes the gap between blocks and may destroy objects created before recording began. There is no cap overflow.

| Metric | Before | After |
| --- | ---: | ---: |
| Created value wrappers | 23,708 | 12,520 |
| QRectF wrappers | 11,040 | 1,167 |
| QPointF wrappers | 1,869 | 164 |
| effectivePreviewRect component reads | 7,711 | 0 |
| viewportSceneOrigin reads | 1,312 | 2 |

Total wrapper creation fell 47.2%. This does not establish the same percentage CPU or frame-time reduction. QColor and QSizeF remain major allocation sources; the captured source locations are retained for the next investigation.

## Clean display comparison

Interleaved A1/B1/B2/A2/A3/B3 fresh processes. A uses the pre-change recorder-ablation host, B the scalar candidate. Both disable the per-frame recorder and census, use original 5 ms GC budget, and send wheel events every 30 ms. Two four-second boundary-bounce blocks per process. No build, test, WPR sampling, or QML profiler overlaps these captures. Both display endpoints must lie inside the input window; leading idle intervals are excluded, internal stalls are retained. All valid captures are kept.

| Run | Block 0 p99 / max (ms) | >9.375 ms | Block 1 p99 / max (ms) | >9.375 ms |
| --- | ---: | ---: | ---: | ---: |
| A1 | 6.4142 / 12.5263 | 6 | 6.3422 / 18.7380 | 3 |
| B1 | 6.3122 / 12.4992 | 3 | 6.3098 / 12.4966 | 2 |
| B2 | 6.3369 / 12.5069 | 3 | 6.3058 / 12.4932 | 1 |
| A2 | 6.3462 / 12.4905 | 2 | 6.2797 / 12.4979 | 1 |
| A3 | 6.2880 / 12.5020 | 2 | 6.2854 / 12.4740 | 1 |
| B3 | 6.2822 / 12.5015 | 2 | 6.2927 / 12.4947 | 1 |

All blocks: A 15, B 12 long display intervals. The six short runs do not prove zero spikes or a deterministic worst-case bound. The wrapper reduction is directly verified; any cadence interpretation must account for pairwise variance.

## Rollout and remaining work

The embedded Qt payload was regenerated and the CGO-disabled Go launcher rebuilt with the system build cache. Payload/extraction tests passed. The experiment archive verifies the launched cached Qt child matches the new host and records final binary identities, direct --gui=qt launch, and empty stderr.

Continue with connected color wrapper churn and non-GC GUI/render preparation. Re-run native attribution if missed refreshes remain. The 160 Hz objective stays active.

Evidence is archived with SHA256 hashes under `profiler/experiments/scalar-preview-geometry-2026-10-19/`; binaries, raw traces, CSVs, and build outputs are excluded.

> AI assistance has been used to create this output.
