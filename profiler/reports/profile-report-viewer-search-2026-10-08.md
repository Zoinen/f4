# Viewer repeated-search profile — 2026-10-08

Repeated search is cheap; repeatedly creating and removing the progress dialog adds measurable Qt work.

## Scope and limits

Source: `P:/[Year 2026]/[2026.05.29] Graduation Ceremony at Music School (at Philharmonic)/files.txt` (2,280 bytes; 38 matches for `mp4`). Go diagnostic runs the real search/reveal/projection code and real asynchronous search action. Qt diagnostic replays its exported document and dialog state through the production QML at 175% scale, logical window 2194×1186, using the typed-state test controller.

**These are controlled replay measurements, not a live keyboard-to-screen profile.** Qt timings include a synchronous `grabWindow()` render/readback. IPC, real key-repeat delivery, and actual production patch selection are not measured. Incremental replay updates only document/dialog stores; the full-scene control also rebuilds hidden menu delegates and must not be attributed to production without a live trace.

QTest completed successfully in every replay. qmlprofiler reported a process crash after test completion during shutdown; both exported XML traces parse successfully. Treat memory-at-exit and final teardown coverage as provisional. The running working application was kept alive; only opt-in diagnostic code was added. Profiling build cache option was restored to OFF.

## Measured stages

| Stage | Median | p95 |
|---|---:|---:|
| Go scan (380 samples) | 0.000 ms | 0.000 ms |
| Go reveal (380 samples) | 0.499 ms | 0.999 ms |
| Go projection (380 samples) | 1.000 ms | 1.508 ms |

Timer resolution is about 0.5 ms: zero readings mean below that resolution. Projection measures visible document rows, not search scanning. Async-action timings exclude scene export; exporting the progress scene allows the worker to finish concurrently, so they are not an unbiased end-to-end latency measurement.

| Incremental Qt replay phase (warm cycles, 114 samples) | Apply median | Apply p95 | Apply + render/readback median |
|---|---:|---:|---:|
| Open progress popup | 5.142 ms | 6.785 ms | 19.629 ms |
| Close popup / publish result | 1.245 ms | 1.474 ms | 11.989 ms |
| Publish result directly (control) | 1.119 ms | 2.711 ms | 15.077 ms |

The two-phase path forces dialog creation/removal for every match, even on this tiny cached file. Its measured apply work is roughly 6.4 ms per repeat; forced frame/readback work totals roughly 32 ms. This identifies an optimization target, but does not establish the exact live latency or whether real key repeats are lost while the progress dialog owns input.

## QML event summary

Inclusive nested event durations overlap; totals and per-frame sums are **not** elapsed wall time. This trace includes startup and four replay cycles.

| Event | Count | Inclusive ms | ms / animation event |
|---|---:|---:|---:|
| Binding | 191,431 | 4125.48 | 33.815 |
| Creating | 60,888 | 2797.30 | 22.929 |
| Javascript | 530,268 | 2288.29 | 18.756 |
| HandlingSignal | 5,236 | 543.01 | 4.451 |
| Compiling | 134 | 404.20 | 3.313 |

Animation metadata: p95 30.3 ms; p99 35.71 ms; 3/122 events over 33 ms. These are animation samples from the replay, not measured input latency.

## Memory

The trace shows allocation churn, with most small-object bytes reclaimed; it does not establish a leak.

592,647 JS allocations; 43.82 MiB allocated; 92.7% reclaimed. Peak live GC heap 4.51 MiB; small JS objects live at recorded exit 3.22 MiB.

| Category | Allocations | Allocated MiB | Reclaimed MiB | Peak live MiB | Live at exit MiB |
|---|---:|---:|---:|---:|---:|
| GC heap pages | 241 | 15.44 | 11.74 | 4.51 | 3.71 |
| Small JS objects | 592,647 | 43.82 | 40.60 | 4.44 | 3.22 |

GC heap pages are VM backing storage; small JS objects are individual VM allocations. Large JS objects: zero recorded.

## Pixmap cache

20 requests, 20 loaded, 20 removed. Startup icons dominate; this trace does not implicate pixmap loading in repeated search.

| Pixmap | Dimensions | Pixels |
|---|---:|---:|
| `qrc:/F4QtHost/icons/lucide/palette.svg?size=18&dpr=1.75&color=%23ff4e9bd4` | 56×56 | 3136 |
| `qrc:/F4QtHost/icons/lucide/plus.svg?size=16&dpr=1.75&color=%23ffd7e0ea` | 42×42 | 1764 |
| `qrc:/F4QtHost/icons/lucide/x.svg?size=14&dpr=1.75&color=%23ffe8edf2` | 42×42 | 1764 |
| `qrc:/F4QtHost/icons/lucide/file-text.svg?size=16&dpr=1.75&color=%23ffe8edf2` | 42×42 | 1764 |
| `qrc:/F4QtHost/icons/lucide/list-checks.svg?size=18&dpr=1.75&color=%23ffe8edf2` | 32×32 | 1024 |
| `qrc:/F4QtHost/icons/app/f4.svg` | 32×32 | 1024 |
| `qrc:/F4QtHost/icons/lucide/file.svg?size=16&dpr=1.75&color=%23ff9aa7b5` | 28×28 | 784 |
| `qrc:/F4QtHost/icons/lucide/x.svg?size=14&dpr=1.75&color=%23ffe8edf2` | 25×25 | 625 |
| `qrc:/F4QtHost/icons/lucide/save.svg?size=14&dpr=1.75&color=%23ffffffff` | 25×25 | 625 |
| `qrc:/F4QtHost/icons/lucide/clock-3.svg?size=14&dpr=1.75&color=%23ffe8edf2` | 25×25 | 625 |
| `qrc:/F4QtHost/icons/lucide/refresh-cw.svg?size=14&dpr=1.75&color=%23ffe8edf2` | 25×25 | 625 |
| `qrc:/F4QtHost/icons/lucide/rotate-ccw.svg?size=14&dpr=1.75&color=%23ffe8edf2` | 25×25 | 625 |
| `qrc:/F4QtHost/icons/lucide/search.svg?size=14&dpr=1.75&color=%23ff9aa7b5` | 25×25 | 625 |
| `qrc:/F4QtHost/icons/lucide/arrow-left.svg?size=14&dpr=1.75&color=%23ffe8edf2` | 25×25 | 625 |
| `qrc:/ZoinGallery/resources/WindowClose.svg` | 18×18 | 324 |
| `qrc:/ZoinGallery/resources/WindowMaximize.svg` | 18×18 | 324 |
| `qrc:/ZoinGallery/resources/WindowMinimize.svg` | 18×18 | 324 |
| `qrc:/ZoinGallery/resources/WindowClose.svg` | 18×18 | 324 |
| `qrc:/ZoinGallery/resources/WindowMaximize.svg` | 18×18 | 324 |
| `qrc:/ZoinGallery/resources/WindowMinimize.svg` | 18×18 | 324 |

## Top 30 hotspots

| # | Total ms | Count | Avg ms | ms / animation event | Type | Source | Details |
|---|---:|---:|---:|---:|---|---|---|
| 1 | 1033.40 | 625 | 1.653 | 8.470 | Binding | [ShellSurfaceHost.qml:357](D:/Code/f4-zoin/qt/host/qml/ShellSurfaceHost.qml:357) |  |
| 2 | 1014.11 | 625 | 1.623 | 8.312 | Binding | [OverlayHost.qml:20](D:/Code/f4-zoin/qt/host/qml/OverlayHost.qml:20) |  |
| 3 | 950.19 | 321 | 2.960 | 7.788 | Creating | [OverlayHost.qml:90](D:/Code/f4-zoin/qt/host/qml/OverlayHost.qml:90) | QtQuick/Loader |
| 4 | 334.95 | 160 | 2.093 | 2.745 | HandlingSignal | [OverlayHost.qml:117](D:/Code/f4-zoin/qt/host/qml/OverlayHost.qml:117) |  |
| 5 | 334.84 | 160 | 2.093 | 2.745 | Javascript | [OverlayHost.qml:117](D:/Code/f4-zoin/qt/host/qml/OverlayHost.qml:117) | expression for onLoaded |
| 6 | 334.69 | 160 | 2.092 | 2.743 | Javascript | [OverlayHost.qml:97](D:/Code/f4-zoin/qt/host/qml/OverlayHost.qml:97) | bindFrame |
| 7 | 333.33 | 160 | 2.083 | 2.732 | Binding | [OverlayHost.qml:99](D:/Code/f4-zoin/qt/host/qml/OverlayHost.qml:99) |  |
| 8 | 325.49 | 320 | 1.017 | 2.668 | Binding | [DialogOverlay.qml:65](D:/Code/f4-zoin/qt/host/qml/DialogOverlay.qml:65) |  |
| 9 | 182.74 | 320 | 0.571 | 1.498 | Binding | [GenericDialog.qml:546](D:/Code/f4-zoin/qt/host/qml/GenericDialog.qml:546) |  |
| 10 | 171.44 | 160 | 1.071 | 1.405 | Creating | [OverlayHost.qml:61](D:/Code/f4-zoin/qt/host/qml/OverlayHost.qml:61) | DialogOverlay |
| 11 | 170.57 | 320 | 0.533 | 1.398 | Creating | [DialogOverlay.qml:6](D:/Code/f4-zoin/qt/host/qml/DialogOverlay.qml:6) | QtQuick/Item |
| 12 | 156.75 | 160 | 0.980 | 1.285 | Creating | [DialogOverlay.qml:60](D:/Code/f4-zoin/qt/host/qml/DialogOverlay.qml:60) | GenericDialog |
| 13 | 155.88 | 320 | 0.487 | 1.278 | Creating | [GenericDialog.qml:11](D:/Code/f4-zoin/qt/host/qml/GenericDialog.qml:11) | QtQuick/Rectangle |
| 14 | 127.41 | 155 | 0.822 | 1.044 | HandlingSignal | [ShellSceneStore.qml:529](D:/Code/f4-zoin/qt/host/qml/ShellSceneStore.qml:529) |  |
| 15 | 127.20 | 155 | 0.821 | 1.043 | Javascript | [ShellSceneStore.qml:529](D:/Code/f4-zoin/qt/host/qml/ShellSceneStore.qml:529) | onDocumentChanged |
| 16 | 127.00 | 155 | 0.819 | 1.041 | Javascript | [ShellSceneStore.qml:404](D:/Code/f4-zoin/qt/host/qml/ShellSceneStore.qml:404) | resetDocumentProjection |
| 17 | 124.17 | 157 | 0.791 | 1.018 | Javascript | [ShellSceneStore.qml:180](D:/Code/f4-zoin/qt/host/qml/ShellSceneStore.qml:180) | captureDocumentSurface |
| 18 | 122.09 | 157 | 0.778 | 1.001 | Binding | [ShellSurfaceHost.qml:147](D:/Code/f4-zoin/qt/host/qml/ShellSurfaceHost.qml:147) |  |
| 19 | 115.02 | 960 | 0.120 | 0.943 | Creating | [SemanticWidgetDelegate.qml:82](D:/Code/f4-zoin/qt/host/qml/SemanticWidgetDelegate.qml:82) | QtQuick/Loader |
| 20 | 98.24 | 1 | 98.243 | 0.805 | Compiling | [main.qml:0](D:/Code/f4-zoin/qt/host/qml/main.qml:0) | qrc:/F4QtHost/qml/main.qml |
| 21 | 72.61 | 812 | 0.089 | 0.595 | Binding | [GenericDialog.qml:79](D:/Code/f4-zoin/qt/host/qml/GenericDialog.qml:79) |  |
| 22 | 67.72 | 484 | 0.140 | 0.555 | Binding | [SemanticDialogLayout.qml:16](D:/Code/f4-zoin/qt/host/qml/SemanticDialogLayout.qml:16) |  |
| 23 | 55.94 | 320 | 0.175 | 0.459 | Binding | [GenericDialog.qml:109](D:/Code/f4-zoin/qt/host/qml/GenericDialog.qml:109) |  |
| 24 | 51.47 | 320 | 0.161 | 0.422 | Creating | [SettingsDialogBody.qml:157](D:/Code/f4-zoin/qt/host/qml/SettingsDialogBody.qml:157) | QtQuick/Repeater |
| 25 | 51.09 | 326 | 0.157 | 0.419 | Creating | [F4Button.qml:8](D:/Code/f4-zoin/qt/host/qml/F4Button.qml:8) | Button |
| 26 | 50.50 | 9,991 | 0.005 | 0.414 | Binding | [DocumentRowDelegate.qml:203](D:/Code/f4-zoin/qt/host/qml/DocumentRowDelegate.qml:203) |  |
| 27 | 48.98 | 1 | 48.979 | 0.401 | Compiling | [ShellSurfaceHost.qml:0](D:/Code/f4-zoin/qt/host/qml/ShellSurfaceHost.qml:0) | qrc:/F4QtHost/qml/ShellSurfaceHost.qml |
| 28 | 48.50 | 968 | 0.050 | 0.398 | Binding | [GenericDialog.qml:99](D:/Code/f4-zoin/qt/host/qml/GenericDialog.qml:99) |  |
| 29 | 45.51 | 320 | 0.142 | 0.373 | Binding | [GenericDialog.qml:111](D:/Code/f4-zoin/qt/host/qml/GenericDialog.qml:111) |  |
| 30 | 43.58 | 320 | 0.136 | 0.357 | Creating | [SettingsDialogBody.qml:9](D:/Code/f4-zoin/qt/host/qml/SettingsDialogBody.qml:9) | QtQuick/Item |

## Detailed analysis

### [ShellSurfaceHost.qml:357](D:/Code/f4-zoin/qt/host/qml/ShellSurfaceHost.qml:357)

```qml
        // old frame on the next GUI presentation change.
        frames: {
            const sceneStore = surfaces.hostWindow.sceneStoreApi
            // Keep the payload properties as direct dependencies as well.
            // A legacy peer can deliver a cross-stream overlay payload at the
```

Both surface and overlay bindings rebuild frame projections when dialog state changes. Cache the bounded overlay projection by its dialog/menu revisions and avoid invalidating it for unrelated document changes.

### [OverlayHost.qml:20](D:/Code/f4-zoin/qt/host/qml/OverlayHost.qml:20)

```qml
        objectName: "semanticOverlayFrameModel"
        frames: overlayHost.frames
    }

    function frameKey(frame) {
```

The overlay frame list changes twice per match: create and remove. For quick searches, defer showing progress until a modest delay has elapsed; cancel the pending display when the search finishes first.

### [OverlayHost.qml:90](D:/Code/f4-zoin/qt/host/qml/OverlayHost.qml:90)

```qml

        delegate: Loader {
            id: overlayLoader
            required property int index
            required property var modelFrame
```

The overlay Loader instantiates a fresh dialog for each progress state. Avoid instantiation on the fast path; reuse a retained progress component only if slow-search measurements justify it.

### [OverlayHost.qml:117](D:/Code/f4-zoin/qt/host/qml/OverlayHost.qml:117)

```qml
                             : dialogOverlayComponent
            onLoaded: bindFrame()
            z: 100 + index

            Connections {
```

onLoaded calls bindFrame and installs bindings for each new dialog. Reducing dialog churn removes this work; ensure binding setup occurs once per retained component.

### [OverlayHost.qml:117](D:/Code/f4-zoin/qt/host/qml/OverlayHost.qml:117)

```qml
                             : dialogOverlayComponent
            onLoaded: bindFrame()
            z: 100 + index

            Connections {
```

The JavaScript onLoaded range overlaps the preceding signal-handler range. It represents the same call chain, not an additional independent cost. Do not sum it twice.

## Next steps

1. Delay/cancel progress display for quick searches while keeping cancellation/progress for slow searches. Profile viewer and editor together because they share this helper.
2. Measure a live key-repeat trace through input dispatch, dialog ownership, IPC, typed-store commit, and render presentation. Check whether repeats arriving during the modal progress phase are consumed instead of starting the next search.
3. Review OverlayHost.qml and ShellSurfaceHost.qml with qt-qml-review if changing projection invalidation. The full-scene replay also flags ApplicationMenuPopup.qml delegate churn, but this must be validated against actual production streams before optimizing it.

## Reproduction artifacts

- [internal/app/viewer_search_profile_test.go](D:/Code/f4-zoin/internal/app/viewer_search_profile_test.go)
- [qt/host/tests/F4OperationsQueueTests.cpp](D:/Code/f4-zoin/qt/host/tests/F4OperationsQueueTests.cpp)
- [artifacts/viewer-search-go-profile.txt](D:/Code/f4-zoin/artifacts/viewer-search-go-profile.txt)
- [artifacts/viewer-search-cpu.pprof](D:/Code/f4-zoin/artifacts/viewer-search-cpu.pprof)
- [artifacts/viewer-search-qt-incremental.txt](D:/Code/f4-zoin/artifacts/viewer-search-qt-incremental.txt)
- [artifacts/viewer-search-qt-incremental-direct.txt](D:/Code/f4-zoin/artifacts/viewer-search-qt-incremental-direct.txt)
- [artifacts/viewer-search-qml-incremental-summary.json](D:/Code/f4-zoin/artifacts/viewer-search-qml-incremental-summary.json)
- [profiler/traces/viewer-search-incremental-2026-10-08.qtd](D:/Code/f4-zoin/profiler/traces/viewer-search-incremental-2026-10-08.qtd)

AI assistance has been used to create this output.
