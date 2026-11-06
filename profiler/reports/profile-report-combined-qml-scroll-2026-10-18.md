# QML scrolling allocation and binding profile — 2026-10-18

Profile: memory, creating, binding, JavaScript and signal handlers. Source: `profiler/traces/combined-qml-memory-2026-10-18.qtd`; filtered input: `combined-qml-scroll-clusters-2026-10-18.qtd`. Client exited 0 and the source XML is complete. Wall-clock frame duration/FPS is unavailable from this trace because animation events were not captured.

The filter retains ranges wholly inside two sustained contentY-change clusters (about 3.107 and 4.132 seconds). This removes setup-heavy panel activation from the scrolling diagnosis. It is a proxy for active scrolling, not exact QPC synchronization; it omits some stationary time at the end of a direction. The independent scheduler capture uses exact raw-QPC alignment.

Sum of captured range durations: 1680.83 ms. Nested JS, binding, creating and signal intervals overlap; this is not wall-clock CPU time. Raw counts scale with the interaction and capture length. No frame-cadence performance claim is made from this instrumented capture.

## Event types

| Type | Count | Inclusive ms |
| --- | ---: | ---: |
| Javascript | 92196 | 657.72 |
| Binding | 49485 | 563.97 |
| HandlingSignal | 7417 | 256.49 |
| Creating | 10008 | 202.66 |

## Memory

The filtered windows contain 86,948 positive small-object allocation events totaling 7,043,680 bytes (6.72 MiB), with 5,688,608 bytes reclaimed (80.8%). These are QML managed-allocation events, not OS heap allocations. No large-item or heap-page events occur in the filtered windows; that does not mean the process has no heap pages. The parser’s 5,284,800-byte peak and 1,355,072-byte ending balance are incremental balances from this filtered input, not the actual engine’s total live heap. Existing objects/pages predate these windows.

The whole source trace includes setup: 512,732 allocation events and 52,273,024 allocated bytes. Those totals must not be described as scrolling-only pressure.

## Top 30 ranges

Locations retain original trace line numbers; GalleryPanelChrome gained one/two lines afterward, and its handler/hidden-position expression currently starts at lines 70/104.

| Inclusive ms | Count | Average ms | Type | Source | Detail |
| ---: | ---: | ---: | --- | --- | --- |
| 144.44 | 1064 | 0.136 | HandlingSignal | [GalleryPanelChrome.qml:69](../../third_party/ZoinGallery/qml/GalleryPanelChrome.qml) |  |
| 143.41 | 1066 | 0.135 | Javascript | [GalleryPanelChrome.qml:69](../../third_party/ZoinGallery/qml/GalleryPanelChrome.qml) | onContentYChanged |
| 55.42 | 1068 | 0.052 | Binding | [GalleryPanelChrome.qml:102](../../third_party/ZoinGallery/qml/GalleryPanelChrome.qml) |  |
| 36.4 | 270 | 0.135 | Javascript | [GalleryEntryPreview.qml:279](../../third_party/ZoinGallery/qml/GalleryEntryPreview.qml) | commitComponent |
| 35.81 | 234 | 0.153 | Binding | [GalleryEntryPreview.qml:284](../../third_party/ZoinGallery/qml/GalleryEntryPreview.qml) |  |
| 33.19 | 305 | 0.109 | Binding | [GalleryEntryDelegateBase.qml:280](../../third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml) |  |
| 32.59 | 1064 | 0.031 | HandlingSignal | [GalleryViewport.qml:353](../../third_party/ZoinGallery/qml/GalleryViewport.qml) |  |
| 32.0 | 730 | 0.044 | Binding | [GalleryEntryDelegateBase.qml:83](../../third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml) |  |
| 30.75 | 1064 | 0.029 | Javascript | [GalleryViewport.qml:353](../../third_party/ZoinGallery/qml/GalleryViewport.qml) | expression for onContentYChanged |
| 29.06 | 236 | 0.123 | HandlingSignal | [GalleryPointerLayer.qml:49](../../third_party/ZoinGallery/qml/GalleryPointerLayer.qml) |  |
| 28.05 | 236 | 0.119 | Javascript | [GalleryPointerLayer.qml:49](../../third_party/ZoinGallery/qml/GalleryPointerLayer.qml) |  |
| 24.3 | 236 | 0.103 | HandlingSignal | [GalleryViewport.qml:711](../../third_party/ZoinGallery/qml/GalleryViewport.qml) |  |
| 23.9 | 236 | 0.101 | Javascript | [GalleryViewport.qml:711](../../third_party/ZoinGallery/qml/GalleryViewport.qml) |  |
| 23.41 | 322 | 0.073 | Binding | [GalleryEntryDelegateBase.qml:160](../../third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml) |  |
| 23.29 | 236 | 0.099 | Javascript | [GalleryPanelFacade.qml:74](../../third_party/ZoinGallery/qml/GalleryPanelFacade.qml) | handlePanelWheel |
| 23.2 | 85 | 0.273 | Creating | [GalleryViewport.qml:388](../../third_party/ZoinGallery/qml/GalleryViewport.qml) | GalleryEntryDelegateBase |
| 22.75 | 170 | 0.134 | Creating | [GalleryEntryDelegateBase.qml:9](../../third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml) | ZoinGallery.Native/BrickItem |
| 22.73 | 236 | 0.096 | Javascript | [GalleryViewportController.qml:301](../../third_party/ZoinGallery/qml/GalleryViewportController.qml) | handlePanelWheel |
| 22.42 | 415 | 0.054 | Binding | [GalleryEntryPreview.qml:142](../../third_party/ZoinGallery/qml/GalleryEntryPreview.qml) |  |
| 20.5 | 2726 | 0.008 | Javascript | [GalleryEntryDelegateBase.qml:100](../../third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml) | iconPixelOffset |
| 19.52 | 1065 | 0.018 | Javascript | [GalleryPanelFacade.qml:175](../../third_party/ZoinGallery/qml/GalleryPanelFacade.qml) | updateVisualCursorForViewport |
| 19.14 | 170 | 0.113 | Creating | [GalleryEntryDelegateBase.qml:292](../../third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml) | QtQuick/Loader |
| 17.41 | 1065 | 0.016 | Javascript | [GalleryCursorController.qml:177](../../third_party/ZoinGallery/qml/GalleryCursorController.qml) | updateVisualCursorForViewport |
| 15.78 | 417 | 0.038 | Binding | [GalleryEntryPreview.qml:309](../../third_party/ZoinGallery/qml/GalleryEntryPreview.qml) |  |
| 14.79 | 1093 | 0.014 | Binding | [GalleryGroupHeaders.qml:12](../../third_party/ZoinGallery/qml/GalleryGroupHeaders.qml) |  |
| 13.18 | 236 | 0.056 | Javascript | [GalleryViewportController.qml:229](../../third_party/ZoinGallery/qml/GalleryViewportController.qml) | scrollBy |
| 12.64 | 236 | 0.054 | Javascript | [GalleryViewportController.qml:187](../../third_party/ZoinGallery/qml/GalleryViewportController.qml) | animatePanelScrollTo |
| 12.53 | 184 | 0.068 | Binding | [GalleryEntryDelegateBase.qml:61](../../third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml) |  |
| 11.87 | 668 | 0.018 | Binding | [GalleryCursorController.qml:88](../../third_party/ZoinGallery/qml/GalleryCursorController.qml) |  |
| 11.85 | 374 | 0.032 | Binding | [GalleryEntryPreview.qml:17](../../third_party/ZoinGallery/qml/GalleryEntryPreview.qml) |  |

## Analysis of the top five ranges

1. GalleryPanelChrome onContentYChanged appears as both HandlingSignal and JavaScript (144.44/143.41 ms). These are overlapping views of one path. It updates the visible vertical scrollbar’s position from the content offset; do not count it twice or remove visible scroll feedback to improve a number.

2. GalleryPanelChrome horizontal position binding executes 1,068 times and totals 55.42 ms even in Masonry, where the horizontal bar is hidden. Guarding the binding with visibility prevents subscribing to contentY while hidden. A native regression observes seven hidden position-change signals before the fix and zero afterward, then verifies the visible Columns binding and resting thumb geometry.

3. GalleryEntryPreview commitComponent (270 calls, 36.40 ms) and the Loader sourceComponent binding (234 calls, 35.81 ms) include deferred renderer replacement and construction. They overlap with creating ranges. Their purpose is to preserve typed source identity during recycled delegate changes; removing the deferred exchange blindly risks stale bitmap/vector rendering. Reduce unnecessary requested-component changes only with recycling/source-replacement regressions.

Relevant current source excerpts:

```qml
function onContentYChanged() {
    const extent = chromeRoot.layout.contentHeight
    verticalScroll.position = extent > 0 ? chromeRoot.layout.contentY / extent : 0
}

// Horizontal bar: new guard, measured expression was previously unconditional.
position: visible && chromeRoot.layout.contentHeight > 0
          ? chromeRoot.layout.contentY / chromeRoot.layout.contentHeight : 0

function commitComponent() {
    if (preview.activePresentation) retainedComponent = requestedComponent
}
sourceComponent: retainedComponent
```

## Allocation attribution proxy and next work

A separate sweep assigns positive small-object events to the earliest-ending active QML range. This is a temporal attribution proxy, not a C++ allocator stack or proof of exclusive causality. Largest sites include iconPixelOffset (3,969 events / 468,256 bytes), the group-header model expression (2,186 / 279,808 bytes), indexIntersectsViewport (2,130 / 204,480 bytes), and shapesReady (1,272 / 161,280 bytes). The enum mapping is 0=heap pages, 1=large items, 2=small items.

Investigate repeated point/URL wrappers and group-header list conversion, particularly empty group models, with dedicated regressions. Measure exact GC phase durations in the combined engine rather than revisiting the already inconclusive constant-only budget/chunk experiments. Delegate creation also appears on long GUI intervals. The independent sampled capture confirms GUI execution/GC as the next critical path after the rendering experiments.

The scrollbar update guard and a cached, motion-suspended thumb pixel correction are the only production changes from this investigation. No claim is made that they eliminate the remaining spikes.

AI assistance has been used to create this output.
