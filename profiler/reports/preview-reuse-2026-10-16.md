# Preview and delegate reuse

Warm Columns scrolling now constructs no QML objects. Large Columns pauses are smaller, but the 160 Hz pacing target remains unmet and p95 did not improve.

## Implementation

- Compatible fallback icon and bitmap preview objects survive identity changes. Component selection is coalesced until the row snapshot has settled, avoiding temporary empty-source component swaps.
- Outgoing pixels are cleared before reassignment. Bitmap retention is allowed only for the same entry; regression captures verify that a missing replacement cannot show the preceding blue image.
- Anonymous reusable spares are bounded by the live viewport, capped at 64, and filtered by delegate component. They release row facades and sources.
- Columns prepares at most two nearby rows per timer tick with a 2 ms preparation budget. A single atomic row operation cannot be preempted. Preparation uses the existing metadata look-ahead and re-evaluates the current range after navigation.
- Columns keeps up to 64 recent clipped rows for reverse scrolling, with temporary retirement debt capped at 128. Excess rows retire incrementally. Prepared/recent offscreen rows are hidden from rendering. Mode changes retain no old presentation cache.

## Native pacing: matched Columns workload

Downloads: 183 entries, left panel; static right photo pane. 160 Hz, DPR 1.75, identical column density and scroll extent. Four-second downward sweeps only, native frame-swap instrumentation, QML recording off. Existing disk/image caches retained. The first exploratory baseline overlapped compilation and is excluded; the clean baseline below had no concurrent builds/tests. These are submission/frame-swap intervals, not GPU execution or scanout timestamps.

| Run | p95 ms | p99 ms | Worst ms | Intervals >=9.375 ms |
|---|---:|---:|---:|---:|
| Clean baseline | 6.83 | 17.57 | 67.81 | 1.3% |
| Final repeated sweep | 8.18 | 18.56 | 26.93 | 4.6% |

The change reduces severe pauses; it is not a uniform frame-time improvement. p95 and p99 remain above 6.25 ms, and p95 is higher than the clean baseline. Do not describe this as 160 Hz smooth scrolling. More frequent small overruns remain a tradeoff that further native rendering work must address.

## Five-mode measurements after the change

| Mode | Density | p95 ms | p99 ms | Worst ms |
|---|---:|---:|---:|---:|
| Details | 23.00 | 13.49 | 15.87 | 18.32 |
| Columns | 22.29 | 8.18 | 18.56 | 26.93 |
| Grid | 112.00 | 14.55 | 24.68 | 30.44 |
| Icons | 64.00 | 10.72 | 19.91 | 37.76 |
| Masonry | 152.00 | 15.19 | 23.06 | 64.66 |

Modes use their saved densities; the table is not a comparison at equal item sizes. Masonry still has significant stalls (including a 203 ms interval in the initial pass); repeated traversal also remains uneven. Source/model and rendering work in those stalls needs separate investigation. Some upward animations were interrupted by viewport restoration, so all headline measurements consistently exclude upward phases.

## Verification

- 25 layout/pool tests, 9 selected session tests, 8 selected panel/physical-pixel tests and 3 import-isolation test cases pass (counts include fixture setup/cleanup).
- New coverage checks compatible icon/bitmap object identity, clearing stale rendered pixels, bounded spare/component reuse, preparation outside the visible range, reuse when that row enters view, and bounded retention after a distant jump.
- All five modes retain resting scene-space pixel-grid and unit-transform coverage at DPR 1.75. Native app captures were inspected for filenames, secondary extensions, sizes and small raster icons. The software file-field fixture has missing-font rendering warnings; its geometry assertions are useful, but its blank text capture is not sharpness evidence.
- Windows system-DLL audit and embedded-only QML imports pass. Host payload regenerated and Go launcher rebuilt with CGO_ENABLED=0 and the embedded Qt tag.

## Final warm Columns QML trace

The final complete warm capture has zero Creating events of any type. The earlier isolated baseline downward trace created 280 IconImage leaves and 108 BrickItem delegates. The final capture begins after priming Columns, with no mode change inside the recording; native pacing above is from a separate recording-off sweep.

Full trace: [churn-rest-columns.qtd](D:/Code/f4-zoin/artifacts/modes-profile/churn-rest-columns.qtd). Parser active-time estimate: 4000.00 ms, not wall-clock duration. Inclusive/nested totals overlap and are not additive frame costs.

| Type | Count | Inclusive ms | ms/animation frame |
|---|---:|---:|---:|
| Binding | 122309 | 349.82 | 0.547 |
| Javascript | 130143 | 313.58 | 0.490 |
| HandlingSignal | 2583 | 111.56 | 0.174 |

## Memory

Short-lived JS allocation churn remains; this trace does not establish a leak. 246,244 object allocations, 37.45 MiB allocated, 59.5% reclaimed, 15.18 MiB live at capture end. Peak GC heap pages: 19.27 MiB.

| Category | Allocations | Total allocated | Reclaimed | Peak live | Live at exit |
|---|---:|---:|---:|---:|---:|
| GC heap pages | 5 | 19.40 MiB | 0.25 MiB | 19.27 MiB | 19.15 MiB |
| Small JS objects | 246244 | 37.45 MiB | 22.27 MiB | 19.18 MiB | 15.18 MiB |

GC heap pages back the QV4 allocator; small/large object counters track allocations within its object accounting. Do not add these categories together. Zero categories omitted.

## Pixmap cache

Requests: 0; loaded: 0; removed: 0. Includes both panes and mode setup. These raster counters do not represent vector-scene geometry. Loaded dimensions below do not reveal an obvious new oversized SVG bitmap; SVGs use no raster fallback.

| Source | Dimensions | Pixels |
|---|---:|---:|

## Top 30 inclusive hotspots

| # | ms | Count | Average ms | ms/frame | Type | Source | Detail |
|---|---:|---:|---:|---:|---|---|---|
| 1 | 240.99 | 117303 | 0.002 | 0.377 | Binding | [GalleryEntryDelegateBase.qml:76](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:76) |  |
| 2 | 137.32 | 117303 | 0.001 | 0.215 | Javascript | [GalleryEntryDelegateBase.qml:76](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:76) | expression for iconSceneOrigin |
| 3 | 82.63 | 640 | 0.129 | 0.129 | HandlingSignal | [GalleryPanelChrome.qml:72](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelChrome.qml:72) |  |
| 4 | 81.95 | 640 | 0.128 | 0.128 | Javascript | [GalleryPanelChrome.qml:72](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelChrome.qml:72) | onContentYChanged |
| 5 | 28.68 | 640 | 0.045 | 0.045 | Binding | [GalleryPanelChrome.qml:105](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelChrome.qml:105) |  |
| 6 | 26.92 | 640 | 0.042 | 0.042 | HandlingSignal | [GalleryViewport.qml:349](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryViewport.qml:349) |  |
| 7 | 26.11 | 640 | 0.041 | 0.041 | Javascript | [GalleryViewport.qml:349](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryViewport.qml:349) | expression for onContentYChanged |
| 8 | 14.32 | 366 | 0.039 | 0.022 | Binding | [GalleryEntryDelegateBase.qml:73](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:73) |  |
| 9 | 14.19 | 2 | 7.094 | 0.022 | Binding | [GalleryCursorController.qml:90](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryCursorController.qml:90) |  |
| 10 | 14.18 | 2 | 7.090 | 0.022 | Binding | [GalleryPanel.qml:327](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanel.qml:327) |  |
| 11 | 12.98 | 645 | 0.020 | 0.020 | Javascript | [GalleryPanelFacade.qml:14](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelFacade.qml:14) | refreshHoveredIndex |
| 12 | 11.72 | 645 | 0.018 | 0.018 | Javascript | [GallerySelectionController.qml:62](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GallerySelectionController.qml:62) | refreshHoveredIndex |
| 13 | 10.35 | 640 | 0.016 | 0.016 | Binding | [GalleryCursorController.qml:88](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryCursorController.qml:88) |  |
| 14 | 9.53 | 641 | 0.015 | 0.015 | Javascript | [GalleryPanelFacade.qml:175](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelFacade.qml:175) | updateVisualCursorForViewport |
| 15 | 8.44 | 641 | 0.013 | 0.013 | Javascript | [GalleryCursorController.qml:176](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryCursorController.qml:176) | updateVisualCursorForViewport |
| 16 | 8.07 | 640 | 0.013 | 0.013 | Binding | [GalleryGroupHeaders.qml:12](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryGroupHeaders.qml:12) |  |
| 17 | 5.15 | 183 | 0.028 | 0.008 | Binding | [GalleryEntryDelegateBase.qml:185](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:185) |  |
| 18 | 4.74 | 641 | 0.007 | 0.007 | Javascript | [GalleryCursorController.qml:111](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryCursorController.qml:111) | indexIntersectsViewport |
| 19 | 4.21 | 640 | 0.007 | 0.007 | Binding | [GalleryPanel.qml:325](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanel.qml:325) |  |
| 20 | 3.70 | 447 | 0.008 | 0.006 | Javascript | [ColumnsEntryDelegate.qml:17](D:/Code/f4-zoin/third_party/ZoinGallery/qml/ColumnsEntryDelegate.qml:17) | pixelOffset |
| 21 | 3.10 | 248 | 0.012 | 0.005 | Binding | [ColumnsEntryDelegate.qml:90](D:/Code/f4-zoin/third_party/ZoinGallery/qml/ColumnsEntryDelegate.qml:90) |  |
| 22 | 2.52 | 640 | 0.004 | 0.004 | Javascript | [GalleryGroupHeaders.qml:12](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryGroupHeaders.qml:12) | expression for model |
| 23 | 2.34 | 248 | 0.009 | 0.004 | Javascript | [ColumnsEntryDelegate.qml:90](D:/Code/f4-zoin/third_party/ZoinGallery/qml/ColumnsEntryDelegate.qml:90) | expression for pixelCorrection |
| 24 | 2.16 | 199 | 0.011 | 0.003 | Binding | [ColumnsEntryDelegate.qml:53](D:/Code/f4-zoin/third_party/ZoinGallery/qml/ColumnsEntryDelegate.qml:53) |  |
| 25 | 1.84 | 640 | 0.003 | 0.003 | Javascript | [GalleryCursorController.qml:88](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryCursorController.qml:88) | expression for cursorPixelGridViewportOrigin |
| 26 | 1.56 | 199 | 0.008 | 0.002 | Javascript | [ColumnsEntryDelegate.qml:53](D:/Code/f4-zoin/third_party/ZoinGallery/qml/ColumnsEntryDelegate.qml:53) | expression for pixelCorrection |
| 27 | 1.03 | 640 | 0.002 | 0.002 | Javascript | [GalleryPanelChrome.qml:105](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelChrome.qml:105) | expression for position |
| 28 | 0.98 | 219 | 0.004 | 0.002 | Javascript | [ColumnsEntryDelegate.qml:56](D:/Code/f4-zoin/third_party/ZoinGallery/qml/ColumnsEntryDelegate.qml:56) | expression for width |
| 29 | 0.87 | 640 | 0.001 | 0.001 | Javascript | [GalleryPanel.qml:325](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanel.qml:325) | expression for cursorPixelGridViewportOrigin |
| 30 | 0.77 | 183 | 0.004 | 0.001 | Javascript | [GalleryEntryDelegateBase.qml:185](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:185) | expression for effectivePreviewRect |

## Source analysis

### [GalleryEntryDelegateBase.qml:76](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:76)

```qml
        && !panelRoot.cursorPixelAlignmentSuspended
    readonly property point iconSceneOrigin: {
        if (!iconPixelAlignmentActive || detailsMode)
            return Qt.point(0, 0)
        // mapToItem() does not register dependencies on ancestor geometry.
        // Track geometry through the complete chain so host
        // panel placement invalidates the correction without any scrolling.
```

Geometry/alignment bindings still reevaluate while scrolling. Trim invalidations while keeping complete scene-space alignment at rest; delegate construction is absent in this warm trace.

### [GalleryPanelChrome.qml:72](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelChrome.qml:72)

```qml

            function onContentYChanged() {
                const extent = chromeRoot.layout.contentHeight
                verticalScroll.position = extent > 0
                        ? chromeRoot.layout.contentY / extent : 0
            }
            function onContentHeightChanged() { verticalScroll.updateSize() }
```

Scrollbar notifications continue on the scroll path. Gate invisible-axis computations and consolidate forwarding after the larger rendering/delegate costs are addressed.

### [GalleryViewport.qml:349](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryViewport.qml:349)

```qml
        }
        onContentYChanged: {
            viewportRoot.panelRoot.updateVisualCursorForViewport()
            viewportRoot.panelRoot.refreshHoveredIndex()
            if (viewportRoot.panelRoot.pathViewportPlacementPending) {
                viewportRoot.panelRoot.traceBenchmarkStage(
                            "layout.content-y.changed", {})
```

This handler/binding remains active during scrolling. Consolidate notifications and inspect native rendering separately; the final trace contains no QML object construction.

### [GalleryCursorController.qml:90](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryCursorController.qml:90)

```qml
        galleryLayout.viewportSceneOrigin
    readonly property bool cursorPixelAlignmentSuspended:
        cursorChromeTransitionActive || panelScrollAnimation.running

    function alignViewportItemRectToDevicePixels(item, rect) {
        if (!item || !rect)
            return rect
```

This handler/binding remains active during scrolling. Consolidate notifications and inspect native rendering separately; the final trace contains no QML object construction.

### [GalleryPanel.qml:327](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanel.qml:327)

```qml
        cursorState.cursorPixelGridViewportOrigin
    readonly property bool cursorPixelAlignmentSuspended:
        cursorState.cursorPixelAlignmentSuspended

    GalleryCursorController {
        id: cursorState
        panel: root
```

This handler/binding remains active during scrolling. Consolidate notifications and inspect native rendering separately; the final trace contains no QML object construction.

## Next targets

Isolate native render/driver work and render-batch rebuilding, particularly in Details and at clipped column boundaries. Also investigate remaining Masonry stalls. Preserve resting pixel-grid checks. A focused qt-qml-review of GalleryEntryPreview.qml and GalleryEntryDelegateBase.qml can check the retained-object lifecycle and remaining binding notifications.

> AI assistance has been used to create this output.