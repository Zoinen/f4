# QML allocation and binding diagnosis — 2026-10-19

Candidate: retained native alignment handles. Memory, creation, bindings, JavaScript and signals are captured through qmlprofiler attach; client exits 0 and the trace has a complete closing trace element.

Trace: profiler/traces/preview-allocation-cached-clusters-2026-10-19.qtd. Two sustained contentY-change clusters select scrolling activity; these are approximate activity windows, not exact QPC frame boundaries. Wall-clock duration unavailable from animation events because animation capture was disabled. Selected clusters span about 8.264 seconds in total.

Captured range durations sum to 1880.51 ms across 370596 events. Nested durations overlap; this is not wall-clock CPU time.

## Event types

| Type | Count | Summed range ms |

| --- | ---: | ---: |

| Binding | 90350 | 984.44 |

| Javascript | 150109 | 535.12 |

| Creating | 10854 | 220.05 |

| HandlingSignal | 9251 | 140.89 |

Raw counts depend on run length and interaction. No animation frame times or pixmap-cache events were captured, so no frame-rate or cache-size conclusion follows from this trace.

## Memory

Small JS allocation traffic remains substantial: 98,989 allocations totaling 7,451,200 bytes; 6,975,392 bytes are reclaimed (93.6%). The running allocation-minus-free peak inside selected windows is 4,641,888 bytes, ending at 475,808 bytes. Window filtering omits earlier allocations: these are deltas, not total live process heap. Heap-page and large-item categories have no recorded events in these windows.

## Top ranges

| Rank | Summed ms | Count | Average ms | Type | Source | Detail |

| --- | ---: | ---: | ---: | --- | --- | --- |

| 1 | 106.03 | 1104 | 0.096 | Binding | [GalleryEntryDelegateBase.qml:295](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryEntryDelegateBase.qml#L295) |  |

| 2 | 49.73 | 1890 | 0.026 | Binding | [GalleryEntryDelegateBase.qml:82](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryEntryDelegateBase.qml#L82) |  |

| 3 | 42.27 | 828 | 0.051 | Binding | [GalleryEntryPreview.qml:18](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryEntryPreview.qml#L18) |  |

| 4 | 39.82 | 985 | 0.04 | Binding | [GalleryEntryDelegateBase.qml:40](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryEntryDelegateBase.qml#L40) |  |

| 5 | 38.15 | 866 | 0.044 | Binding | [GalleryEntryPreview.qml:23](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryEntryPreview.qml#L23) |  |

| 6 | 35.5 | 758 | 0.047 | Binding | [GalleryFallbackIconHost.qml:28](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryFallbackIconHost.qml#L28) |  |

| 7 | 30.72 | 739 | 0.042 | Binding | [GalleryFallbackIconHost.qml:35](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryFallbackIconHost.qml#L35) |  |

| 8 | 29.53 | 266 | 0.111 | HandlingSignal | [GalleryPointerLayer.qml:49](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryPointerLayer.qml#L49) |  |

| 9 | 28.7 | 266 | 0.108 | Javascript | [GalleryPointerLayer.qml:49](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryPointerLayer.qml#L49) |  |

| 10 | 28.54 | 832 | 0.034 | Binding | [MasonryEntryDelegate.qml:48](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/MasonryEntryDelegate.qml#L48) |  |

| 11 | 25.66 | 1296 | 0.02 | HandlingSignal | [GalleryPanelChrome.qml:70](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryPanelChrome.qml#L70) |  |

| 12 | 25.17 | 534 | 0.047 | Binding | [GalleryEntryDelegateBase.qml:189](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryEntryDelegateBase.qml#L189) |  |

| 13 | 24.6 | 1298 | 0.019 | Javascript | [GalleryPanelChrome.qml:70](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryPanelChrome.qml#L70) | onContentYChanged |

| 14 | 24.37 | 266 | 0.092 | HandlingSignal | [GalleryViewport.qml:711](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryViewport.qml#L711) |  |

| 15 | 24.34 | 405 | 0.06 | Binding | [GalleryEntryDelegateBase.qml:61](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryEntryDelegateBase.qml#L61) |  |

| 16 | 24.0 | 266 | 0.09 | Javascript | [GalleryViewport.qml:711](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryViewport.qml#L711) |  |

| 17 | 23.69 | 949 | 0.025 | Binding | [GalleryEntryVisualFactories.qml:156](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryEntryVisualFactories.qml#L156) |  |

| 18 | 23.52 | 266 | 0.088 | Javascript | [GalleryPanelFacade.qml:74](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryPanelFacade.qml#L74) | handlePanelWheel |

| 19 | 23.0 | 266 | 0.086 | Javascript | [GalleryViewportController.qml:301](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryViewportController.qml#L301) | handlePanelWheel |

| 20 | 22.26 | 348 | 0.064 | Creating | [GalleryVisualHost.qml:5](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryVisualHost.qml#L5) | QtQuick/Loader |

| 21 | 21.28 | 350 | 0.061 | Binding | [GalleryEntryVisualFactories.qml:73](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryEntryVisualFactories.qml#L73) |  |

| 22 | 20.07 | 350 | 0.057 | Binding | [GalleryThumbnailShaderLayer.qml:31](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryThumbnailShaderLayer.qml#L31) |  |

| 23 | 17.22 | 87 | 0.198 | Creating | [GalleryViewport.qml:388](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryViewport.qml#L388) | GalleryEntryDelegateBase |

| 24 | 16.96 | 939 | 0.018 | Binding | [GalleryEntryVisualFactories.qml:180](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryEntryVisualFactories.qml#L180) |  |

| 25 | 16.94 | 174 | 0.097 | Creating | [GalleryEntryDelegateBase.qml:9](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryEntryDelegateBase.qml#L9) | ZoinGallery.Native/BrickItem |

| 26 | 16.18 | 1803 | 0.009 | Binding | [GalleryFallbackIconHost.qml:57](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryFallbackIconHost.qml#L57) |  |

| 27 | 15.93 | 1296 | 0.012 | HandlingSignal | [GalleryViewport.qml:353](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryViewport.qml#L353) |  |

| 28 | 15.46 | 1016 | 0.015 | Binding | [GalleryFallbackIconHost.qml:42](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryFallbackIconHost.qml#L42) |  |

| 29 | 14.49 | 1296 | 0.011 | Javascript | [GalleryViewport.qml:353](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryViewport.qml#L353) | expression for onContentYChanged |

| 30 | 13.99 | 448 | 0.031 | Binding | [GalleryEntryPreview.qml:127](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryEntryPreview.qml#L127) |  |

## Source analysis

### [GalleryEntryDelegateBase.qml:295](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryEntryDelegateBase.qml#L295)

```qml

    // exchanged. This avoids rebuilding image-provider and pixel-grid state
    // for every retained viewport row during a presentation transaction.
    GalleryEntryPreview {
        id: sharedPreview
        entry: entry
        shaderThumbnail: entry.masonryMode || entry.gridMode
        activePresentation: entry.viewIndex >= 0
            && entry.visualGeometryReady && !entry.visualIdentityChanging
            && (!entry.folderPreviewRequested
            || entry.visualModel.folderPreviewState !== 2)
        z: 1
    }

    GallerySceneItem {
        anchors.fill: parent

```

The active-presentation predicate propagates identity-guard, geometry and folder readiness changes into renderer and source bindings. Its inclusive duration can include downstream work and GC. Count individual reactivations during a complete row commit before changing this guard; removing it would allow old-entry pixels.

### [GalleryEntryDelegateBase.qml:82](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryEntryDelegateBase.qml#L82)

```qml

            && previewWidth > 0 && previewHeight > 0)
            return Math.max(0, previewY)
        return Math.min(
            Math.max(0, Math.floor((Math.round(height * renderDpr) - 1) / 2)) / renderDpr,
            Math.round(Math.max(0, panelRoot.metrics.cellVerticalPadding) * renderDpr) / renderDpr)
    }
    readonly property bool iconPixelAlignmentActive:
        visible && sceneVisible && !geometryAnimationRunning
    readonly property bool motionActive:
        geometryAnimationRunning || panelRoot.cursorPixelAlignmentSuspended
    // Compatibility values for embedders; local placement is now owned by the
    // cached grid scope rather than reactive scene-origin dependency walks.
    readonly property point iconSceneOrigin: Qt.point(0, 0)
    readonly property real iconPixelAlignmentRevision: 0


```

Vertical inset selection depends on presentation mode and the committed preview rectangle. Its arithmetic is not proved to dominate this inclusive range. Inspect duplicate rectangle/inset notifications and shared native layout metadata; preserve scene-space alignment rather than adding rounding.

### [GalleryEntryPreview.qml:18](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryEntryPreview.qml#L18)

```qml

    required property bool activePresentation
    readonly property alias thumbnailItem: thumbnail
    readonly property alias fallbackState: fallbackIcon
    readonly property GalleryEntryVisualFactories visualFactories: entry.panelRoot.entryVisualFactories
    readonly property bool isVector: entry.visualModel.isVector
    readonly property bool motionActive: entry.motionActive
    readonly property bool thumbnailEnabled:
        activePresentation && entry.panelRoot.controller.thumbnailsEnabled
        && (!entry.masonryMode || entry.masonryGeometryReady)
    // Recycled renderers can survive until the deferred Loader exchange. Give
    // each renderer a typed source, never the replacement's untyped URL.
    readonly property url bitmapThumbnailSource:
        thumbnailEnabled && !entry.visualModel.isVector ? entry.imageIdUrl : ""
    readonly property url vectorThumbnailSource:
        thumbnailEnabled && entry.visualModel.isVector

```

Thumbnail enablement propagates presentation and metadata-ready state into source selection. Measure state churn during retired-row reuse and ensure acquisition is requested only for the final identity and geometry.

### [GalleryEntryDelegateBase.qml:40](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryEntryDelegateBase.qml#L40)

```qml

    readonly property bool pointerHovered:
        viewIndex >= 0 && panelRoot.hoveredIndex === viewIndex
    // A sparse page hand-off may briefly invalidate the row facade while the
    // bounded replacement snapshot is being installed. Keep the already
    // supplied row paintable through the typed controller's one-row lookup;
    // this never scans or copies the catalog.
    readonly property string effectiveDisplayName:
        visualModel.valid
        ? displayName
        : (panelRoot.controllerReady
           ? panelRoot.controller.entryNameAt(sourceIndex) : "")
    readonly property bool folderPreviewRequested:
        visualGeometryReady && (masonryMode || gridMode || iconsMode) && panelRoot.controllerReady
        && panelRoot.controller.thumbnailsEnabled
        && panelRoot.controller.directoryPreviewEnabled

```

Folder-preview activation joins asynchronous loader readiness with the requested state. It can fan out into preview/geometry changes. Measure unnecessary changes separately from required folder state transitions; do not disable previews as a performance shortcut.

### [GalleryEntryPreview.qml:23](../experiments/alignment-handle-experiment-2026-10-19/sources/cached/GalleryEntryPreview.qml#L23)

```qml

    readonly property bool motionActive: entry.motionActive
    readonly property bool thumbnailEnabled:
        activePresentation && entry.panelRoot.controller.thumbnailsEnabled
        && (!entry.masonryMode || entry.masonryGeometryReady)
    // Recycled renderers can survive until the deferred Loader exchange. Give
    // each renderer a typed source, never the replacement's untyped URL.
    readonly property url bitmapThumbnailSource:
        thumbnailEnabled && !entry.visualModel.isVector ? entry.imageIdUrl : ""
    readonly property url vectorThumbnailSource:
        thumbnailEnabled && entry.visualModel.isVector
        ? entry.visualModel.vectorSource : ""
    readonly property bool thumbnailHasSource:
        thumbnail.source.toString() !== ""
    readonly property bool thumbnailReady:
        thumbnailHasSource && previewContent.item

```

Bitmap source selection changes image-provider requests when identity or enablement changes. Track real source reads and renderer exchanges; optimize duplicate publication only when the same immutable source/revision and decoded pixels are preserved.

## Next steps

Prioritize source/readiness notification churn and renderer exchange counts, then correlate their expensive calls with native GC and render scheduling. A QML range duration alone cannot distinguish arithmetic, downstream binding work and GC. The concentrated GalleryEntryDelegateBase/GalleryEntryPreview paths are suitable targets for a focused QML review.

> AI assistance has been used to create this output.
