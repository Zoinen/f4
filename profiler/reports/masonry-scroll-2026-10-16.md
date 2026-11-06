# Masonry scrolling profile — 2026-10-16

Masonry is not consistently meeting the 160 Hz frame budget. Two warm, unrecorded runs give p95 **14.93 ms**, p99 **25.18 ms**, and a worst frame of **60.31 ms**. The largest identified GUI stall is an unintended raster SVG load during reuse of a bitmap preview. This is a regression in the preview reuse path.

## Capture and validity

- Worktree: `D:/Code/f4-zoin`, branch `zoin`; current Release host and existing local changes.
- Folder: `C:/Users/xs/Downloads`, 183 entries; left panel Masonry, density 152.
- Window: 2194×1179 logical pixels, DPR 1.75; screen 160 Hz.
- Native frame intervals come from `QQuickWindow::frameSwapped`, with QML recording, native trace logging, renderer debug logging and builds disabled during the two headline runs.
- The existing panel NumberAnimation performs a four-second downward scroll. GUI samples confirm the animation and the alignment-suspension flag remain active, with one unchanged layout revision and zero caption measurements in both warm runs.
- Actual downward travel was about 3835–3838 of 4221 logical pixels. This is a measured scrolling workload, not a claim that every run reached the bottom. The animation driver does not advance by the full wall-clock interval during stalls.
- Return animations were interrupted by normal viewport restoration in some runs, so all headline and attribution numbers use the downward phase only. Idle gaps and partial upward phases are excluded.
- Other desktop applications remained running, including a video conference. No claim is made that their load caused the measured stalls. Whole-window render submission also includes the unchanged right Details panel.
- The diagnostic host uses the same native objects/QML resources as the normal build, with a separate main object for timing and profiler support. No product source was changed in this profiling pass.
- Full trace: [qmlprofiler-trace-f4-masonry-2026-10-16.qtd](D:/Code/f4-zoin/profiler/traces/qmlprofiler-trace-f4-masonry-2026-10-16.qtd); parser summary: [masonry-phases-summary.json](D:/Code/f4-zoin/artifacts/modes-profile/masonry-phases-summary.json); phase/asset analysis: [masonry-now-diagnosis.json](D:/Code/f4-zoin/artifacts/modes-profile/masonry-now-diagnosis.json).
- QML trace wall span: 71.897 s including idle time. Animation-derived run estimate: 4.005 s. Isolated downward attribution window: 3.992 s.
- Sum of captured range-event durations (bindings/JS/creation/etc): 1884.59 ms; **not wall-clock time**. Total events: 330,473.
- Native/QML clock alignment error: median 0.0036 ms, p95 0.0239 ms.

## Measured frame pacing

Frame time is the wall-clock gap between submitted frames. p95/p99 mean that 5%/1% of measured intervals were worse. At 160 Hz the budget is **6.25 ms**. A gap of at least **9.375 ms** is used as a conservative missed-refresh indicator; tiny jitter around 6.25 ms is not counted as a dropped refresh.

| Warm run | Intervals | p50 | p95 | p99 | Worst | ≥9.375 ms | >33 ms | >50 ms |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| [masonry-baseline-warm2-masonry](D:/Code/f4-zoin/artifacts/modes-profile/masonry-baseline-warm2-masonry.json) | 558 | 6.20 | **15.74** | **25.03** | **56.31** | 78 | 2 | 1 |
| [masonry-baseline-warm3-masonry](D:/Code/f4-zoin/artifacts/modes-profile/masonry-baseline-warm3-masonry.json) | 557 | 6.24 | **14.63** | **24.50** | **60.31** | 74 | 4 | 1 |

Combined: 152/1115 intervals (13.6%) crossed the conservative missed-refresh threshold. Approximately 56 intervals lie in the slowest 5%, and 11 in the slowest 1%.

| Native phase, warm runs | p50 | p95 | p99 | Worst |
|---|---:|---:|---:|---:|
| Scene synchronization | 0.013 | 0.264 | 0.430 | 1.001 |
| Render submission CPU | 1.899 | 5.724 | 12.090 | 26.192 |

Render submission time is not GPU execution time. GUI, render-thread and presentation phases overlap; their percentiles must not be added together. These signals do not establish the individual driver/batching/GPU costs.

## Largest confirmed stall: SVG passed to the old bitmap component

[GalleryEntryPreview.qml:245](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:245) keeps a compatible preview object and defers changing its `retainedComponent` with `Qt.callLater`. However, the old bitmap component remains connected to the shared `thumbnail.source`. When that source changes to the next vector URL, the retained shader thumbnail immediately receives the SVG. [GalleryThumbnailShaderLayer.qml:27](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryThumbnailShaderLayer.qml:27) loads it synchronously through `Image` before the deferred vector component replaces it.

The trace contains SVG pixmap size/load events **inside that exact bitmap source binding**. This is not an inference from a filename extension or an unsupported-feature fallback in VectorImage. The source SVGs contain no embedded `<image>` elements.

| SVG loaded by bitmap Image | Raster pixels | Load span | Enclosing source binding |
|---|---:|---:|---:|
| FileIcons_Dark+_nb_pro_1 (1).svg | 1792×1792 | 18.425 ms | 18.710 ms |
| FileIcons_Dark+_nb_pro_1 (2).svg | 1792×1792 | 17.934 ms | 18.205 ms |
| FileIcons_Dark+_nb_pro_1.svg | 1792×1792 | 20.074 ms | 20.418 ms |
| FileIcons_Dark+.svg | 1792×1792 | 4.486 ms | 4.766 ms |
| FileIcons_Dark+64.svg | 112×112 | 0.815 ms | 1.030 ms |
| menu.svg | 49×49 | 0.298 ms | 0.591 ms |
| Microsoft-Windows-Logo-2--Streamline-Logos.svg | 42×42 | 0.331 ms | 0.573 ms |

The four 1792×1792 loads sum to **60.92 ms**, equivalent to about 49 MiB at 32 bits per pixel. The surrounding seven-row native commit took **73.64 ms**, including **64.24 ms** in assignment notifications. The corresponding instrumented frame interval was **91.69 ms**, with approximately **74.53 ms** of QML exclusive time and **13.41 ms** of render submission CPU. Instrumentation affects the absolute duration; the unrecorded worst intervals were 56–60 ms, and an earlier full QML capture independently showed the same source-binding stall.

First fix: give each retained preview component a type-safe source. A bitmap component must receive an empty source while the next entry is vector, and a vector component must not receive a photo URL. Keep identity, source type, final geometry and activation coherent through the complete row commit and row release. Preserve existing same-entry retention without publishing replacement media to an outgoing renderer. Add a regression that recycles an already materialized bitmap delegate into an SVG and asserts zero raster SVG loads, not merely a vector-only initial render.

## Remaining work after that bug

1. **Reduce row-entry and row-release cascades.** In the detailed downward run, the native commit p95 was 6.54 ms. The post-delegate release phase reached 10.36 ms when old rows were cleared. [MasonryLayoutDelegateCommit.cpp:410](D:/Code/f4-zoin/third_party/ZoinGallery/MasonryLayoutDelegateCommit.cpp:410) clears visual identity, facade, model and view indexes in separate steps. Batch coherent changes and avoid waking fallback source/size/tint bindings for outgoing or thumbnail-covered entries. Check the completed type/geometry transaction before considering bounded preparation ahead of the visible edge.
2. **Reduce renderer work on entering tiles.** Warm render submission CPU p95/p99 is 5.72/12.09 ms. The detailed sweep still creates 26 GalleryThumbnailShaderLayer instances and records 107 repeated cached-thumbnail requests. The QML Image uses `cache: false`; each source detach/reattach creates QML pixmap/scene work even though the library bitmap is cached. Keep compatible texture/preview state reusable, then measure batching and texture upload separately. No GPU bottleneck is claimed by this capture.
3. **Remove unnecessary per-frame notifications.** [GalleryPanelChrome.qml:72](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelChrome.qml:72) and its hidden horizontal scrollbar binding consume about 95 ms exclusive over the selected downward phase. [GalleryEntryDelegateBase.qml:76](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:76) is still reevaluated around 35,900 times despite the alignment-suspension flag being true. Its early return avoids the ancestor walk during motion, but remaining invalidations still cost about 84 ms including the enclosing binding. Narrow dependency/notification scopes while preserving full scene-space alignment at rest; do not add more rounding.

Visible-index lookup itself is cheap: median 0.0016 ms, p95 0.0028 ms. The warm geometry revision stays unchanged, caption measurements remain zero, and all **107/107** synchronous thumbnail-provider lookups are cache hits (2.367 ms total, worst 0.0814 ms; largest returned bitmap 0.58 MiB). Actual photo decoding/cache reads are therefore not the principal cause of these warm scrolling stalls.

## QML event totals

The following totals cover the full trace, including setup and idle-time activity. Nested inclusive durations overlap. Counts scale with capture length; the isolated exclusive attribution below is more useful for prioritizing scroll work.

| Type | Count | Inclusive total, ms | ms / animation event |
|---|---:|---:|---:|
| Binding | 72,662 | 1455.48 | 2.271 |
| Javascript | 105,100 | 306.14 | 0.478 |
| HandlingSignal | 4,605 | 107.38 | 0.168 |
| Creating | 651 | 15.59 | 0.024 |

The raw animation counter reports p95/p99 7.04/7.04 ms and zero >33 ms events over 641 events. It measures the animation driver, not the actual frame-swap gaps, and demonstrably misses the 91.69 ms stall. It must not be used to claim 160 Hz smoothness.

Isolated downward QML exclusive work: **586.69 ms**, **1.027 ms/frame** on average. Bursts matter more than this average.

| Source / event | Calls | Exclusive ms | Largest inclusive call, ms |
|---|---:|---:|---:|
| [GalleryPanelChrome.qml:72](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelChrome.qml:72) — Javascript onContentYChanged | 570 | 69.481 | 0.575 |
| [GalleryThumbnailShaderLayer.qml:27](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryThumbnailShaderLayer.qml:27) — Binding  | 125 | 67.704 | 20.418 |
| [GalleryEntryPreview.qml:112](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:112) — Binding  | 368 | 57.571 | 1.811 |
| [GalleryEntryDelegateBase.qml:76](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:76) — Javascript expression for iconSceneOrigin | 35,890 | 51.174 | 5.359 |
| [GalleryEntryPreview.qml:192](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:192) — Binding  | 329 | 44.251 | 0.707 |
| [GalleryEntryPreview.qml:190](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:190) — Binding  | 217 | 33.970 | 1.075 |
| [GalleryEntryDelegateBase.qml:76](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:76) — Binding  | 35,894 | 32.344 | 5.366 |
| [GalleryPanelChrome.qml:105](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelChrome.qml:105) — Binding  | 570 | 25.331 | 0.204 |
| [GalleryEntryPreview.qml:186](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:186) — Binding  | 254 | 23.557 | 0.468 |
| [MasonryEntryDelegate.qml:46](D:/Code/f4-zoin/third_party/ZoinGallery/qml/MasonryEntryDelegate.qml:46) — Binding  | 498 | 7.298 | 0.281 |
| [GalleryEntryPreview.qml:281](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:281) — Binding  | 462 | 6.602 | 0.405 |
| [GalleryEntryDelegateBase.qml:185](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:185) — Binding  | 367 | 5.722 | 1.996 |
| [GalleryEntryDelegateBase.qml:51](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:51) — Binding  | 115 | 5.012 | 2.094 |
| [GalleryGroupHeaders.qml:12](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryGroupHeaders.qml:12) — Binding  | 570 | 4.555 | 0.042 |
| [GalleryCursorController.qml:111](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryCursorController.qml:111) — Javascript indexIntersectsViewport | 570 | 4.119 | 0.023 |
| [GalleryEntryDelegateBase.qml:305](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:305) — Binding  | 538 | 4.103 | 20.545 |
| [GalleryThumbnailShaderLayer.qml:28](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryThumbnailShaderLayer.qml:28) — Binding  | 142 | 4.076 | 0.443 |
| [GalleryEntryDelegateBase.qml:226](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:226) — Javascript isLucideIconSource | 1,352 | 3.359 | 0.028 |
| [GalleryEntryDelegateBase.qml:242](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:242) — Javascript sourceColorIconAtSize | 589 | 3.313 | 0.053 |
| [GalleryCursorController.qml:88](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryCursorController.qml:88) — Binding  | 570 | 2.953 | 0.045 |

## Top 30 full-trace hotspots

| # | Total ms | Calls | Average ms | ms / animation event | Type | Source | Details |
|---:|---:|---:|---:|---:|---|---|---|
| 1 | 149.36 | 580 | 0.258 | 0.233 | Binding | [GalleryEntryDelegateBase.qml:305](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:305) |  |
| 2 | 128.51 | 387 | 0.332 | 0.200 | Binding | [GalleryEntryDelegateBase.qml:185](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:185) |  |
| 3 | 104.07 | 386 | 0.270 | 0.162 | Binding | [GalleryEntryPreview.qml:112](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:112) |  |
| 4 | 93.19 | 40,180 | 0.002 | 0.145 | Binding | [GalleryEntryDelegateBase.qml:76](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:76) |  |
| 5 | 80.62 | 558 | 0.144 | 0.126 | Binding | [GalleryEntryPreview.qml:213](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:213) |  |
| 6 | 77.91 | 641 | 0.122 | 0.122 | HandlingSignal | [GalleryPanelChrome.qml:72](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelChrome.qml:72) |  |
| 7 | 77.29 | 641 | 0.121 | 0.121 | Javascript | [GalleryPanelChrome.qml:72](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelChrome.qml:72) | onContentYChanged |
| 8 | 72.06 | 133 | 0.542 | 0.112 | Binding | [GalleryEntryPreview.qml:359](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:359) |  |
| 9 | 71.34 | 133 | 0.536 | 0.111 | Binding | [GalleryThumbnailShaderLayer.qml:27](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryThumbnailShaderLayer.qml:27) |  |
| 10 | 62.38 | 459 | 0.136 | 0.097 | Binding | [GalleryEntryDelegateBase.qml:58](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:58) |  |
| 11 | 56.83 | 40,176 | 0.001 | 0.089 | Javascript | [GalleryEntryDelegateBase.qml:76](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:76) | expression for iconSceneOrigin |
| 12 | 55.43 | 555 | 0.100 | 0.086 | Binding | [GalleryEntryPreview.qml:100](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:100) |  |
| 13 | 52.04 | 618 | 0.084 | 0.081 | Binding | [GalleryEntryDelegateBase.qml:172](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:172) |  |
| 14 | 51.90 | 271 | 0.192 | 0.081 | Binding | [GalleryEntryDelegateBase.qml:140](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:140) |  |
| 15 | 49.67 | 357 | 0.139 | 0.077 | Binding | [GalleryEntryPreview.qml:98](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:98) |  |
| 16 | 48.08 | 357 | 0.135 | 0.075 | Binding | [GalleryEntryPreview.qml:192](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:192) |  |
| 17 | 37.05 | 454 | 0.082 | 0.058 | Binding | [GalleryEntryPreview.qml:148](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:148) |  |
| 18 | 36.12 | 227 | 0.159 | 0.056 | Binding | [GalleryEntryPreview.qml:190](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:190) |  |
| 19 | 34.17 | 825 | 0.041 | 0.053 | Binding | [GalleryEntryPreview.qml:107](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:107) |  |
| 20 | 34.00 | 119 | 0.286 | 0.053 | Binding | [GalleryEntryDelegateBase.qml:51](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:51) |  |
| 21 | 29.92 | 641 | 0.047 | 0.047 | Binding | [GalleryPanelChrome.qml:105](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelChrome.qml:105) |  |
| 22 | 29.19 | 280 | 0.104 | 0.046 | Binding | [GalleryEntryPreview.qml:151](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:151) |  |
| 23 | 25.72 | 276 | 0.093 | 0.040 | Binding | [GalleryEntryPreview.qml:186](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:186) |  |
| 24 | 19.73 | 270 | 0.073 | 0.031 | Binding | [GalleryEntryDelegateBase.qml:150](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:150) |  |
| 25 | 17.35 | 234 | 0.074 | 0.027 | Javascript | [GalleryEntryPreview.qml:251](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:251) | commitComponent |
| 26 | 16.85 | 78 | 0.216 | 0.026 | Binding | [GalleryEntryPreview.qml:256](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:256) |  |
| 27 | 14.72 | 641 | 0.023 | 0.023 | HandlingSignal | [GalleryViewport.qml:349](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryViewport.qml:349) |  |
| 28 | 14.26 | 510 | 0.028 | 0.022 | Binding | [GalleryEntryPreview.qml:281](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:281) |  |
| 29 | 14.05 | 641 | 0.022 | 0.022 | Javascript | [GalleryViewport.qml:349](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryViewport.qml:349) | expression for onContentYChanged |
| 30 | 11.13 | 540 | 0.021 | 0.017 | Binding | [GalleryEntryDelegateBase.qml:30](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:30) |  |

## Source context for the five full-trace hotspots

### [GalleryEntryDelegateBase.qml:305](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:305)

Activation becomes observable before the deferred component exchange finishes. This can send replacement media to the outgoing renderer. Publish a coherent renderer/source pair and clear type-incompatible sources immediately.

```qml
    GalleryEntryPreview {
        id: sharedPreview
        entry: entry
        shaderThumbnail: entry.masonryMode || entry.gridMode
        activePresentation: entry.viewIndex >= 0 && entry.visualGeometryReady && !entry.visualIdentityChanging
            && (!entry.folderPreviewRequested
            || entry.visualModel.folderPreviewState !== 2)
```

### [GalleryEntryDelegateBase.qml:185](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:185)

A changed preview rectangle propagates through image dimensions, icon geometry and loaders. Its large inclusive total contains dependent image work; this expression alone is not the whole cost. Commit geometry once before final preview activation.

```qml
    readonly property rect effectivePreviewRect: {
        if (detailsMode || columnsMode) {
            // Keep the slot tied to the scaled icon, not unused row height:
            // both the preview centering and filename origin use this extent.
            const extent = Math.max(0, Math.round((height
                - 2 * panelRoot.metrics.detailsIconVerticalPadding
                + panelRoot.detailsIconSlotSize - panelRoot.detailsIconSize)
                * renderDpr) / renderDpr)
            const origin = detailsMode ? Qt.point(0, 0) : iconSceneOrigin
            const left = panelRoot.detailsRowInset
            const top = Math.max(0, (height - extent) / 2)
            return Qt.rect(Math.round((origin.x + left) * renderDpr) / renderDpr - origin.x,
                           Math.round((origin.y + top) * renderDpr) / renderDpr - origin.y,
                           extent, extent)
        }
        const rect = previewRect
        if (rect && rect.width > 0 && rect.height > 0)
            return Qt.rect(cellHorizontalInset, rect.y, cellContentWidth,
                           Math.max(1 / renderDpr, rect.height - masonryCaptionReserve))
        return Qt.rect(cellHorizontalInset, cellVerticalInset,
                       cellContentWidth,
```

### [GalleryEntryPreview.qml:112](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:112)

The icon extent depends on the preview geometry; changing it can issue image/tint updates. Suppress inactive fallback work and avoid transitional extents during recycled-row binding.

```qml
        readonly property real nominalIconSize:
            preview.entry.detailsMode || preview.entry.columnsMode
            ? Math.max(0, Math.min(parent.width,
                       lucideSource && preview.entry.panelRoot.density <= 30
                           ? preview.entry.panelRoot.detailsIconSize : Infinity,
                       preview.entry.height
                       - 2 * preview.entry.panelRoot.metrics.detailsIconVerticalPadding))
            : preview.entry.iconsMode || preview.entry.gridMode
            ? Math.max(0, Math.min(parent.width, parent.height))
            : Math.max(0, Math.min(parent.width, parent.height) * 0.55)
        readonly property bool fullAreaIcon: preview.entry.iconsMode || preview.entry.gridMode
        width: fullAreaIcon ? parent.width
                                      : preview.entry.snapIconExtent(nominalIconSize)
        height: fullAreaIcon ? parent.height : width
        readonly property point pixelGridOffset:
```

### [GalleryEntryDelegateBase.qml:76](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:76)

The binding executes very frequently. Its early return is already active during scrolling, so the next opportunity is removing unnecessary invalidations rather than adding rounding or another ancestor walk.

```qml
    readonly property bool iconPixelAlignmentActive:
        visible && !geometryAnimationRunning
        && !panelRoot.cursorPixelAlignmentSuspended
    readonly property point iconSceneOrigin: {
        if (!iconPixelAlignmentActive || detailsMode)
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
            // row to every ancestor's size repeats the whole chain on resize.
            if (ancestor.scale !== 1 || ancestor.rotation !== 0)
                dependency += ancestor.width + ancestor.height + ancestor.transformOrigin
            ancestor = ancestor.parent
        }
        return entry.mapToItem(null, dependency * 0, dependency * 0)
    }
    readonly property real iconPixelAlignmentRevision: {
        if (!iconPixelAlignmentActive)
```

### [GalleryEntryPreview.qml:213](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:213)

The pixel-snapped thumbnail width is itself small arithmetic, but geometry publication also invalidates the thumbnail path. Keep one stable geometry publication and enforce renderer/source compatibility.

```qml
        width: Math.round(parent.width * preview.entry.renderDpr)
               / preview.entry.renderDpr
        height: Math.round(parent.height * preview.entry.renderDpr)
                / preview.entry.renderDpr
        property url source:
            preview.activePresentation
                && preview.entry.panelRoot.controller.thumbnailsEnabled
                && (!preview.entry.masonryMode || preview.entry.masonryGeometryReady)
                ? (preview.isVector ? preview.entry.visualModel.vectorSource : preview.entry.imageIdUrl) : ""
        visible: thumbnail.source.toString() !== ""
                 && !(preview.entry.panelRoot.viewerTransitionActive
                      && preview.entry.panelRoot.viewerTransitionEntryId
                         === preview.entry.entryId)
    }

    Loader {
        id: previewContent
        property bool frameCreated: false
```

## Memory

The capture shows allocation churn, not enough evidence to diagnose a leak. It records 134,577 JS allocations totaling 20.32 MiB; 44.3% of the captured small-object bytes were reclaimed. Peak recorded GC heap pages were 15.44 MiB, and small-object balance at trace end was 11.33 MiB. This starts after application startup; balances describe the capture, not total process memory.

| Category | Allocations | Total allocated | Reclaimed | Peak live | Live at trace end |
|---|---:|---:|---:|---:|---:|
| GC heap pages | 2 | 15.44 MiB | 0.00 MiB | 15.44 MiB | 15.44 MiB |
| Small JS objects | 134,577 | 20.32 MiB | 8.99 MiB | 15.35 MiB | 11.33 MiB |

GC heap pages are allocator backing storage; small JS objects are individual QML/JavaScript allocations inside those pages. These categories are not additive process-memory totals. Large JS object events were zero.

## Pixmap cache

Full trace: 330 load requests, 230 loaded, 436 removed. These are Qt pixmap lifecycle events, distinct from the library thumbnail-store cache hits. The full per-load inventory, including exact revision URLs, is in [masonry-phases-pixmaps.csv](D:/Code/f4-zoin/artifacts/modes-profile/masonry-phases-pixmaps.csv). The 1792×1792 SVG rasters are unnecessary and much larger than their panel tiles.

| Loaded pixmap (query omitted) | Width×height | Pixels | Load events |
|---|---:|---:|---:|
| `file:///C:/Users/xs/Downloads/FileIcons_Dark+_nb_pro_1 (1).svg` | 1792×1792 | 3,211,264 | 1 |
| `file:///C:/Users/xs/Downloads/FileIcons_Dark+_nb_pro_1 (2).svg` | 1792×1792 | 3,211,264 | 1 |
| `file:///C:/Users/xs/Downloads/FileIcons_Dark+_nb_pro_1.svg` | 1792×1792 | 3,211,264 | 1 |
| `file:///C:/Users/xs/Downloads/FileIcons_Dark+.svg` | 1792×1792 | 3,211,264 | 1 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-fb704ebd5f003655-v1789051989722368100:799344-s799344-604x338-176` | 604×338 | 204,152 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-358422fb44357a4f-v1789051992780984400:795683-s795683-604x338-177` | 604×338 | 204,152 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-695f9c6c65e46a7c-v1789052003684039800:785999-s785999-604x338-178` | 604×338 | 204,152 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-acafa8327f8b8b33-v1789051982300703000:674207-s674207-607x334-180` | 607×334 | 202,738 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-1a365f3a7cf06a6d-v1789051982469777600:687802-s687802-607x334-181` | 607×334 | 202,738 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-dd6d7289823242b0-v1789051983202534600:690087-s690087-603x332-182` | 603×332 | 200,196 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-893713db778ff93d-v1789052183361837400:679716-s679716-603x332-183` | 603×332 | 200,196 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-e4f4b0f8b1590147-v1789052181430938300:533790-s533790-603x332-184` | 603×332 | 200,196 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-1246daafdad65e23-v1789052005136169600:777537-s777537-597x334-179` | 597×334 | 199,398 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-cb4d2ea17147aa57-v1786564674664732600:6444277-s6444277-516x288-162` | 516×288 | 148,608 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-908282ec06bbbc56-v1786564675195132200:6333629-s6333629-516x288-163` | 516×288 | 148,608 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-d069df292a6980c7-v1789051907680071200:456828-s456828-508x280-146` | 508×280 | 142,240 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-8c7b6dc8a7f8dc51-v1789051990733052400:810982-s810982-504x282-174` | 504×282 | 142,128 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-216a4ab0cdf1f546-v1789051989726868500:809398-s809398-504x282-175` | 504×282 | 142,128 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-2ef176d5acb84b0b-v1789051904603847600:1958221-s1958221-503x282-173` | 503×282 | 141,846 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-984c2e22aa8a029e-v1785516985488341600:745856157-s745856157-492x277-153` | 492×277 | 136,284 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-3b57924670087300-v1788455513456249400:8499105-s8499105-460x295-144` | 460×295 | 135,700 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-d5028b27c7baa217-v1785516969840189400:395230721-s395230721-450x277-154` | 450×277 | 124,650 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-e149e1f8ee7d06f6-v1789052181495278500:656823-s656823-468x257-185` | 468×257 | 120,276 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-23006e6f88ba9bac-v1786565933722006200:9302945-s9302945-382x288-160` | 382×288 | 110,016 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-13141bfcdc7cb9cc-v1786564757558397900:7126336-s7126336-380x287-161` | 380×287 | 109,060 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-cd58567a32ff9556-v1786565933722006200:44882-s44882-380x287-164` | 380×287 | 109,060 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-1d1f22cbf49b2990-v1784046400707440400:13195558-s13195558-350x307-126` | 350×307 | 107,450 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-18a789a6abf5a13c-v1784046394632663500:13259276-s13259276-350x307-127` | 350×307 | 107,450 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-35226e09609937e8-v1784046391174270000:13537769-s13537769-350x307-128` | 350×307 | 107,450 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-48c36c9a2d417772-v1785448486978980300:30790-s30790-294x294-157` | 294×294 | 86,436 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-221d25a0ceeabe30-v1785516399377774300:48469-s48469-294x294-186` | 294×294 | 86,436 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-b95a9b4dde03b012-v1784122757378878600:12976-s12976-294x294-188` | 294×294 | 86,436 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-34219f55058c6590-v1784046536858183200:22207648-s22207648-293x258-133` | 293×258 | 75,594 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-6d80472d596eeb3e-v1784046543430051200:23449165-s23449165-293x258-134` | 293×258 | 75,594 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-30b07a601e68f26c-v1784046541813037100:22073270-s22073270-293x258-135` | 293×258 | 75,594 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-fe8d674a8b0f948c-v1784046542164321000:21569251-s21569251-293x258-136` | 293×258 | 75,594 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-f58a217345aaa026-v1784046539506353800:22088051-s22088051-293x258-137` | 293×258 | 75,594 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-d518f2ebfd7d9858-v1784046250584785800:7924155-s7924155-293x257-132` | 293×257 | 75,301 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-ad51260f0603c0d4-v1784046261064991000:8450668-s8450668-293x257-129` | 293×257 | 75,301 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-751dc0fd23ed32e2-v1784046256476180400:8008060-s8008060-293x257-130` | 293×257 | 75,301 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-3524d55d2c72140e-v1784046257271230000:8896730-s8896730-293x257-131` | 293×257 | 75,301 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-ab72794de88f22e1-v1784046460751070200:3345953-s3345953-293x257-138` | 293×257 | 75,301 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-ed79727fb57ff5c1-v1784046251626275600:8028027-s8028027-293x257-139` | 293×257 | 75,301 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-765c79d8dee7580d-v1784046255973426400:7608901-s7608901-293x257-140` | 293×257 | 75,301 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-2381e37cf085a5a6-v1785369435180652900:130823-s130823-258x287-158` | 258×287 | 74,046 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-f9d5c9a92566ea9c-v1785369437748362200:117235-s117235-252x288-159` | 252×288 | 72,576 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-e483c29faf3b6db2-v1789572330743896400:1325988-s1325988-266x266-156` | 266×266 | 70,756 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-35917994ed6a0a0c-v1784045356275755500:98572-s98572-280x246-141` | 280×246 | 68,880 | 1 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-c19916a1a2d652a8-v1784122849445675900:107370-s107370-256x256-187` | 256×256 | 65,536 | 1 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-e1a6dd28bb556873-v1791816474589389500:314015-s314015-208x295-143` | 208×295 | 61,360 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-9b5eda74cf70a4-v1784044979481490400:548930-s548930-172x307-122` | 172×307 | 52,804 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-6723138049a8edd3-v1784044977380512800:485804-s485804-172x307-123` | 172×307 | 52,804 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-8ae9bcd8a0b5795a-v1784044979769918100:616496-s616496-172x307-124` | 172×307 | 52,804 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-37e3f23d014420c9-v1784044904085362100:1825195-s1825195-172x307-125` | 172×307 | 52,804 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-fb354f88a3abca80-v1784067002024621500:1640853-s1640853-165x294-142` | 165×294 | 48,510 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-60c9db7c62b48f6-v1784110708116041000:38184619-s38184619-134x293-171` | 134×293 | 39,262 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-3af9100ccc7cbe7e-v1783677820299848100:26477999-s26477999-134x293-170` | 134×293 | 39,262 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-255394c391babf03-v1784739102385224800:1644880-s1644880-148x264-148` | 148×264 | 39,072 | 2 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-a51a2b952948c627-v1786743380975636500:8561108-s8561108-148x264-155` | 148×264 | 39,072 | 2 |
| `qrc:/F4QtHost/icons/zoin/image.svg` | 186×200 | 37,200 | 1 |
| `qrc:/F4QtHost/icons/zoin/image.svg` | 184×198 | 36,432 | 1 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-b7a13496e35f7d46-v1784233917055999900:2295421-s2295421-128x279-145` | 128×279 | 35,712 | 2 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 186×186 | 34,596 | 1 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 184×184 | 33,856 | 1 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-1d997d947ccc91fe-v1791813179678280300:288267-s288267-124x270-147` | 124×270 | 33,480 | 2 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 168×168 | 28,224 | 2 |
| `qrc:/F4QtHost/icons/zoin/image.svg` | 161×173 | 27,853 | 1 |
| `qrc:/F4QtHost/icons/zoin/image.svg` | 159×171 | 27,189 | 1 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 163×163 | 26,569 | 2 |
| `qrc:/F4QtHost/icons/zoin/text.svg` | 145×181 | 26,245 | 1 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 161×161 | 25,921 | 6 |
| `qrc:/F4QtHost/icons/zoin/svg.svg` | 161×161 | 25,921 | 1 |
| `qrc:/F4QtHost/icons/zoin/default.svg` | 144×180 | 25,920 | 1 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 159×159 | 25,281 | 3 |
| `qrc:/F4QtHost/icons/zoin/default.svg` | 142×178 | 25,276 | 1 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 156×156 | 24,336 | 3 |
| `qrc:/F4QtHost/icons/zoin/default.svg` | 138×173 | 23,874 | 1 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 154×154 | 23,716 | 3 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 152×152 | 23,104 | 3 |
| `qrc:/F4QtHost/icons/zoin/default.svg` | 135×169 | 22,815 | 1 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 149×149 | 22,201 | 4 |
| `qrc:/F4QtHost/icons/zoin/image.svg` | 142×153 | 21,726 | 3 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 147×147 | 21,609 | 4 |
| `qrc:/F4QtHost/icons/zoin/image.svg` | 140×151 | 21,140 | 1 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 145×145 | 21,025 | 3 |
| `qrc:/F4QtHost/icons/zoin/html.svg` | 144×144 | 20,736 | 1 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 144×144 | 20,736 | 4 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 142×142 | 20,164 | 4 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 140×140 | 19,600 | 3 |
| `qrc:/F4QtHost/icons/zoin/default.svg` | 124×155 | 19,220 | 1 |
| `qrc:/F4QtHost/icons/zoin/table.svg` | 124×155 | 19,220 | 1 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 138×138 | 19,044 | 2 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 137×137 | 18,769 | 4 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 135×135 | 18,225 | 3 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 133×133 | 17,689 | 2 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 131×131 | 17,161 | 3 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 130×130 | 16,900 | 4 |
| `qrc:/F4QtHost/icons/zoin/zip.svg` | 145×116 | 16,820 | 1 |
| `qrc:/F4QtHost/icons/zoin/zip.svg` | 144×115 | 16,560 | 2 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 124×124 | 15,376 | 2 |
| `qrc:/F4QtHost/icons/zoin/zip.svg` | 137×110 | 15,070 | 1 |
| `qrc:/F4QtHost/icons/zoin/zip.svg` | 131×105 | 13,755 | 1 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 114×114 | 12,996 | 2 |
| `file:///C:/Users/xs/Downloads/FileIcons_Dark+64.svg` | 112×112 | 12,544 | 1 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 107×107 | 11,449 | 1 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 105×105 | 11,025 | 1 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 95×95 | 9,025 | 2 |
| `qrc:/F4QtHost/icons/zoin/image.svg` | 91×98 | 8,918 | 1 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 91×91 | 8,281 | 2 |
| `qrc:/F4QtHost/icons/zoin/image.svg` | 82×88 | 7,216 | 1 |
| `qrc:/F4QtHost/icons/zoin/default.svg` | 75×94 | 7,050 | 1 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 82×82 | 6,724 | 2 |
| `image://f4icons/lucide/cGxheS1maWxsZWQ` | 79×79 | 6,241 | 1 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 75×75 | 5,625 | 2 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 74×74 | 5,476 | 2 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 72×72 | 5,184 | 2 |
| `qrc:/F4QtHost/icons/zoin/image.svg` | 68×73 | 4,964 | 1 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 68×68 | 4,624 | 2 |
| `file:///C:/Users/xs/Downloads/menu.svg` | 49×49 | 2,401 | 1 |
| `file:///C:/Users/xs/Downloads/Microsoft-Windows-Logo-2--Streamline-Logos.svg` | 42×42 | 1,764 | 1 |

## Next validation

Fix renderer/source isolation first, then repeat the same warm Masonry workload without recording. Verify photo→SVG, SVG→photo, icon→SVG and row release/reuse paths; assert zero SVG raster pixmap loads while allowing the VectorImage scene. After that, address row commit/release bursts and profile render batching/upload with native symbols or GPU timings. Keep DPR 1.75 scene-space leaf alignment and rendered sharpness checks for any eventual UI change. The concentrated QML files for a structural review are GalleryEntryPreview.qml, GalleryEntryDelegateBase.qml and GalleryThumbnailShaderLayer.qml.

> AI assistance has been used to create this output.
