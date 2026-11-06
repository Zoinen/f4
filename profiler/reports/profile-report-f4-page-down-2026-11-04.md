# Held Page Down in Masonry · 2026-11-04

Measured the expanded left Masonry panel on `P:\Events\[2026.10] UAE\[2026.10.28]` with 802 entries including the parent entry, a 160 Hz display and DPR 1.75. A disposable copy of the normal profile used the current worktree launcher and a diagnostic Qt host linked from the current production objects. An independent producer posted one press, 23 Qt autorepeats and a release through normal Qt key dispatch, with 32 ms and 16 ms repeat intervals. The initial press and its repeat delay are excluded. The existing thumbnail cache was in use. The diagnostic window held focus and the collectors took no screenshots during input.

## Frame cadence and input

| Metric | 32 ms repeats, two passes | 16 ms repeats, two passes |
| --- | ---: | ---: |
| Qt swaps per second | 155–160 | 160–163 |
| Distinct viewport positions per second | 114–121 | 66–74 |
| p95 interval between distinct positions | 19.9–26.1 ms | 31.4–31.6 ms |
| Longest interval between distinct positions | 39.0–45.3 ms | 31.6–37.6 ms |
| Key handler p50 | 0.42–0.57 ms | 0.38–0.40 ms |
| Key queue wait p95 | 26.8–36.6 ms | 37.6–42.3 ms |

Intervals compare successive native camera commits with a change of at least 0.25 logical pixels. At 160 Hz each refresh has 6.25 ms and at 60 Hz it has 16.67 ms. p95 is the 95th-percentile interval. Counting swaps alone overestimates visible movement when a frame reuses the same camera position. At 16 ms repeats, 29 of 55 and 32 of 56 camera commits had no meaningful movement. The repeat window ends at release or when input has already reached the final destination, whichever comes first. All 24 presses reached Qt.

PresentMon confirmed the display was refreshing during a separate attribution capture. The median display interval was 6.25 ms, p95 was about 12.5 ms, and p95 GPU-active time was 0.64–0.73 ms. The low-overhead capture put the CPU render phase at 1.5–2.7 ms p95. Two 32 ms passes kept moving for roughly 186 ms and 315 ms after release before reaching the last planned destination, despite a nominal 150 ms animation.

## Where time goes

1. **Synchronous tile updates on the GUI thread.** The render camera queues `syncNativePose()`, which writes the latest pose into `setContentY()`. If the visible tile set changed, `setContentYInternal()` calls `updateProperties()`. That commit accounted for 82–86% of the scroll-sync time in three attribution passes. The commit iterated a median 62–66 delegates, with up to 110 in a pass. Tile acquisition and visual-row notifications dominated expensive commits. At 32 ms repeats, their individual p95 times were 8.0–10.6 ms and 5.1–7.0 ms respectively. Thumbnail provider lookups on the GUI thread took 0.026–0.029 ms median. Worker cache lookup waits were 0.033–0.045 ms median. [Scroll sync](../../third_party/ZoinGallery/GalleryScrollAnimation.cpp#L546), [viewport update](../../third_party/ZoinGallery/MasonryLayoutProperties.cpp#L1162), [tile commit](../../third_party/ZoinGallery/MasonryLayoutDelegateCommit.cpp#L591)
2. **The camera catches up in bursts.** Each Page Down can add about 698 logical pixels to the pending destination in this viewport. The render camera clips travel to tiles already prepared and resets the easing origin when it hits that boundary. The measured clamp affected 18–47 commits in the 32 ms runs and 40–42 commits in the 16 ms runs. The animation can keep producing frames while the viewport waits for the GUI to prepare another band. The camera commit itself cost 0.10–0.12 ms p95. [Coverage clamp](../../third_party/ZoinGallery/GalleryScrollAnimation.cpp#L271), [paging destination](../../third_party/ZoinGallery/qml/GalleryNavigationController.qml#L571)
3. **A separate geometry update can discard queued travel.** The traced 32 ms runs had two and four rewraps during the repeat window. One pass logged a target of 6277.5 while the camera was at 3549.1 when the geometry invalidation stopped the scroll. The next page restarted from the live viewport and its cursor target moved backwards even though input was still Page Down. One rewrap followed a thumbnail metadata update by about 0.5 ms. The tile-commit bottleneck also occurred in a 16 ms attribution pass with zero rewraps. [Page invalidation](../../third_party/ZoinGallery/qml/GalleryNavigationController.qml#L362), [image-size updates](../../third_party/ZoinGallery/MasonryLayoutModelDelta.cpp#L370)

Keep individual page repeats and retain their pending logical destination across a real geometry revision. To improve cadence, prioritize preparing the next band in the movement direction, bound the synchronous tile work in scroll sync, and reuse existing delegate visuals where possible. Judge the change by distinct viewport positions, input queue wait and arrival at the final destination.

## QML attribution

A separate Qt QML profiler capture recorded the same 32 ms repeat sequence with JavaScript, bindings, signal handling, object creation, animation, scene graph and pixmap events enabled. The cropped repeat interval contains 216,627 parsed events. The parser estimated 976 ms from animation events. Range durations sum to 1253.23 ms and include nested work. They cannot be added to the native wall-clock timings.

| Type | Events | Inclusive total | Per recorded animation event |
| --- | ---: | ---: | ---: |
| JavaScript | 116,837 | 530.44 ms | 3.658 ms |
| Binding | 84,489 | 441.45 ms | 3.044 ms |
| Creating | 11,476 | 217.73 ms | 1.502 ms |
| Handling signal | 2,802 | 63.60 ms | 0.439 ms |

Qt recorded 145 animation events with 6.02 ms p50, 32.26 ms p95, 45.45 ms p99 and 47.62 ms max. Ten exceeded 25 ms and four exceeded 33 ms. These profiler animation estimates include different animation streams. The native camera and PresentMon measurements above distinguish content motion from display refresh. The pixmap trace recorded 128 requests, 128 loads and 187 removals. The largest reported load was 489 × 275 pixels. The profiler counts object and binding work, including pooled delegates updating their children.

The two largest individual tile costs were the preview binding in [GalleryEntryDelegateBase.qml](../../third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml#L298) and creating loader instances in [GalleryVisualHost.qml](../../third_party/ZoinGallery/qml/GalleryVisualHost.qml#L5). Both are downstream of admitting a new row. The three paging functions below are nested in one dispatch. They include synchronous work when a scroll job is started again and should be read as one call chain. The tile signal costs are the place to look for repeated work when the visual identity or prepared subtree is reusable.

| Rank | Total ms | Count | Avg ms | ms per animation event | Type | Source | Details |
| ---: | ---: | ---: | ---: | ---: | --- | --- | --- |
| 1 | 39.55 | 1042 | 0.038 | 0.273 | Binding | [GalleryEntryDelegateBase.qml:298](../../third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml#L298) |  |
| 2 | 33.34 | 624 | 0.053 | 0.23 | Creating | [GalleryVisualHost.qml:5](../../third_party/ZoinGallery/qml/GalleryVisualHost.qml#L5) | QtQuick/Loader |
| 3 | 31.72 | 23 | 1.379 | 0.219 | Javascript | [GalleryPanelInput.qml:129](../../third_party/ZoinGallery/qml/GalleryPanelInput.qml#L129) | handlePagePress |
| 4 | 31.48 | 23 | 1.369 | 0.217 | Javascript | [GalleryPanelFacade.qml:290](../../third_party/ZoinGallery/qml/GalleryPanelFacade.qml#L290) | navigateViewportPage |
| 5 | 31.45 | 23 | 1.367 | 0.217 | Javascript | [GalleryNavigationController.qml:632](../../third_party/ZoinGallery/qml/GalleryNavigationController.qml#L632) | navigateViewportPage |
| 6 | 31.32 | 23 | 1.362 | 0.216 | Javascript | [GalleryNavigationController.qml:571](../../third_party/ZoinGallery/qml/GalleryNavigationController.qml#L571) | navigateMasonryViewportPage |
| 7 | 28.84 | 22 | 1.311 | 0.199 | HandlingSignal | [GalleryPanel.qml:682](../../third_party/ZoinGallery/qml/GalleryPanel.qml#L682) |  |
| 8 | 28.68 | 22 | 1.304 | 0.198 | Javascript | [GalleryPanel.qml:682](../../third_party/ZoinGallery/qml/GalleryPanel.qml#L682) |  |
| 9 | 28.61 | 22 | 1.3 | 0.197 | Javascript | [GalleryPanelInput.qml:200](../../third_party/ZoinGallery/qml/GalleryPanelInput.qml#L200) | handlePressed |
| 10 | 26.66 | 1150 | 0.023 | 0.184 | Binding | [GalleryEntryDelegateBase.qml:82](../../third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml#L82) |  |
| 11 | 26.42 | 156 | 0.169 | 0.182 | Creating | [GalleryViewport.qml:389](../../third_party/ZoinGallery/qml/GalleryViewport.qml#L389) | GalleryEntryDelegateBase |
| 12 | 26.14 | 312 | 0.084 | 0.18 | Creating | [GalleryEntryDelegateBase.qml:9](../../third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml#L9) | ZoinGallery.Native/BrickItem |
| 13 | 22.91 | 23 | 0.996 | 0.158 | Javascript | [GalleryNavigationController.qml:67](../../third_party/ZoinGallery/qml/GalleryNavigationController.qml#L67) | animatePanelScrollTo |
| 14 | 22.85 | 23 | 0.994 | 0.158 | Javascript | [GalleryViewportController.qml:187](../../third_party/ZoinGallery/qml/GalleryViewportController.qml#L187) | animatePanelScrollTo |
| 15 | 22.38 | 23 | 0.973 | 0.154 | Javascript | [GalleryPanelMotion.qml:27](../../third_party/ZoinGallery/qml/GalleryPanelMotion.qml#L27) | restartScroll |
| 16 | 17.99 | 817 | 0.022 | 0.124 | Binding | [GalleryFallbackIconHost.qml:27](../../third_party/ZoinGallery/qml/GalleryFallbackIconHost.qml#L27) |  |
| 17 | 14.55 | 728 | 0.02 | 0.1 | Binding | [GalleryEntryDelegateBase.qml:40](../../third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml#L40) |  |
| 18 | 14.48 | 994 | 0.015 | 0.1 | Binding | [GalleryFallbackIconHost.qml:55](../../third_party/ZoinGallery/qml/GalleryFallbackIconHost.qml#L55) |  |
| 19 | 12.37 | 728 | 0.017 | 0.085 | Binding | [MasonryEntryDelegate.qml:47](../../third_party/ZoinGallery/qml/MasonryEntryDelegate.qml#L47) |  |
| 20 | 12.07 | 120 | 0.101 | 0.083 | Creating | [GalleryThumbnailShaderLayer.qml:31](../../third_party/ZoinGallery/qml/GalleryThumbnailShaderLayer.qml#L31) | ZoinGallery.Native/GalleryThumbnailImage |
| 21 | 11.61 | 56 | 0.207 | 0.08 | Javascript | [GalleryCursorController.qml:158](../../third_party/ZoinGallery/qml/GalleryCursorController.qml#L158) | updateVisualCursorForViewport |
| 22 | 11.54 | 425 | 0.027 | 0.08 | Binding | [GalleryEntryDelegateBase.qml:189](../../third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml#L189) |  |
| 23 | 10.85 | 156 | 0.07 | 0.075 | Creating | [GalleryEntryDelegateBase.qml:294](../../third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml#L294) | GalleryEntryPreview |
| 24 | 10.73 | 34 | 0.316 | 0.074 | HandlingSignal | [GalleryViewport.qml:354](../../third_party/ZoinGallery/qml/GalleryViewport.qml#L354) |  |
| 25 | 10.71 | 34 | 0.315 | 0.074 | Javascript | [GalleryViewport.qml:354](../../third_party/ZoinGallery/qml/GalleryViewport.qml#L354) | expression for onContentYChanged |
| 26 | 10.56 | 312 | 0.034 | 0.073 | Creating | [GalleryEntryPreview.qml:6](../../third_party/ZoinGallery/qml/GalleryEntryPreview.qml#L6) | ZoinGallery.Native/GallerySceneItem |
| 27 | 10.55 | 37 | 0.285 | 0.073 | Javascript | [GalleryPanelFacade.qml:175](../../third_party/ZoinGallery/qml/GalleryPanelFacade.qml#L175) | updateVisualCursorForViewport |
| 28 | 10.14 | 7902 | 0.001 | 0.07 | Binding | [GalleryEntryDelegateBase.qml:111](../../third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml#L111) |  |
| 29 | 9.42 | 912 | 0.01 | 0.065 | Binding | [GalleryEntryPreview.qml:18](../../third_party/ZoinGallery/qml/GalleryEntryPreview.qml#L18) |  |
| 30 | 8.98 | 1034 | 0.009 | 0.062 | Binding | [GalleryEntryPreview.qml:23](../../third_party/ZoinGallery/qml/GalleryEntryPreview.qml#L23) |  |

## Local captures

Capture directory `C:/Users/xs/AppData/Local/Temp/f4-page-profile-20261104`. The `baseline2` directory contains the light-probe repeat timings, `detail` contains native attribution and PresentMon, and `media-detail` contains delegate subphases and cache timings. The full Qt trace is `qmlprofiler-trace-f4-page-down-2026-11-04.qtd`. The cropped interval and parser output are `qml-repeat-window.qtd` and `qml-repeat-parsed.json`.

> AI assistance has been used to create this output.
