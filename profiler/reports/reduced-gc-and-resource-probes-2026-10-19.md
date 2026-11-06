# Reduced-allocation GC budgets and render-resource churn

Date: 2026-10-19. Constant 160 Hz without spikes remains unachieved.

## Budget-only comparison

Current valid-index/scalar-preview build, same executable for both budgets. A=5 ms, B=1 ms, fresh process order A1/B1/B2/A2/A3/B3. Downloads 186 entries, density 152, actual 160 Hz/DPR 1.75, wheel 120 every 30 ms, continuous boundary bounce, two four-second blocks. Per-frame recorder, census, WPR, QML profiler, and UI Automation are disabled. No build or tests overlap clean captures. Every run validates actual budget, active window, movement and zero caption measurements. Both display endpoints must lie inside the input window. All six runs are retained.

| Run | Block 0 p99 / max (ms) | >9.375 ms | Block 1 p99 / max (ms) | >9.375 ms |
| --- | ---: | ---: | ---: | ---: |
| A1 | 6.3328 / 12.5095 | 3 | 6.2881 / 12.5039 | 2 |
| B1 | 6.2921 / 12.4884 | 1 | 6.3192 / 12.4256 | 1 |
| B2 | 6.2787 / 12.5095 | 2 | 6.3111 / 12.4946 | 1 |
| A2 | 6.3760 / 12.5042 | 3 | 6.3200 / 12.4967 | 1 |
| A3 | 12.5008 / 24.9530 | 15 | 12.5216 / 18.7563 | 33 |
| B3 | 6.3877 / 12.5035 | 2 | 6.4463 / 18.7681 | 6 |

All blocks: A 57, B 13 long display intervals. A3 accounts for 48 of the 57 control intervals and dominates the difference. It is not discarded; the trace does not establish why that clean run was unusually slow. B has fewer misses in each matched pair, but before changing the default, repeat with reversed ordering and examine the remaining misses. The original 5 ms production setting remains unchanged.

## Observer validity

The following are separate diagnostic captures, using clocks/counters around original calls. They are not clean before/after frame benchmarks. They retain original rendering behavior, snapshots, binding updates, and resource lifetime. The SDK installation and normal portable host are unchanged; alternate libraries and hosts live only in artifacts. No /FORCE symbol replacement or class-layout change is used.

The Quick observer records cumulative render-thread counters at render exit, including releases before render. Cumulative differences bracket each input window with existing snapshots and can include a partial boundary frame. There is one observed renderer, four layout classes, no layout overflow, and fewer than 32768 events. The D3D observer uses complete command-buffer cycles wholly inside each input window; count equals cleared snapshot count for every selected cycle. There is one observed command buffer and no state/event-cap overflow. Original 5 ms GC budget is used.

## SRB pool pressure

The existing layout-compatible QMultiHash pool is not capacity constrained in this capture. Its maximum stock is 188, below the default capacity 1024.

| Block | Reuse attempts | Hits | Misses / new bindings | Evictions | New object time (ms) | Bake time (ms) |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 0 | 2879 | 2463 | 416 | 0 | 1.5190 | 0.2168 |
| 1 | 2417 | 2417 | 0 | 0 | 0.0000 | 0.0000 |

The two blocks perform 819,097 material updates whose resource bindings are unchanged. There are zero new SRBs in the second block. Raising the pool threshold is not supported by these measurements; creation/baking is too small in aggregate to explain the recurring stalls.

## D3D command snapshots and backend rebuilds

| Block | Cycles | Retained snapshots | Retain mean (ms/cycle) | Reset mean (ms/cycle) | Binding rebuilds | Rebuild total (ms) | Pipeline-only rebuilds |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 0 | 638 | 409740 | 0.2223 | 0.1214 | 12125 | 8.2002 | 0 |
| 1 | 638 | 435080 | 0.2242 | 0.1121 | 11520 | 7.6620 | 0 |

Retain plus reset averages approximately 0.34 ms per cycle. Reset includes all command-buffer retained data pools and cached-state reset, not solely SRB destruction. Per-call timestamp overhead is included in these diagnostic timings, so a proposed optimization cannot claim the whole amount as recoverable. Snapshot copies contain implicitly shared vectors; the data does not prove deep copies.

All observed backend rebuilds follow resource changes. There are zero pipeline-only rebuilds and zero same-pipeline redundant candidates. Their total duration is approximately 15.86 ms across eight seconds. The hypothesis that the backend repeatedly rebuilds unchanged bindings after pipeline switches is not supported. No caching shortcut or lifetime change is applied.

## Next work and rollout

1. Repeat the reduced-allocation GC budget test with reversed run order. If the benefit persists, use native phase attribution to separate marking improvements from the remaining atomic sweep limit.
2. Measure the unchanged-material update path itself before touching render-resource policy. The pool and backend rebuild counters rule out two tempting tuning targets. Snapshot retention is a smaller opportunity and requires immutable command semantics to be preserved.

Normal sources, defaults, payload, and SDK are unchanged in this experiment. The normal worktree app is restored; the archive verifies direct --gui=qt launch, the cached host hash, and empty startup stderr. Five accounting regressions pass. A probe relink initially encountered LNK1104 because the earlier probe executable was running; it was rebuilt under a distinct filename while that working instance stayed alive. The normal application was unaffected.

Compact captures, analysis, diagnostic sources, source patches and SHA256 manifest are under `profiler/experiments/reduced-gc-and-resource-probes-2026-10-19/`. Binaries, libraries, raw CSV/ETL and build outputs are excluded.

> AI assistance has been used to create this output.
