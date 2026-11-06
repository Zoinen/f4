# Masonry: complete incubation callback timing

Status: attribution progress, not a performance fix or full acceptance.

The previous goal turn made progress by matching exact binary symbols and independently verifying profile samples. This experiment measures full event delivery to the existing incubation controller and QML engine without changing controller, timer, priority or GC settings. It uses the same initialized empty-folder baseline, public navigation action, Downloads dataset (192 rows), Masonry density 152 and 30-second wheel input.

## Build and scope

Accepted case: `cold-k12`, host SHA-256 `e9bf5c874d6f1cffdc8a43f40c657a18b1e7bc4d1734d82ea6083b5a6a0a714d`.
Raw evidence: `C:/Users/xs/AppData/Local/Temp/f4-current-cold-motion-20261031/cold-k12/`.
The diagnostic subclass times Timer/MetaCall delivery for the exact runtime class names `QQuickWindowIncubationController` and `QQmlApplicationEngine`, using the existing bounded native census. It does not identify which slot every engine MetaCall invokes.

The source compile path and both diagnostic header inclusions were verified. The static Windows import audit and GUI-subsystem header check pass. This is an explicit diagnostic host override, not a rebuilt embedded production payload. No product layout changed. The normal app remains alive.

Case `cold-k11` is rejected: an adapted build script compiled the old GC-observer main rather than its new copied main, so the entry gate and callback probe were absent. The same live instance was inspected before closing its exact diagnostic process pair. The invalid binary, inputs and rejection receipt are retained. V2 asserts the actual `-c` source path; this invalid attempt provides no timing evidence.

## Measurements

The accepted capture verifies 160 Hz, DPR 1.75, initialized baseline and an empty pre-navigation cache. At first wheel input nine frames were cached and zero decode requests pending; more work arrived afterward. This is not an OS-cold file-cache claim or a matched-cache comparison to j13. A bounded temporary 160 Hz mode was used; the original 60 Hz mode was restored afterward without a registry change.

| Full callback | Count ending after input start | Median | Maximum |
| --- | ---: | ---: | ---: |
| Incubation controller event | 4 | 0.5870 ms | 4.0151 ms |
| QML engine event | 34 | 4.8166 ms | 6.2910 ms |

The incubation callback at +13.5855 through +17.6006 ms contains eight complete delegate commits; their largest is 0.4630 ms. Another just before input takes 2.2506 ms and contains ten commits; one at +29.1295 ms takes 0.9617 ms and contains six. Commit counts include contained layouts. They are nested observations and must not be added as exclusive callback costs. The parser uses phase-probe commit kind 34; catalog-update kind 7 is a different scope and cannot substitute for it.

Two display gaps exceed 9.375 ms: 18.7490 and 18.7467 ms. Their presentation intervals are approximately -2.6701 to +16.7981 ms and +16.7981 to +32.1558 ms. The 4.0151-ms callback overlaps both. This is temporal overlap, not proof that shortening it alone removes either miss. There are zero dropped rows and zero coverage holds. No physical motion error samples are admitted by the current wheel analyzer; its 4792 excluded pairs remain recorded.

Qt 6.11.1 initializes this controller's budget to one third of a frame, with an integer minimum, and the non-interleaved branch requests twice that duration. See the [tagged Qt source](https://github.com/qt/qtdeclarative/blob/v6.11.1/src/quick/items/qquickwindow.cpp#L71-L125). This source describes the requested budget, not a hard runtime limit: synchronous completion can exceed it. The actual private budget field was not sampled, so its precise value in this process is not claimed.

## Next action

Measure publication and nearby-preparation spans against these complete callback intervals, then evaluate a bounded scheduling change with visible-preview latency and viewport-coverage checks. Lowering the incubation timer cannot preempt synchronous QML finalization. QML engine callbacks also consume a large share of a 6.25-ms frame and need slot-level attribution before changing their scheduling. Retain startup-boundary misses and the complete cold/warm wheel and middle-scroll requirements; constant 160 Hz remains unachieved.
