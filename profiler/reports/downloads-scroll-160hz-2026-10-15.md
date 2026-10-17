# Downloads: scrolling at 160 Hz

All five panel modes miss the 6.25 ms frame budget. SVG scene construction produces the largest directly attributed stalls; Details additionally exceeds the budget in sustained rendering.

## Measurement

Live C:\Users\xs\Downloads: 182 entries (169 files, 12 directories and parent entry), including 11 SVGs, 72 JPG/PNG images and five MP4s. Display: 160 Hz, DPR 1.75, 3840×2063 physical window. Existing cache retained. The static left pane and application shell remain included in native render measurements.

Each mode used its saved density. The existing application scroll animation traversed the full range over four seconds. Headline statistics use complete warm downward sweeps, QML recording and verbose logging off. First traversal is not a cold-cache test. Some upward sweeps were interrupted by viewport restoration, so they are excluded consistently. QML traces were separate attribution passes. These are frame-swap/submission timings, not measured GPU execution or scanout.

## Warm scrolling frame intervals (ms)

| Mode | p50 | p95 | p99 | Worst | ≥1.5 refresh periods |
|---|---:|---:|---:|---:|---:|
| Details | 6.24 | 13.33 | 17.62 | 28.66 | 13.9% |
| Columns | 6.23 | 11.08 | 17.73 | 84.40 | 5.4% |
| Grid | 6.24 | 12.88 | 24.29 | 74.62 | 8.1% |
| Icons | 6.23 | 12.59 | 24.59 | 69.70 | 6.7% |
| Masonry | 6.23 | 11.69 | 18.95 | 68.99 | 8.2% |

The ≥9.375 ms threshold avoids classifying tiny timer jitter around 6.25 ms as a visibly missed refresh. Percentiles are per mode, not a pooled average.

## What blocks smooth scrolling

1. **Synchronous SVG scene construction.** A single VectorImage creation reached 12.5–20.1 ms across modes. Grid recorded a 74.96 ms frame containing 63.59 ms QML work, including 54.46 ms constructing vector scenes. Icons recorded 79.06 ms, with 51.42 ms vector construction. Details recorded 30.12 ms, with 18.98 ms vector creation. Source caching does not cache the generated scene. Keep vector rendering, but prepare and retain a bounded set of nearby surfaces by revision.

2. **Entering-item and render-batch churn.** A scrolling trace created 278–560 fallback icon leaves per mode. Columns can instantiate a whole entering column together: its 86.37 ms spike included 55.62 ms QML and 16.87 ms render submission CPU. Separate debug captures showed whole-window medians of approximately 271–433 render batches and frequent full batch rebuilds. Debug logging distorts timing, so only counts from that pass are used. Preserve compatible leaves and investigate redundant clip/batch boundaries rather than adding more caches indiscriminately.

3. **Details sustained rendering.** Warm downward p95 render submission CPU was 7.20 ms, already above the entire 6.25 ms budget; p95 sync was only 0.13 ms. Merely switching the text backend did not remove this bottleneck. Other modes had p95 render CPU around 3.4–4.2 ms. CPU sampling found 11.59% of sampled Qt-host CPU in the NVIDIA user driver and 2.33% in D3D11; static Qt/application code is unresolved within the executable (55.28%). These samples include setup and all host threads, so they do not prove a specific GPU or Qt function bottleneck.

4. **Smaller repeated binding costs.** Exclusive QML averages during scrolling were Details 0.759, Columns 0.617, Grid 0.757, Icons 1.262 and Masonry 0.894 ms/frame. Icons attribution ran alongside ETL analysis and may be inflated. Scrollbars account for roughly 0.13–0.23 ms/frame; residual alignment and viewport notifications add work. Columns evaluated iconSceneOrigin 52,771 times even though its expensive alignment walk was suspended. These are secondary targets, not explanations for 70–80 ms spikes.

**The caption cache is working:** the warm Grid sweep performed zero caption measurements. Expanding caption caching is not the first fix for this workload.

## Priority

First address vector scene preparation/reuse and amortize entering rows/columns. Then reduce rendering/batch churn, especially in Details. Finally trim repeated bindings. Reprofile all five modes after each change, retaining the 175% resting pixel-grid checks and direct SVG rendering. No product code was changed during this investigation.

## Evidence and per-mode detail

- [Details](downloads-details-2026-10-15.md)
- [Columns](downloads-columns-2026-10-15.md)
- [Grid](downloads-grid-2026-10-15.md)
- [Icons](downloads-icons-2026-10-15.md)
- [Masonry](downloads-masonry-2026-10-15.md)

Raw traces, screenshots, native timelines and analysis JSON: [artifacts](D:/Code/f4-zoin/artifacts/modes-profile). Windows CPU capture: native-cpu.etl; module attribution: native-module-summary.json; renderer counts: render-batches.json.

QML inclusive totals include nested calls and mode setup; use isolated exclusive costs for scrolling. Masonry trace alignment has approximately one-frame uncertainty, so individual spike attribution is limited. WPR and profiler recording were stopped after capture; the app was restored to its normal portable host.

> AI assistance has been used to create this output.