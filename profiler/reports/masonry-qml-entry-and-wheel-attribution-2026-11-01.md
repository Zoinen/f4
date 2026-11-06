# Masonry QML creation and binding attribution — 2026-11-01

The recorded folder entry performs synchronous presentation commits, while wheel input retargeting overlaps delegate and thumbnail-surface creation. These are candidates for narrower native attribution; this instrumented run does not establish production frame-time costs or a successful optimization.

Profile: logic plus debug messages, Qt 6.11.1. Trace: `P:/f4-profiler-traces-f4-zoin-20261101/cold-t35/creation-bindings.qtd` (38,419,018 bytes, SHA-256 `8be0519b6a471ccf32f1218dce81ef6a214a76eed199b97581c286fb72e7799c`). Both the profiler and owned Go launcher exited 0; Qt exited normally. The trace was explicitly flushed before application shutdown.

Wall-clock duration unavailable from animation events (none captured). Native recorder independently measured 1,366.4994 ms of input plus settling. Sum of captured range-event durations: **8,352.89 ms**, including nested binding/JS/creating ranges; this is **not wall time**. Positive-duration range events: **464,526**. Counts depend on capture length and interaction. Creating counts represent profiler events, not necessarily distinct object instances.

The gate verified an initialized empty start, both panels navigating to Downloads, an empty private image cache before navigation, 160 Hz and DPR 1.75. The short one-second wheel run is attribution only: no full warm tour, middle-scroll run, PresentMon deadline assessment, or accepted physical-motion test was performed. `fullPerformanceAccepted` and `physicalMotionVerified` remain false. The normal running app was preserved. Temporary display settings were restored from 160 to the prior 60 Hz without changing registry settings.

## Event types

| Type | Events | Inclusive total ms |
|---|---:|---:|
| Javascript | 284052 | 3864.32 |
| Binding | 145116 | 1911.3 |
| HandlingSignal | 6606 | 1197.98 |
| Creating | 28553 | 1018.63 |
| Compiling | 199 | 360.67 |

Memory, pixmap-cache and animation/frame percentiles were not captured. No inference about their absence or cost is possible.

## Top captured ranges

| Rank | Inclusive ms | Count | Average ms | Type | Source | Details |
|---:|---:|---:|---:|---|---|---|
| 1 | 278.5 | 10 | 27.85 | HandlingSignal | [ShellSceneStore.qml:541](D:/Code/f4-zoin/qt/host/qml/ShellSceneStore.qml:541) |  |
| 2 | 278.49 | 10 | 27.849 | Javascript | [ShellSceneStore.qml:541](D:/Code/f4-zoin/qt/host/qml/ShellSceneStore.qml:541) | onShellChanged |
| 3 | 278.48 | 10 | 27.848 | Javascript | [ShellSceneStore.qml:405](D:/Code/f4-zoin/qt/host/qml/ShellSceneStore.qml:405) | resetShellProjection |
| 4 | 263.31 | 12 | 21.943 | Javascript | [ShellSceneStore.qml:181](D:/Code/f4-zoin/qt/host/qml/ShellSceneStore.qml:181) | captureShellSurface |
| 5 | 209.43 | 17 | 12.32 | Javascript | [GalleryPanelFacade.qml:105](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelFacade.qml:105) | endPresentationStateUpdate |
| 6 | 209.36 | 17 | 12.316 | Javascript | [GalleryViewportController.qml:496](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryViewportController.qml:496) | endPresentationStateUpdate |
| 7 | 209.04 | 18 | 11.613 | HandlingSignal | [GalleryPanelHostAdapter.qml:560](D:/Code/f4-zoin/qt/host/qml/GalleryPanelHostAdapter.qml:560) |  |
| 8 | 209.02 | 18 | 11.612 | Javascript | [GalleryPanelHostAdapter.qml:560](D:/Code/f4-zoin/qt/host/qml/GalleryPanelHostAdapter.qml:560) | onPanelPresentationTransactionFinished |
| 9 | 208.96 | 9 | 23.218 | Javascript | [GalleryPanelHostAdapter.qml:414](D:/Code/f4-zoin/qt/host/qml/GalleryPanelHostAdapter.qml:414) | endPanelPresentationTransaction |
| 10 | 190.57 | 52 | 3.665 | Javascript | [PanelsSurface.qml:46](D:/Code/f4-zoin/qt/host/qml/PanelsSurface.qml:46) | updatePanelDescriptor |
| 11 | 179.28 | 2 | 89.641 | Binding | [ShellSurfaceHost.qml:116](D:/Code/f4-zoin/qt/host/qml/ShellSurfaceHost.qml:116) |  |
| 12 | 171.95 | 2 | 85.975 | Creating | [PanelsSurface.qml:122](D:/Code/f4-zoin/qt/host/qml/PanelsSurface.qml:122) | QtQuick/Loader |
| 13 | 146.87 | 27 | 5.44 | HandlingSignal | [ShellSceneStore.qml:532](D:/Code/f4-zoin/qt/host/qml/ShellSceneStore.qml:532) |  |
| 14 | 144.86 | 27 | 5.365 | Javascript | [ShellSceneStore.qml:532](D:/Code/f4-zoin/qt/host/qml/ShellSceneStore.qml:532) | onCompactPresentationChanged |
| 15 | 144.82 | 27 | 5.364 | Javascript | [ShellSceneStore.qml:443](D:/Code/f4-zoin/qt/host/qml/ShellSceneStore.qml:443) | applyCompactPatch |
| 16 | 100.72 | 31 | 3.249 | Javascript | [RetainedGalleryPanelContent.qml:17](D:/Code/f4-zoin/qt/host/qml/RetainedGalleryPanelContent.qml:17) | activatePanel |
| 17 | 84.19 | 16 | 5.262 | Binding | [PanelPairSurface.qml:89](D:/Code/f4-zoin/qt/host/qml/PanelPairSurface.qml:89) |  |
| 18 | 83.93 | 17 | 4.937 | HandlingSignal | [PanelsSurface.qml:74](D:/Code/f4-zoin/qt/host/qml/PanelsSurface.qml:74) |  |
| 19 | 83.91 | 17 | 4.936 | Javascript | [PanelsSurface.qml:74](D:/Code/f4-zoin/qt/host/qml/PanelsSurface.qml:74) | onRightPanelPresentationOverrideChanged |
| 20 | 79.57 | 47 | 1.693 | HandlingSignal | [GalleryPointerLayer.qml:50](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPointerLayer.qml:50) |  |
| 21 | 79.44 | 47 | 1.69 | Javascript | [GalleryPointerLayer.qml:50](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPointerLayer.qml:50) |  |
| 22 | 78.82 | 47 | 1.677 | HandlingSignal | [GalleryViewport.qml:712](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryViewport.qml:712) |  |
| 23 | 78.75 | 47 | 1.675 | Javascript | [GalleryViewport.qml:712](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryViewport.qml:712) |  |
| 24 | 78.64 | 47 | 1.673 | Javascript | [GalleryPanelFacade.qml:74](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelFacade.qml:74) | handlePanelWheel |
| 25 | 78.54 | 47 | 1.671 | Javascript | [GalleryViewportController.qml:301](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryViewportController.qml:301) | handlePanelWheel |
| 26 | 77.16 | 47 | 1.642 | Javascript | [GalleryViewportController.qml:229](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryViewportController.qml:229) | scrollBy |
| 27 | 77.05 | 47 | 1.639 | Javascript | [GalleryViewportController.qml:187](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryViewportController.qml:187) | animatePanelScrollTo |
| 28 | 76.65 | 1 | 76.654 | Compiling | [main.qml:0](D:/Code/f4-zoin/qt/host/qml/main.qml:0) | qrc:/F4QtHost/qml/main.qml |
| 29 | 75.98 | 47 | 1.617 | Javascript | [GalleryPanelMotion.qml:27](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelMotion.qml:27) | restartScroll |
| 30 | 74.97 | 15 | 4.998 | Binding | [PanelPairSurface.qml:76](D:/Code/f4-zoin/qt/host/qml/PanelPairSurface.qml:76) |  |

These totals include startup and nested calls. The first four ranges describe the same shell-update chain, so their durations must not be added.

## Five leading project ranges

### [ShellSceneStore.qml:541](D:/Code/f4-zoin/qt/host/qml/ShellSceneStore.qml:541)

The shellChanged connection invokes resetShellProjection synchronously. The 10 captured handlers include startup. Investigate whether semantically unchanged shell frames are causing work before adding any deferral; preserving same-stack presentation correctness is required.

```qml

    Connections {
        target: store.surfaceRegistry

        function onShellChanged() { store.resetShellProjection() }
        function onDocumentChanged() { store.resetDocumentProjection() }
        function onOperationsQueueChanged() { store.resetOperationsQueueProjection() }
    }

    Connections {
        target: store.overlayState

        function onMenuBarChanged() { store.menuBarOverride = null }
        function onCommandMenusChanged() { store.commandMenusUpdated() }
    }
```

### [ShellSceneStore.qml:541](D:/Code/f4-zoin/qt/host/qml/ShellSceneStore.qml:541)

This JavaScript event is nested inside the preceding signal handler. It is not another 278 ms of work. Attribute expensive descendants and avoid removing necessary updates based only on this wrapper total.

```qml

    Connections {
        target: store.surfaceRegistry

        function onShellChanged() { store.resetShellProjection() }
        function onDocumentChanged() { store.resetDocumentProjection() }
        function onOperationsQueueChanged() { store.resetOperationsQueueProjection() }
    }

    Connections {
        target: store.overlayState

        function onMenuBarChanged() { store.menuBarOverride = null }
        function onCommandMenusChanged() { store.commandMenusUpdated() }
    }
```

### [ShellSceneStore.qml:405](D:/Code/f4-zoin/qt/host/qml/ShellSceneStore.qml:405)

resetShellProjection replaces retained shell state and clears panel overrides, which can invalidate dependent bindings. A useful candidate is suppressing equal projections at the owning publication boundary; prove which source values are unchanged first.

```qml
        captureRetainedSurfaces()
        sceneReset()
    }

    function resetShellProjection() {
        shellPresentationOverrideSet = false
        shellPresentationOverride = null
        captureShellSurface()
        leftPanelPresentationOverride = null
        rightPanelPresentationOverride = null
        leftPanelLayoutStateOverride = null
        rightPanelLayoutStateOverride = null
        panelActivationOverride = -1
        sceneReset()
    }
```

### [ShellSceneStore.qml:181](D:/Code/f4-zoin/qt/host/qml/ShellSceneStore.qml:181)

captureShellSurface reads currentShellFrame and assigns retainedShellFrame. Its high inclusive time can include downstream presentation and delegate work triggered by assignment. Measure changed fields and child transactions; optimizing the trivial assignment itself would miss the synchronous cascade.

```qml
        const frame = surfaceRegistry.documentMetadata
        return isDocumentSurface(frame) ? frame : null
    }

    function captureShellSurface() {
        const shell = currentShellFrame()
        if (shell !== null) {
            retainedShellFrame = shell
            retainedShellSurfaceCreated = true
        }
    }

    function captureDocumentSurface() {
        const document = currentDocumentFrame()
        documentSurfaceActive = document !== null
```

### [GalleryPanelFacade.qml:105](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelFacade.qml:105)

The facade forwards into GalleryViewportController.endPresentationStateUpdate. That function ends the native layout transaction and completes the presentation switch synchronously. Preserve its protection against exposing obsolete intermediate viewports. Next probe should separate layout commit, delegate preparation, and dependent visual binding costs, then reduce proven redundant work within the transaction.

```qml
    function beginPresentationStateUpdate(switchingMode) {
        panelFacade.viewportController.beginPresentationStateUpdate(switchingMode)
    }

    function endPresentationStateUpdate(publishVisibleRange) {
        panelFacade.viewportController.endPresentationStateUpdate(publishVisibleRange)
    }

    function restoreScrollOffset() {
        return panelFacade.viewportController.restoreScrollOffset()
    }

    function restoreScrollOrEnsureCursor() {
        panelFacade.viewportController.restoreScrollOrEnsureCursor()
    }
```

## Entry and input scopes

The Qt trace format omitted debug-message text. Scope mapping uses the ordered five QtInfo events: two startup messages followed by the three instrumented markers (navigation, input start, input done). The native navigation-to-input interval agrees with the corresponding Qt interval within **1 microsecond**. This is a sequence/delta cross-check, not a preserved message payload. Exact observed timestamps and mapping are in `artifacts/entry-qml-flush-20261101/scoped-summary.json`. Ranges crossing boundaries are clipped. Interval unions below avoid adding nested ranges, but are still observed elapsed ranges rather than CPU residency.

### Entry

Scope span 683.5432 ms; union of captured positive-duration ranges 478.5966 ms.

| Type | Source | Count | Inclusive ms | Largest clipped range ms |
|---|---|---:|---:|---:|
| Creating | [GalleryVisualHost.qml:5](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryVisualHost.qml:5) | 448 | 24.8944 | 0.5263 |
| Creating | [GalleryViewport.qml:388](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryViewport.qml:388) | 112 | 19.5733 | 0.4370 |
| Creating | [GalleryEntryDelegateBase.qml:9](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:9) | 224 | 19.3599 | 0.4341 |
| Creating | [GalleryFallbackIconHost.qml:5](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryFallbackIconHost.qml:5) | 224 | 11.6015 | 0.2394 |
| Creating | [GalleryEntryVisualFactories.qml:9](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryVisualFactories.qml:9) | 162 | 9.9644 | 0.1532 |
| Binding | [GalleryEntryDelegateBase.qml:295](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:295) | 1193 | 57.9963 | 0.5528 |
| Binding | [PanelPairSurface.qml:89](D:/Code/f4-zoin/qt/host/qml/PanelPairSurface.qml:89) | 8 | 49.5644 | 12.3026 |
| Binding | [PanelPairSurface.qml:76](D:/Code/f4-zoin/qt/host/qml/PanelPairSurface.qml:76) | 7 | 44.9152 | 10.5851 |
| Binding | [FilePanelView.qml:397](D:/Code/f4-zoin/qt/host/qml/FilePanelView.qml:397) | 15 | 39.0988 | 7.9595 |
| Binding | [PanelsSurface.qml:21](D:/Code/f4-zoin/qt/host/qml/PanelsSurface.qml:21) | 4 | 36.7975 | 12.9725 |
| Javascript | [GalleryPanelHostAdapter.qml:560](D:/Code/f4-zoin/qt/host/qml/GalleryPanelHostAdapter.qml:560) | 14 | 208.9564 | 53.4807 |
| Javascript | [GalleryPanelHostAdapter.qml:414](D:/Code/f4-zoin/qt/host/qml/GalleryPanelHostAdapter.qml:414) | 7 | 208.9336 | 53.4789 |
| Javascript | [GalleryPanelFacade.qml:105](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelFacade.qml:105) | 7 | 208.8339 | 53.4676 |
| Javascript | [GalleryViewportController.qml:496](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryViewportController.qml:496) | 7 | 208.8257 | 53.4664 |
| Javascript | [PanelsSurface.qml:46](D:/Code/f4-zoin/qt/host/qml/PanelsSurface.qml:46) | 22 | 103.2242 | 12.9641 |
### Input

Scope span 1366.4994 ms; union of captured positive-duration ranges 332.2425 ms.

| Type | Source | Count | Inclusive ms | Largest clipped range ms |
|---|---|---:|---:|---:|
| Creating | [GalleryThumbnailShaderLayer.qml:59](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryThumbnailShaderLayer.qml:59) | 184 | 27.6330 | 20.4575 |
| Creating | [GalleryVisualHost.qml:5](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryVisualHost.qml:5) | 396 | 23.1169 | 0.3974 |
| Creating | [GalleryEntryVisualFactories.qml:140](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryVisualFactories.qml:140) | 197 | 20.3158 | 7.0688 |
| Creating | [GalleryViewport.qml:388](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryViewport.qml:388) | 99 | 17.1371 | 0.3330 |
| Creating | [GalleryEntryDelegateBase.qml:9](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:9) | 198 | 16.8933 | 0.3292 |
| Binding | [GalleryEntryDelegateBase.qml:295](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:295) | 321 | 16.8681 | 1.8368 |
| Binding | [GalleryEntryDelegateBase.qml:82](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:82) | 561 | 14.7656 | 0.2616 |
| Binding | [GalleryEntryVisualFactories.qml:153](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryVisualFactories.qml:153) | 358 | 13.4278 | 0.1857 |
| Binding | [GalleryEntryDelegateBase.qml:61](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:61) | 240 | 11.1136 | 0.6531 |
| Binding | [GalleryEntryVisualFactories.qml:137](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryVisualFactories.qml:137) | 98 | 10.0817 | 0.1801 |
| Javascript | [GalleryPointerLayer.qml:50](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPointerLayer.qml:50) | 47 | 79.4391 | 8.8464 |
| Javascript | [GalleryViewport.qml:712](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryViewport.qml:712) | 47 | 78.7456 | 8.8318 |
| Javascript | [GalleryPanelFacade.qml:74](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelFacade.qml:74) | 47 | 78.6419 | 8.8297 |
| Javascript | [GalleryViewportController.qml:301](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryViewportController.qml:301) | 47 | 78.5422 | 8.8275 |
| Javascript | [GalleryViewportController.qml:229](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryViewportController.qml:229) | 47 | 77.1582 | 8.7995 |

## Findings and next checks

- During entry, seven facade presentation commits account for 208.83 ms inclusive under profiling. The activePresentation binding at GalleryEntryDelegateBase:295 evaluates 1,193 times (58.00 ms inclusive). This is consistent with downstream visual activation cascades; it does not establish which invalidations are redundant.
- During input, 47 restartScroll calls take 75.98 ms inclusive, maximum 8.77 ms. This wrapper calls native retarget(), so the trace cannot isolate its native children. Instrument target preparation and animation-state changes separately, preserving target-leading coverage.
- Input contains 184 ShaderEffect creation events at GalleryThumbnailShaderLayer:59, with 27.63 ms inclusive total and a 20.46 ms maximum. Such a rare event may include scheduler delay, GC or nested initialization. Correlate a native CPU/scheduling capture before calling it shader compilation or replacing the renderer.
- Entry creates both visual hosts and fallback hosts. Input still creates visual hosts, fallback images and folder-preview surfaces. Evaluate reusing the specific active thumbnail/fallback primitives across identity changes, bounded by existing viewport limits; do not repeat unproven larger delegate-reserve experiments.
- Preserve source rendering, native SVG support, failure handling and final 175% leaf alignment. Any candidate needs focused correctness regressions, rendered small-label checks, clean unprofiled ABBA cadence tests, then the full cold/warm/middle scope. The wheel physical-motion acceptance gap remains unresolved.
- Review the measured QML files with qt-qml-review only if broader structural analysis is needed after the native attribution; the current evidence does not justify broad refactoring.

## Capture failures retained

cold-t31/t32/t33 were rejected before server observation. t33 exited 0 with empty output: the harness was following the pre-detach Windows launcher. Windows checkAndDetach normally relaunches with F4_DETACHED=1 and discards output handles. The corrected harness sets that existing environment flag explicitly, keeping direct Qt launch and ownership checks. Disk exhaustion occurred during t31 but was not proved to cause its exit. No unsupported host-warning explanation was applied.

cold-t34 reached the done state, but the attach client reported connection loss while recording and exited nonzero; it has no accepted trace. cold-t35 uses a distinct diagnostic host with a 20-second shutdown delay, interactive flush on done, XML validation, and profiler quit after the completed trace. Product sources and the normal running payload were unchanged by these capture changes.

> AI assistance has been used to create this output.
