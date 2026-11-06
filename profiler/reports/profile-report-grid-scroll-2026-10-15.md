# Live right-panel Grid scrolling profile

Release-optimized Qt 6.11.1 host with temporary QML profiler plugins; same saved tab, directory `D:\Code\f4-zoin`, size and panel settings. Three wheel gestures: down 960, up 960, down 480. Startup excluded. No product source changes were made for profiling.

Mode: full. Trace: `artifacts/grid-scroll-live.qtd`. Native trace: `artifacts/grid-scroll-native.jsonl`.
Trace interval: 19.277 s, including idle gaps between gestures. Parser animation-derived active-time estimate: 307.0 ms; this is not the full recording duration.
Captured range-duration sum: 1014.3 ms (nested binding/JS/creation events, **not wall-clock time**). Non-overlapping QML range time: 306.25 ms. Events: 180494.

## Diagnosis

Scrolling moves an already physically snapped C++ viewport, but QML still propagates its scene origin through every tile and walks the full ancestor chain. That invalidates per-icon and per-caption correction bindings repeatedly. The pixel-alignment path occupies about 84% of exclusive captured QML range time; the two JavaScript functions alone use 169.11 ms. Keep alignment, but cache the local correction/physical phase and invalidate on actual geometry, DPR, mode or ancestor-transform changes, rather than on every whole-pixel scroll translation.

A second avoidable cost is native Grid caption shaping: `applyDelegateRowLayout()` constructs a `GalleryIconTextMeasurer` and wraps/elides every retained tile caption during every viewport commit. Last 35 native commits: median total 2.728 ms, median layout-metadata phase 2.148 ms; maximum total 17.956 ms includes delegate churn. Cache the result by text, font and caption width, and recompute only when those inputs change. No source/identity/geometry changes are needed for retained tiles during ordinary scrolling.

Numbers include profiler overhead and enabled diagnostic logging. Inclusive parent/child hotspot totals overlap; use exclusive totals for attribution. This capture identifies GUI-thread work, not GPU execution time.

## Event types

| Type | Count | Inclusive ms | Inclusive ms/animation frame |
|---|---:|---:|---:|
| Binding | 29072 | 622.23 | 17.778 |
| Javascript | 47402 | 351.53 | 10.044 |
| Creating | 987 | 21.65 | 0.619 |
| HandlingSignal | 328 | 18.64 | 0.533 |
| Compiling | 2 | 0.25 | 0.007 |

Counts scale with interaction duration. Per-frame values here are also nested inclusive sums, so they must not be added to estimate elapsed frame time.

## Frames

Frame period is lower when scrolling is smoother. p50 is median; 5%/1% exceed p95/p99. A 60 Hz budget is 16.67 ms; over 33 ms is visible stutter and over 50 ms is a stall. These periods are derived from Qt animation framerate samples, not presentation timestamps.

35 animation samples: p50 12.05 ms, p95 **25 ms** (~2 samples at/above), p99 **29.41 ms** (~1 sample), maximum **29.41 ms**. Two samples exceed 25 ms; none exceed 33 or 50 ms. The short wheel bursts do not reproduce a severe sustained stall, but expose redundant per-frame work. Average framerate 113.9, minimum 34, maximum 1000; the extreme maximum makes average FPS a poor headline.

## Memory

The capture shows substantial temporary JS churn, not evidence of a leak. 94,540 JS allocations total 18.39 MiB, with 60.3% reclaimed. Peak GC pages 17.54 MiB; final GC pages 16.06 MiB. Final live small JS objects 7.30 MiB. Exit here means recording end, not process exit.

| Category | Allocations | Allocated | Reclaimed | Peak live | Live at recording end |
|---|---:|---:|---:|---:|---:|
| GC pages | 46 | 17.54 MiB | 1.48 MiB | 17.54 MiB | 16.06 MiB |
| Small JS objects | 94540 | 18.39 MiB | 11.09 MiB | 17.42 MiB | 7.30 MiB |

GC pages are allocator storage; small JS objects are individual allocations. No large-item allocation events were recorded.

## Pixmap cache

13 load requests, 13 loaded, 12 removed. Most are small cached thumbnails or bundled interface icons; none are full-resolution photos. This is not the dominant cost.

| Source | Width × height | Pixels |
|---|---:|---:|
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 172 × 172 | 29584 |
| `qrc:/F4QtHost/icons/zoin/folder-up.svg` | 172 × 138 | 23736 |
| `qrc:/F4QtHost/icons/zoin/folder.svg` | 172 × 138 | 23736 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8030c596ad738514-v1791931959235077400:7377-s7377-94x71-7` | 94 × 71 | 6674 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8030c596ad738514-v1791931959235077400:7377-s7377-94x71-7` | 94 × 71 | 6674 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-7-2bdd57bd6ea23100-v1791147831554246000:8880-s8880-67x24-9` | 67 × 24 | 1608 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-7-87f774e4a05a3d6b-v1791147831554746500:8565-s8565-67x24-10` | 67 × 24 | 1608 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-11-a430eaaebd54f33e-v1791396821100044800:5141981-s5141981-44x24-13` | 44 × 24 | 1056 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-11-37a4b044f4c80585-v1791396797408371400:4544692-s4544692-44x24-14` | 44 × 24 | 1056 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-11-9b108c4b20964f71-v1791396820596077500:4187325-s4187325-65x16-12` | 65 × 16 | 1040 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-7-a88e210db5449e7c-v1791147831553746500:7826-s7826-33x24-8` | 33 × 24 | 792 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-7-c88dad34072bea61-v1791147831555746300:9065-s9065-33x24-11` | 33 × 24 | 792 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-11-bbba06a66cc91652-v1791933122140277800:141713-s141713-31x24-15` | 31 × 24 | 744 |

## Top hotspots (inclusive)

| Rank | ms | Count | Average ms | ms/frame | Type | Source | Details |
|---:|---:|---:|---:|---:|---|---|---|
| 1 | 170.28 | 3757 | 0.045 | 4.865 | Binding | [GalleryEntryDelegateBase.qml:73](../../third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml#L73) |  |
| 2 | 104.12 | 14862 | 0.007 | 2.975 | Javascript | [GalleryEntryDelegateBase.qml:113](../../third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml#L113) | iconPixelOffset |
| 3 | 89.67 | 35 | 2.562 | 2.562 | Binding | [GalleryCursorController.qml:88](../../third_party/ZoinGallery/qml/GalleryCursorController.qml#L88) |  |
| 4 | 89.46 | 35 | 2.556 | 2.556 | Binding | [GalleryPanel.qml:316](../../third_party/ZoinGallery/qml/GalleryPanel.qml#L316) |  |
| 5 | 83.61 | 3746 | 0.022 | 2.389 | Binding | [GalleryEntryDelegateBase.qml:92](../../third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml#L92) |  |
| 6 | 71.77 | 7388 | 0.01 | 2.051 | Binding | [GridEntryDelegate.qml:24](../../third_party/ZoinGallery/qml/GridEntryDelegate.qml#L24) |  |
| 7 | 64.99 | 3757 | 0.017 | 1.857 | Javascript | [GalleryEntryDelegateBase.qml:73](../../third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml#L73) | expression for iconSceneOrigin |
| 8 | 62.26 | 6916 | 0.009 | 1.779 | Binding | [GalleryEntryPreview.qml:272](../../third_party/ZoinGallery/qml/GalleryEntryPreview.qml#L272) |  |
| 9 | 61.36 | 7388 | 0.008 | 1.753 | Javascript | [GridEntryDelegate.qml:24](../../third_party/ZoinGallery/qml/GridEntryDelegate.qml#L24) | expression for pixelCorrection |
| 10 | 52.62 | 6916 | 0.008 | 1.503 | Javascript | [GalleryEntryPreview.qml:272](../../third_party/ZoinGallery/qml/GalleryEntryPreview.qml#L272) | expression for pixelGridOffset |
| 11 | 10.54 | 3746 | 0.003 | 0.301 | Javascript | [GalleryEntryDelegateBase.qml:92](../../third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml#L92) | expression for iconPixelAlignmentRevision |
| 12 | 8.03 | 60 | 0.134 | 0.229 | Binding | [GalleryEntryDelegateBase.qml:302](../../third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml#L302) |  |
| 13 | 5.87 | 35 | 0.168 | 0.168 | HandlingSignal | [GalleryPanelChrome.qml:72](../../third_party/ZoinGallery/qml/GalleryPanelChrome.qml#L72) |  |
| 14 | 5.83 | 35 | 0.167 | 0.167 | Javascript | [GalleryPanelChrome.qml:72](../../third_party/ZoinGallery/qml/GalleryPanelChrome.qml#L72) | onContentYChanged |
| 15 | 4.48 | 468 | 0.01 | 0.128 | Binding | [GalleryThumbnailShaderLayer.qml:57](../../third_party/ZoinGallery/qml/GalleryThumbnailShaderLayer.qml#L57) |  |
| 16 | 3.74 | 468 | 0.008 | 0.107 | Javascript | [GalleryThumbnailShaderLayer.qml:57](../../third_party/ZoinGallery/qml/GalleryThumbnailShaderLayer.qml#L57) | expression for pixelCorrection |
| 17 | 3.41 | 66 | 0.052 | 0.097 | Binding | [GalleryEntryPreview.qml:138](../../third_party/ZoinGallery/qml/GalleryEntryPreview.qml#L138) |  |
| 18 | 3.41 | 50 | 0.068 | 0.097 | Binding | [GalleryEntryPreview.qml:83](../../third_party/ZoinGallery/qml/GalleryEntryPreview.qml#L83) |  |
| 19 | 3.17 | 62 | 0.051 | 0.091 | Creating | [GalleryEntryPreview.qml:164](../../third_party/ZoinGallery/qml/GalleryEntryPreview.qml#L164) | QtQuick.Controls.impl/IconImage |
| 20 | 2.95 | 29 | 0.102 | 0.084 | Javascript | [GalleryPanel.qml:519](../../third_party/ZoinGallery/qml/GalleryPanel.qml#L519) | traceBenchmarkStage |
| 21 | 2.93 | 29 | 0.101 | 0.084 | Javascript | [GalleryPanelDiagnostics.qml:85](../../third_party/ZoinGallery/qml/GalleryPanelDiagnostics.qml#L85) | trace |
| 22 | 2.69 | 52 | 0.052 | 0.077 | Binding | [GalleryEntryPreview.qml:173](../../third_party/ZoinGallery/qml/GalleryEntryPreview.qml#L173) |  |
| 23 | 2.65 | 35 | 0.076 | 0.076 | HandlingSignal | [GalleryViewport.qml:349](../../third_party/ZoinGallery/qml/GalleryViewport.qml#L349) |  |
| 24 | 2.6 | 35 | 0.074 | 0.074 | Javascript | [GalleryViewport.qml:349](../../third_party/ZoinGallery/qml/GalleryViewport.qml#L349) | expression for onContentYChanged |
| 25 | 2.59 | 43 | 0.06 | 0.074 | Javascript | [GallerySelectionController.qml:62](../../third_party/ZoinGallery/qml/GallerySelectionController.qml#L62) | refreshHoveredIndex |
| 26 | 2.33 | 29 | 0.08 | 0.067 | HandlingSignal | qrc:/F4QtHost/qml/GalleryPanelHost.qml |  |
| 27 | 2.3 | 29 | 0.079 | 0.066 | Javascript | qrc:/F4QtHost/qml/GalleryPanelHost.qml |  |
| 28 | 2.27 | 41 | 0.055 | 0.065 | Javascript | [GalleryPanelFacade.qml:14](../../third_party/ZoinGallery/qml/GalleryPanelFacade.qml#L14) | refreshHoveredIndex |
| 29 | 2.27 | 29 | 0.078 | 0.065 | Javascript | qrc:/F4QtHost/qml/GalleryPanelHostAdapter.qml | forwardBenchmarkStage |
| 30 | 2.03 | 12 | 0.169 | 0.058 | Creating | [GalleryViewport.qml:384](../../third_party/ZoinGallery/qml/GalleryViewport.qml#L384) | GalleryEntryDelegateBase |

## Top five source locations

### [GalleryEntryDelegateBase.qml:73](../../third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml#L73)

`iconSceneOrigin` walks ancestors and subscribes to x/y/scale/rotation. Scroll moves an ancestor and wakes this for every tile. 3,757 reevaluations; 170.28 ms inclusive. Cache the shared viewport phase and observe structural/static geometry separately.

```qml
            Math.max(0, Math.floor((Math.round(height * renderDpr) - 1) / 2)) / renderDpr,
            Math.round(Math.max(0, panelRoot.metrics.cellVerticalPadding) * renderDpr) / renderDpr)
    }
    readonly property point iconSceneOrigin: {
        if (detailsMode)
            return Qt.point(0, 0)
        // mapToItem() does not register dependencies on ancestor geometry.
        // Track geometry through the complete chain so host
        // panel placement invalidates the correction without any scrolling.
        let dependency = 0
        let ancestor = entry
        while (ancestor) {
            dependency += ancestor.x + ancestor.y + ancestor.scale + ancestor.rotation
            // Size does not affect an untransformed item's origin. Anchors
            // publish x/y when resizing actually moves it; subscribing every
```

### [GalleryEntryDelegateBase.qml:113](../../third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml#L113)

`iconPixelOffset` maps each leaf parent to scene coordinates and rounds the result. 14,862 calls; 104.12 ms exclusive. Reuse the local correction while the viewport moves by whole physical pixels; keep scene-space regressions for host placement and DPR.

```qml
                           / renderDpr)
    }

    function iconPixelOffset(item) {
        if (!visible || !item || !item.parent || !item.visible)
            return Qt.point(0, 0)
        const revision = iconPixelAlignmentRevision
        const origin = iconSceneOrigin
        // Details' brick origin is already aligned by C++; only the short
        // chain inside the brick can contribute a local centering offset.
        const scenePoint = item.parent.mapToItem(detailsMode ? entry : null, item.x, item.y)
        return Qt.point(
            Math.round(scenePoint.x * renderDpr) / renderDpr
                - scenePoint.x + revision * 0 + origin.x * 0,
            Math.round(scenePoint.y * renderDpr) / renderDpr
```

### [GalleryCursorController.qml:88](../../third_party/ZoinGallery/qml/GalleryCursorController.qml#L88)

The cursor viewport-origin forwarding binding fans out into all tile correction dependencies. 35 evaluations, 89.67 ms inclusive, mostly descendant work. Publish a stable physical phase or geometry revision instead of a changing absolute translation to each leaf.

```qml
                cursorChromeTargetWidth, cursorChromeTargetHeight)
    // C++ publishes the actual snapped viewport translation. Reconstructing
    // it from contentY would put the cursor back on the analytical fraction.
    readonly property point cursorPixelGridViewportOrigin:
        galleryLayout.viewportSceneOrigin
    readonly property bool cursorPixelAlignmentSuspended:
        cursorChromeTransitionActive || panelScrollAnimation.running

    function alignViewportItemRectToDevicePixels(item, rect) {
        if (!item || !rect)
            return rect
        const dpr = Math.max(0.01, Number(devicePixelRatio) || 1)
        const origin = cursorPixelGridViewportOrigin
        const itemSceneX = origin.x + item.x
        const itemSceneY = origin.y + item.y
```

### [GalleryPanel.qml:316](../../third_party/ZoinGallery/qml/GalleryPanel.qml#L316)

The panel alias forwards the same cursor origin again. 35 evaluations, 89.46 ms inclusive; this overlaps the previous item and is not an additional 89 ms cost. Stop full absolute-origin changes from invalidating every tile.

```qml
    readonly property rect cursorChromeRect: cursorState.cursorChromeRect
    readonly property rect cursorChromeTargetRect:
        cursorState.cursorChromeTargetRect
    readonly property point cursorPixelGridViewportOrigin:
        cursorState.cursorPixelGridViewportOrigin
    readonly property bool cursorPixelAlignmentSuspended:
        cursorState.cursorPixelAlignmentSuspended

    GalleryCursorController {
        id: cursorState
        panel: root
        controller: root.controller
        layoutItem: root.galleryLayout
        scrollAnimation: root.panelScrollAnimationObject
        chromeAnimation: root.cursorChromeAnimationObject
```

### [GalleryEntryDelegateBase.qml:92](../../third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml#L92)

`iconPixelAlignmentRevision` includes viewport coordinates in its dependency expression. 3,746 evaluations, 83.61 ms inclusive. Separate local size/preview/mode invalidations from scroll movement; no extra rounding loops are needed.

```qml
        }
        return entry.mapToItem(null, dependency * 0, dependency * 0)
    }
    readonly property real iconPixelAlignmentRevision: {
        if (!visible)
            return 0
        const viewport = detailsMode ? Qt.point(0, 0)
                                     : panelRoot.cursorPixelGridViewportOrigin
        const preview = previewContainerItem
        return Number(viewport.x || 0) + Number(viewport.y || 0)
                + (detailsMode ? 0 : x + y) + width + height
                + (preview ? preview.x + preview.y
                             + preview.width + preview.height : 0)
                + presentationMode
    }
```

## Next steps

1. Remove scroll-driven alignment fan-out while retaining physical-pixel correctness. Validate actual icon and caption leaves at DPR 1.75, including after scrolling, resizing and panel movement.
2. Cache Grid caption measurement for retained delegates; avoid rebuilding QTextLayout when text/font/width are unchanged.
3. Reprofile longer repeated wheel/drag sequences with native logging disabled to measure real presentation stalls. QML profiling changes timing; this report is attribution evidence, not a production latency guarantee.
The related QML review targets are GalleryEntryDelegateBase.qml, GalleryCursorController.qml, GalleryPanel.qml and GridEntryDelegate.qml.

> AI assistance has been used to create this output.
