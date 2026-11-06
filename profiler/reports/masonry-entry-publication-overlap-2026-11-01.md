# Masonry: model delivery versus delegate creation during first scroll

Status: attribution progress. Constant 160 Hz remains unachieved; no product scheduling change is promoted.

## Experiment

Case `cold-k13` extends the preceding callback probe with full Timer/MetaCall delivery timing for runtime class `ZoinGallery::ExternalCatalogModel`. Existing controller and QML engine timing remains. No signal delivery, timer, thread priority, GC budget or public navigation behavior changes. The host is `9a74cacc3bb67bfd7027508b409f026c3e68735e00fc2c1d139dc30fcdb0cbd9`. The actual copied main source and diagnostic header inclusion were checked before launch; static Windows DLL and GUI-subsystem checks passed.

The initialized-app gate verifies an empty cache before navigation from two empty panels to Downloads (192 rows), Masonry density 152, actual DPR 1.75 and 160 Hz. At input start 19 frames are cached and 18 requests pending. This is an instrumented attribution capture, not a matched-cache performance comparison or OS-cold read test. A bounded temporary refresh-mode lease was released and the prior 60 Hz setting restored. The normal app remains alive; this diagnostic host is not an embedded production rollout.

Raw evidence: `C:/Users/xs/AppData/Local/Temp/f4-current-cold-motion-20261031/cold-k13/`.

## Results

There are 4,552 complete model events ending after input start: median 0.0041 ms, maximum 0.5448 ms. Incubation-controller events have median 4.21105 ms and maximum 4.3093 ms; QML engine events have median 4.8563 ms and maximum 8.4617 ms. Class-level engine events do not identify their exact slot.

Two display gaps exceed 9.375 ms, at 18.7537 and 24.9804 ms. There are no dropped rows or viewport coverage holds. The first gap crosses input start and remains included. Wheel physical-motion verification remains unproven (zero admitted physical-error samples; 4,792 excluded pairs).

| Presentation interval relative to input | Display gap | Model-event union | Incubation-event union | Delegate-commit union | Nearby-preparation union |
| --- | ---: | ---: | ---: | ---: | ---: |
| −6.2058 to +12.8796 ms | 18.7537 ms | 0.0446 ms | 1.9430 ms | 1.3257 ms | 1.8309 ms |
| +12.8796 to +31.3055 ms | 24.9804 ms | 0.0236 ms | 6.4791 ms | 7.3809 ms | 2.0509 ms |

These are clipped wall-time unions within each kind, not exclusive CPU categories. Nested categories overlap and **must not be summed**. The union helper has five passing tests covering nested spans, clipping, disjoint/touching spans, empty boundaries and reversed windows. The full census has zero overflow.

A visible delegate commit starts at +29.2620 ms and lasts 7.0685 ms, of which 6.5495 ms is the preparation phase. It crosses the second presentation interval's end. Another at +18.2413 ms takes 3.4514 ms, including 1.5443 ms preparation, 0.7838 ms visual binding and 0.7227 ms layout. These main-layout commits overlap distinct incubation work; no claim is made that a timer callback alone explains an entire missed display interval.

## Decision

This capture does not support model-notification batching as the next fix: measured model delivery occupies very little of the misses. It strengthens the case for reducing synchronous delegate preparation/creation and its competition with incubation. The preparation phase includes existing-row lookup as well as acquiring new/recycled delegates; it is not assigned wholly to allocation or GC without a narrower measurement.

Next: inspect and measure delegate acquisition, reuse and synchronous component finalization inside that preparation phase. Preserve initial visible coverage, folder previews, all formats, prefetch limits and preview readiness. Evaluate any candidate on both cold first scroll and the full warm tour/wheel/middle-scroll suite before claiming a cadence improvement.
