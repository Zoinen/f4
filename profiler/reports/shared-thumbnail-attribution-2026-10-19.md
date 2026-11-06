# Shared-thumbnail build: current scrolling attribution

Logic capture, 2026-10-19, Qt 6.11.1, active 160 Hz display, DPR 1.75,
Downloads with 186 entries, Masonry density 152, actual wheel input every 30 ms.
The two measured directions last four seconds each. Caption measurements: zero.
This is attribution with observers, not a clean display-cadence benchmark.

The QML-only trace is filtered to ranges wholly inside the two measured
direction windows using calibrated native/QML clocks. Setup and navigation
are excluded. Clock offset spread is 400 ns; maximum uncertainty is 900 ns;
all 267 QML wheel starts fall inside their native handler windows.
Wall-clock duration unavailable from the QML parser (no animation events captured).
The native driver supplies the measured eight seconds; it is not inferred from summed events.

Captured ranges: 142641. Sum of captured nested range durations:
987.12 ms; this is not wall-clock CPU time.

| Type | Count | Nested total ms |
| --- | ---: | ---: |
| Binding | 48166 | 456.5 |
| Javascript | 88254 | 416.5 |
| HandlingSignal | 6221 | 114.12 |

Counts depend on capture length and interaction; nested totals overlap.
Memory and pixmap-cache events were not enabled, so no memory/cache conclusions follow.

## Native periods and correlated costs

Frame time here is the interval between native frame callbacks, not a measured display interval.
p50 is the median; p95/p99 leave five/one percent of frames worse. At 160 Hz
the refresh interval is 6.25 ms. With 1277 frames, about 64 are at or above
the p95 of 7.4001 ms and about 13 at or above the p99 of 8.545 ms.
Median 6.2288 ms, maximum 11.8953 ms; seven periods exceed 9.375 ms.
Three of those seven overlap GC, four do not. No period exceeds 25/33/50 ms.

The longest vector-loader binding is 5.9653 ms, including 5.8207 ms of
GC states. Its remaining inclusive time is 0.1446 ms. It is not evidence
of six milliseconds of vector geometry or centering arithmetic. Event-loop
mark draining reaches 5.0834 ms, and atomic sweep reaches 2.8383 ms.
Preview renderer exchange reaches 1.552 ms without GC. Nested binding
and commitComponent durations must not be added together.

## QML hotspots during scrolling

| Rank | ms | Count | Average ms | Type | Source | Detail |
| --- | ---: | ---: | ---: | --- | --- | --- |
| 1 | 36.34 | 281 | 0.129 | Javascript | [GalleryEntryPreview.qml:282](../../third_party/ZoinGallery/qml/GalleryEntryPreview.qml#L282) | commitComponent |
| 2 | 35.64 | 224 | 0.159 | Binding | [GalleryEntryPreview.qml:287](../../third_party/ZoinGallery/qml/GalleryEntryPreview.qml#L287) |  |
| 3 | 34.32 | 742 | 0.046 | Binding | [GalleryEntryDelegateBase.qml:83](../../third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml#L83) |  |
| 4 | 31.31 | 311 | 0.101 | Binding | [GalleryEntryDelegateBase.qml:286](../../third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml#L286) |  |
| 5 | 29.8 | 265 | 0.112 | HandlingSignal | [GalleryPointerLayer.qml:49](../../third_party/ZoinGallery/qml/GalleryPointerLayer.qml#L49) |  |
| 6 | 28.77 | 265 | 0.109 | Javascript | [GalleryPointerLayer.qml:49](../../third_party/ZoinGallery/qml/GalleryPointerLayer.qml#L49) |  |
| 7 | 24.45 | 1083 | 0.023 | HandlingSignal | [GalleryPanelChrome.qml:70](../../third_party/ZoinGallery/qml/GalleryPanelChrome.qml#L70) |  |
| 8 | 24.35 | 265 | 0.092 | HandlingSignal | [GalleryViewport.qml:711](../../third_party/ZoinGallery/qml/GalleryViewport.qml#L711) |  |
| 9 | 23.96 | 265 | 0.09 | Javascript | [GalleryViewport.qml:711](../../third_party/ZoinGallery/qml/GalleryViewport.qml#L711) |  |
| 10 | 23.67 | 341 | 0.069 | Binding | [GalleryEntryDelegateBase.qml:166](../../third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml#L166) |  |
| 11 | 23.44 | 1083 | 0.022 | Javascript | [GalleryPanelChrome.qml:70](../../third_party/ZoinGallery/qml/GalleryPanelChrome.qml#L70) | onContentYChanged |
| 12 | 23.39 | 265 | 0.088 | Javascript | [GalleryPanelFacade.qml:74](../../third_party/ZoinGallery/qml/GalleryPanelFacade.qml#L74) | handlePanelWheel |
| 13 | 22.85 | 265 | 0.086 | Javascript | [GalleryViewportController.qml:301](../../third_party/ZoinGallery/qml/GalleryViewportController.qml#L301) | handlePanelWheel |
| 14 | 14.2 | 1083 | 0.013 | HandlingSignal | [GalleryViewport.qml:353](../../third_party/ZoinGallery/qml/GalleryViewport.qml#L353) |  |
| 15 | 13.56 | 265 | 0.051 | Javascript | [GalleryViewportController.qml:229](../../third_party/ZoinGallery/qml/GalleryViewportController.qml#L229) | scrollBy |
| 16 | 13.22 | 193 | 0.068 | Binding | [GalleryEntryDelegateBase.qml:61](../../third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml#L61) |  |
| 17 | 13.04 | 265 | 0.049 | Javascript | [GalleryViewportController.qml:187](../../third_party/ZoinGallery/qml/GalleryViewportController.qml#L187) | animatePanelScrollTo |
| 18 | 12.86 | 1083 | 0.012 | Javascript | [GalleryViewport.qml:353](../../third_party/ZoinGallery/qml/GalleryViewport.qml#L353) | expression for onContentYChanged |
| 19 | 12.67 | 389 | 0.033 | Binding | [GalleryEntryPreview.qml:17](../../third_party/ZoinGallery/qml/GalleryEntryPreview.qml#L17) |  |
| 20 | 11.49 | 309 | 0.037 | Binding | [GalleryEntryPreview.qml:143](../../third_party/ZoinGallery/qml/GalleryEntryPreview.qml#L143) |  |
| 21 | 11.36 | 403 | 0.028 | Binding | [GalleryEntryPreview.qml:22](../../third_party/ZoinGallery/qml/GalleryEntryPreview.qml#L22) |  |
| 22 | 9.8 | 559 | 0.018 | Binding | [GalleryEntryDelegateBase.qml:40](../../third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml#L40) |  |
| 23 | 9.79 | 657 | 0.015 | Binding | [GalleryEntryPreview.qml:404](../../third_party/ZoinGallery/qml/GalleryEntryPreview.qml#L404) |  |
| 24 | 9.66 | 677 | 0.014 | Binding | [GalleryCursorController.qml:88](../../third_party/ZoinGallery/qml/GalleryCursorController.qml#L88) |  |
| 25 | 8.9 | 369 | 0.024 | Binding | [GalleryEntryPreview.qml:312](../../third_party/ZoinGallery/qml/GalleryEntryPreview.qml#L312) |  |
| 26 | 8.46 | 505 | 0.017 | Binding | [GalleryEntryPreview.qml:155](../../third_party/ZoinGallery/qml/GalleryEntryPreview.qml#L155) |  |
| 27 | 8.44 | 423 | 0.02 | Binding | [MasonryEntryDelegate.qml:47](../../third_party/ZoinGallery/qml/MasonryEntryDelegate.qml#L47) |  |
| 28 | 7.98 | 530 | 0.015 | Javascript | [GalleryViewportController.qml:83](../../third_party/ZoinGallery/qml/GalleryViewportController.qml#L83) | cancelCursorChromeTransition |
| 29 | 7.63 | 657 | 0.012 | Binding | [GalleryEntryPreview.qml:169](../../third_party/ZoinGallery/qml/GalleryEntryPreview.qml#L169) |  |
| 30 | 7.26 | 530 | 0.014 | Javascript | [GalleryPanelFacade.qml:200](../../third_party/ZoinGallery/qml/GalleryPanelFacade.qml#L200) | cancelCursorChromeTransition |

## Source checks for the five largest totals

### [GalleryEntryPreview.qml:282](../../third_party/ZoinGallery/qml/GalleryEntryPreview.qml#L282)

```qml
        }
        function commitComponent() {
            if (preview.activePresentation) retainedComponent = requestedComponent
        }
        onRequestedComponentChanged: updateComponent()
```

The deferred callback assigns retainedComponent, which synchronously
exchanges a Loader subtree through sourceComponent. These two hotspot
rows overlap. Test staged incubation with readiness and row-identity
checks before publishing; merely deferring the assignment again does
not divide the creation work.

### [GalleryEntryPreview.qml:287](../../third_party/ZoinGallery/qml/GalleryEntryPreview.qml#L287)

```qml
        Component.onCompleted: updateComponent()
        sourceComponent: retainedComponent
    }

    Component {
```

The deferred callback assigns retainedComponent, which synchronously
exchanges a Loader subtree through sourceComponent. These two hotspot
rows overlap. Test staged incubation with readiness and row-identity
checks before publishing; merely deferring the assignment again does
not divide the creation work.

### [GalleryEntryDelegateBase.qml:83](../../third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml#L83)

```qml
    }
    readonly property bool iconPixelAlignmentActive:
        visible && sceneVisible && !geometryAnimationRunning
    readonly property bool motionActive:
        geometryAnimationRunning || panelRoot.cursorPixelAlignmentSuspended
```

cellVerticalInset reads native previewRect and falls back to size/DPR
arithmetic. Descendant geometry updates are included in its duration.
Check whether atomic native row commits invalidate unchanged rectangles
before adding another cache or rounding operation.

### [GalleryEntryDelegateBase.qml:286](../../third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml#L286)

```qml
        shaderThumbnail: entry.masonryMode || entry.gridMode
        activePresentation: entry.viewIndex >= 0
            && entry.visualGeometryReady && !entry.visualIdentityChanging
            && (!entry.folderPreviewRequested
            || entry.visualModel.folderPreviewState !== 2)
```

activePresentation changes preview source/Loader activity during row
identity handoff. Its inclusive time includes downstream work. Preserve
the identity barrier; inspect duplicate state transitions before changing
publication, otherwise recycled content can flash in the wrong row.

### [GalleryPointerLayer.qml:49](../../third_party/ZoinGallery/qml/GalleryPointerLayer.qml#L49)

```qml
        acceptedButtons: Qt.NoButton
        onWheel: wheel => {
            if (pointerLayer.wheelMode === "console") {
                pointerLayer.consoleWheelRequested(
                    wheel.x, wheel.y, wheel.angleDelta.y, wheel.modifiers)
```

The wheel handler forwards input through the existing panel router.
Its average is 0.112 ms, including nested work; these data do not
justify throttling or dropping wheel events. Prioritize the verified
multi-millisecond creation and GC intervals.

These totals include nested work and possible GC. The correlated subtraction
above is necessary before treating a binding as expensive arithmetic.
Renderer exchange creates a real component subtree synchronously; incremental
or staged creation is a candidate only with publication/recycling tests proving
that a ready thumbnail never flashes blank. Existing nearby preparation remains
enabled because disabling it did not improve the clean comparison.

## Simultaneous scheduling repeat

shared-thumbnail-sampled-wheel-masonry adds CPU samples and context switches.
All 56890 relevant raw/exported switches match in identity and order at a
10 MHz QPC frequency, with a 900 ns offset spread. ETW reports zero events
and buffers lost. QML clock spread is 600 ns, uncertainty 1000 ns, and all
268 wheel starts match. These observer results do not establish a clean speedup.

The worst 36.1337 ms native interval has 0.1368 ms of captured QML and no
GC-state overlap. GUI execution is 1.0185 ms, waiting 34.9642 ms, and ready
delay 0.151 ms. Most waiting is classified WrResource. Render execution is
3.2177 ms and waiting 32.9 ms. This is not a 36 ms binding or scheduler starvation.
One GUI CPU sample contains QWindowsContext::initTouch and
QWindowsPointerHandler::createTouchDevice. Exact host-only linker-map
resolution uses the PE entry-point RVA and main-thread start address;
identical-code-folding aliases are retained and ambiguous large sets remain
explicitly unidentified. DLL symbols and inlined frames remain unresolved.

The 14.3101 ms interval is different: GUI execution 13.2251 ms, waiting
1.01 ms and GC-state overlap 7.0824 ms. Samples contain mark draining and
event-loop GC transitions. Component creation/nearby preparation also remains
visible in CPU samples. No single cause explains all missed intervals.

## Device-change query probe

The exact [Qt 6.11.1 Windows context source](https://raw.githubusercontent.com/qt/qtbase/v6.11.1/src/plugins/platforms/windows/qwindowscontext.cpp)
calls initTouch synchronously for DBT_DEVNODES_CHANGED when no touch device
has been registered. [createTouchDevice](https://raw.githubusercontent.com/qt/qtbase/v6.11.1/src/plugins/platforms/windows/qwindowspointerhandler.cpp)
queries SM_DIGITIZER first. This makes the sampled stack a plausible lead,
not proof of the complete wait site.

A diagnostic-only IAT observer forwards the original GetSystemMetrics call
unchanged and records GUI-thread metric/result/start/end in a bounded 4096-entry
array. Snapshot serialization occurs after measurement; both captures install
the hook successfully and report no overflow. Other threads bypass recording.
Three notifications are sent only to this diagnostic window, never broadcast
to the OS. Each causes one SM_DIGITIZER query returning zero. They take
0.0366, 0.0521 and 0.0596 ms. They do not reproduce the long resource wait.
A passive repeat has five startup metrics, max 0.0008 ms, and no digitizer
calls. Its native maximum remains 13.634 ms, proving that this query is not
required for the remaining ordinary stalls. No device-enumeration production
change is justified from this experiment.

The full network-symbol export was deliberately stopped after a complete
independent no-symbol export and validated offline host-map replacement were
available. Its partial scheduler-local.csv is excluded. The map exporter
stores large ICF alias sets separately rather than assigning an arbitrary name.

## Next work and retained state

Keep the GC and component-incubation costs as confirmed targets. A future
real device-change capture can use the metrics observer to establish the
exact API duration before changing touch initialization. Do not disable device
support or repeat nominal GC-budget changes as if they bounded atomic sweeping.
The normal portable launcher is restored, no production change was made in
this attribution step, and the constant-160-Hz goal remains incomplete.

Compact native captures, joined summaries, scheduling/sampling summaries,
scripts and binary identities are archived with a SHA256 manifest under
profiler/experiments/shared-thumbnail-attribution-2026-10-19. Raw ETL/CSV/QTD
and executables remain outside the evidence archive. No commit or push.

> AI assistance has been used to create this output.
