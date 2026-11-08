# SVG logo zoom: offscreen filter texture growth

Captured 2026-11-08 on branch `zoin`, commit `941ec11ea`, canonical macOS checkout, Qt 6.11.1, Cocoa/Metal, DPR 2. The original SVG is `FarManager/logo.svg` (2,760 bytes, 256×256, viewBox 32×32). This capture predates the fix described below.

## Finding

The SVG contains a shadow filter: flood → composite-in → Gaussian blur → offset → composite-over. Qt generates multiple `ShaderEffectSource` passes whose `textureSize` follows `ItemSpy.requiredTextureSize`. The generated root scales with the viewer image dimensions; these offscreen passes follow the whole magnified filter rectangle, rather than the visible viewport. A clip around the image does not bound the effect render targets.

In the generated-scene replay at 1600%, six explicit passes request 9216×9216 and one requests 9472×9472 physical pixels. At four bytes/pixel this is 2,397,306,880 bytes (2.23 GiB), excluding MultiEffect internals, render attachments, driver overhead and overlapping old/new resources. At 100% the corresponding requests are only 576/592 pixels per side. There is a quadratic growth mechanism even though the SVG paths themselves are simple.

[Qt documents `textureSize` in physical pixels and supports a smaller `sourceRect` for a partial source](https://doc.qt.io/qt-6/qml-qtquick-shadereffectsource.html). The practical remedy is a bounded, viewport-aware filter render region with enough halo for blur/offset, while retaining vector geometry. A blanket zoom cap or rasterizing the entire SVG would change the viewer contract; a fixed filter-resolution cap would trade filtered-detail quality for bounded memory.

## Native application evidence

- Opened the viewer with Enter; held `+` grows continuously, without relying on injected OS auto-repeat. Initial sampled screenshot reached 1103%; a second 3.5-second hold reached the existing 12800% maximum.
- Pre-fix CPU sample (`f4-svg-zoom-before.sample.txt`): process physical footprint 8.1 GiB, peak 13.3 GiB. This is not the small JS heap and not comparable to RSS alone on shared-memory Metal.
- Main thread: 466/2746 samples wait in `QSGThreadedRenderLoop::polishAndSync → QWaitCondition`. Render thread: 464 samples wait in `QRhiMetal::beginFrame → semaphore_wait_trap`; another 165 wait for `CAMetalLayer nextDrawable`. Nested `QSGRhiLayer::grab/updateTexture` passes are present.
- These waits indicate GPU/frame backpressure during this capture. The sample does not establish GPU shader utilization or isolate a single Metal allocation call.
- At 12800%, the actual screenshot still contains cyan/blue geometry; the icon's edges are outside the viewport. A completely empty/failed frame was not reproduced or proved. The apparent disappearance has this additional geometric explanation.
- Native RSS was about 47 MiB at an idle observation after capture; RSS omits shared/GPU accounting and does not prove that filter resources were released. Returned the viewer to 100% using its `9` shortcut and verified the complete icon on screen.

## QML profiling capture

Profile mode: full. Raw traces, samples and replay files are local capture artifacts. A minimal standalone replay loads an exact copy of the production-generated `Prepared.qml`, uses the same Qt libraries/Metal/DPR, clips to 1512×840 logical pixels, and animates 100%→1600% over 5 seconds. It exits normally after settling. It isolates the generated effect graph; it does not profile f4 input routing or the complete Gallery viewport. A 500 ms inspector logs requested texture sizes and is an observer cost.

- Trace: `logo-filter.qtd` (6.6 MiB; profiler exited 0).
- Replay: `Replay.qml`.
- Generated scene: `Prepared.qml`.
- Parsed data: `summary.json`.
- Estimated duration from animation counters: 4.651 seconds; the scripted lifetime is approximately 6 seconds. These differ because the estimator is not a direct wall-clock measurement.
- Sum of captured range-event durations: 239.91 ms; **not wall-clock time**. Inclusive events overlap.
- Total captured events: 83643.

### Event types

| Type | Count | Total ms | ms/animation frame |
|---|---:|---:|---:|
| Creating | 164 | 91.73 | 0.168 |
| Binding | 24041 | 66.27 | 0.122 |
| Compiling | 4 | 56.46 | 0.104 |
| Javascript | 26946 | 22.88 | 0.042 |
| HandlingSignal | 12 | 2.56 | 0.005 |

### Animation counters and frame-gap limitations

Lower frame gaps are smoother. p95/p99 mean approximately 5%/1% of observed intervals are worse (about 27/5 out of 545). At 60 Hz one display interval is 16.67 ms; >33 ms is a visible interruption, >50 ms a stall. These are **QML animation counters**, not a GPU presentation trace: startup/idle/termination boundaries and capped rate counters affect them. The 1000 ms maximum must not be presented as a proven one-second zoom stall.

| Counter | Value |
|---|---:|
| frame_count | 545 |
| frame_ms_p50 | 8.0 |
| frame_ms_p95 | 9.01 |
| frame_ms_p99 | 47.62 |
| frame_ms_max | 1000.0 |
| frames_over_25ms | 14 |
| frames_over_33ms | 10 |
| frames_over_50ms | 5 |
| avg_framerate | 117.2 |
| min_framerate | 1 |
| max_framerate | 125 |

### QML memory

The JS heap stays small; the profiler does not measure the Metal filter textures. There are 29,905 JS allocations totaling 4,646,240 bytes, about 68.9% reclaimed. Peak live GC pages are 1,878,272 bytes; final live GC pages are 1,619,200 bytes. This cannot explain the native 13.3 GiB process peak.

| Category | Allocations | Total bytes | Reclaimed bytes | Peak live bytes | Live at exit |
|---|---:|---:|---:|---:|---:|
| heap_pages | 40 | 3756544 | 2137344 | 1878272 | 1619200 |
| small_items | 29905 | 4646240 | 3200128 | 1719008 | 1446112 |

GC pages are allocator capacity; small items are JS object allocations. Large items are all zero. No pixmap-cache summary was emitted; the problematic surfaces are effect render targets, not loaded bitmap pixmaps.

### Top hotspots

Counts depend on run length and zoom updates. Inclusive startup construction dominates total range time; it is not the sustained high-zoom bottleneck.

| Rank | Total ms | Count | Average ms | ms/frame | Type | Source | Details |
|---:|---:|---:|---:|---:|---|---|---|
| 1 | 85.94 | 2 | 42.969 | 0.158 | Creating | `Replay.qml:3` | QtQuick/Window |
| 2 | 32.1 | 1 | 32.097 | 0.059 | Compiling | `Replay.qml:0` | Replay.qml |
| 3 | 25.54 | 1088 | 0.023 | 0.047 | Binding | `Prepared.qml:15` |  |
| 4 | 23.73 | 2 | 11.864 | 0.044 | Compiling | qrc:/qt-project.org/imports/QmlRuntime/Config/default.qml | qrc:/qt-project.org/imports/QmlRuntime/Config/default.qml |
| 5 | 14.84 | 3270 | 0.005 | 0.027 | Binding | `Prepared.qml:215` |  |
| 6 | 6.53 | 684 | 0.01 | 0.012 | Javascript | `Replay.qml:28` | inspect |
| 7 | 4.9 | 3268 | 0.001 | 0.009 | Binding | `Prepared.qml:60` |  |
| 8 | 4.09 | 3268 | 0.001 | 0.008 | Binding | `Prepared.qml:118` |  |
| 9 | 4.05 | 3268 | 0.001 | 0.007 | Binding | `Prepared.qml:89` |  |
| 10 | 3.93 | 3266 | 0.001 | 0.007 | Binding | `Prepared.qml:132` |  |
| 11 | 3.73 | 3268 | 0.001 | 0.007 | Binding | `Prepared.qml:151` |  |
| 12 | 3.63 | 3268 | 0.001 | 0.007 | Binding | `Prepared.qml:179` |  |
| 13 | 3.25 | 3270 | 0.001 | 0.006 | Javascript | `Prepared.qml:215` | expression for textureSize |
| 14 | 2.56 | 12 | 0.213 | 0.005 | HandlingSignal | `Replay.qml:35` |  |
| 15 | 2.53 | 12 | 0.211 | 0.005 | Javascript | `Replay.qml:35` | expression for onTriggered |
| 16 | 2.19 | 544 | 0.004 | 0.004 | Javascript | `Replay.qml:14` | expression for width |
| 17 | 2.07 | 2 | 1.033 | 0.004 | Creating | `Prepared.qml:220` | QtQuick/Loader |
| 18 | 1.24 | 2 | 0.62 | 0.002 | Creating | `Prepared.qml:91` | QtQuick.Effects/MultiEffect |
| 19 | 1.07 | 3268 | 0.0 | 0.002 | Javascript | `Prepared.qml:118` | expression for textureSize |
| 20 | 0.98 | 3268 | 0.0 | 0.002 | Javascript | `Prepared.qml:60` | expression for textureSize |
| 21 | 0.82 | 3266 | 0.0 | 0.002 | Javascript | `Prepared.qml:132` | expression for textureSize |
| 22 | 0.8 | 3268 | 0.0 | 0.001 | Javascript | `Prepared.qml:89` | expression for textureSize |
| 23 | 0.76 | 3268 | 0.0 | 0.001 | Javascript | `Prepared.qml:179` | expression for textureSize |
| 24 | 0.68 | 1 | 0.682 | 0.001 | Binding | `Prepared.qml:224` |  |
| 25 | 0.68 | 1 | 0.681 | 0.001 | Javascript | `Prepared.qml:224` | expression for sourceX |
| 26 | 0.67 | 1 | 0.674 | 0.001 | Binding | `Prepared.qml:185` |  |
| 27 | 0.67 | 1 | 0.672 | 0.001 | Javascript | `Prepared.qml:185` | expression for originalBounds |
| 28 | 0.66 | 3268 | 0.0 | 0.001 | Javascript | `Prepared.qml:151` | expression for textureSize |
| 29 | 0.64 | 1 | 0.636 | 0.001 | Compiling | qrc:/data/BlurItem.qml | qrc:/data/BlurItem.qml |
| 30 | 0.58 | 545 | 0.001 | 0.001 | Javascript | `Replay.qml:16` | expression for x |

### Five project/replay hotspot analyses

#### `Replay.qml:3` — Creating

```qml
import QtQuick
import QtQuick.Window
Window {
    id: win
    width: 1512; height: 840; visible: true
    title: "SVG filter zoom profiling replay"
```

Window construction is a startup cost, not repeated on every zoom tick. Reusing the viewer window is already the correct production pattern; reducing this inclusive constructor time does not solve filter target growth.

#### `Replay.qml:0` — Compiling

```qml
import QtQuick
import QtQuick.Window
Window {
    id: win
    width: 1512; height: 840; visible: true
```

The replay compiles on startup. Production prepared components are cached by source revision. Preserve this reuse; compilation is not a scale-dependent explanation.

#### `Prepared.qml:15` — Binding

```qml
    signal restart()
    transform: [
        Scale { xScale: width / 32; yScale: height / 32 }
    ]
    id: _qt_node0
    transformOrigin: Item.TopLeft
```

The scale binding propagates the requested magnification through the scene. Its own measured CPU cost is small. Downstream ItemSpy sizes drive filter target reallocation; bound/crop that work rather than throttling input or freezing scale.

#### `Prepared.qml:215` — Binding

```qml
        sourceItem: _qt_node2
        sourceRect: _qt_node1_filterParameters.adaptToFilterRect(_qt_node2.originalBounds.x, _qt_node2.originalBounds.y, _qt_node2.originalBounds.width, _qt_node2.originalBounds.height)
        textureSize: _qt_node2_itemspy.requiredTextureSize
        width: sourceRect.width
        height: sourceRect.height
        visible: false
```

This binding sizes a filter pass at its transformed full-image resolution. Individual binding execution is cheap, but the value admits a very large offscreen texture. Use a viewport-aware filter source rectangle with blur/offset halo and consistent coordinates across dependent passes.

#### `Replay.qml:28` — Javascript

```qml
        ScriptAction { script: Qt.quit() }
    }
    function inspect(item, result) {
        if (item.textureSize !== undefined && item.sourceItem !== undefined)
            result.push({w: item.textureSize.width, h: item.textureSize.height})
        for (let child of item.children) inspect(child, result)
```

This is the diagnostic inspector, not production code. Its object traversal and JSON logging add observer work; remove it from clean cadence validation.

## Original follow-up options

1. Bound the filter working region to the visible viewport plus the authored filter neighborhood; preserve vector sharpness and correct composite coordinates. Handle zoom and pan, DPR, offscreen sources and rotated images.
2. Add a Metal/RHI regression based on this filtered SVG: held zoom 100%→12800%, pan at large scale, zoom back to fit; check real render-target dimensions, memory bound and painted current-generation output. Include 175% DPR, and retain software-rendering fallback coverage.
3. Repeat the native capture with frame timing and shared/GPU memory accounting. Keep logical JS memory separate from native Metal allocation and do not use average framerate as the acceptance criterion.

> AI assistance has been used to create this output.

## Implemented fix and validation

`third_party/ZoinGallery/VectorSource.cpp` now bounds the Qt-generated
`ShaderEffectSource.textureSize` bindings during worker-side preparation.
Each explicit filter target has a maximum edge of 2048 physical pixels, and
16 MiPixels are divided between generated texture bindings. Aspect ratio is
preserved. This is an intermediate-texture budget, not a bound on total process
memory: MultiEffect internal targets, attachments, transient generations and
instantiated reusable filter components also consume memory.

The geometry, source rectangles, filter coordinates and viewer zoom remain
unchanged. Unfiltered paths continue to use the curve renderer. At extreme zoom,
filtered content has reduced raster detail; viewport cropping with dependency
halos remains a possible quality improvement, not part of this fix. Normal-scale
requests below the budget keep their original resolution. A small floating-point
rounding tolerance prevents capped target dimensions oscillating by one pixel.
The generated-binding contract is checked before publishing the prepared scene.

Regression: `GallerySessionTest::vectorFilterTexturesStayBoundedAtHighZoom`.
The pre-fix run failed with `unbounded texture: 129024x129024` before showing the
window, avoiding the corresponding GPU allocation (effective DPR 3.5 on Retina).
After the fix, the same test passes on Metal at measured DPR 3.5 and 1.75.
It checks explicit target sizes and aggregate pixels at 12800%, renders at 100%,
zooms/pans at 400%, 1600% and 12800%, verifies both unfiltered cyan geometry and
filtered black output, and compares the restored 100% image exactly. Normal and
restored captures at DPR 1.75 were inspected; the authored shadow remains visible.
Existing gradient/alpha/embedded-image and authored-animation regressions pass
in the same run (5 QtTest results including init/cleanup). The animation test
still emits its existing paused-animation warning.

The canonical Qt host was rebuilt and launched with `./run-f4.command` and the
normal profile. Opened the original FarManager/logo.svg with Enter, reset to
100%, held plus for 10 seconds, confirmed 12800% and visible current output,
then reset to 100% and confirmed the complete icon. Native sample:
`f4-svg-zoom-after.sample.txt` (12 seconds): physical footprint 420.5M,
peak 527.6M, versus the previous observed peak 13.3G. These are process physical
footprints, not isolated GPU texture accounting or a frame-time guarantee.
No Go behavior changed.
