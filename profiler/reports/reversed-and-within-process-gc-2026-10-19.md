# GC budget crossover: reversed order and one persistent process

Date: 2026-10-19. Goal: constant 160 Hz without spikes. **Not achieved.**

## Reversed fresh-process comparison

A=5 ms, B=1 ms. Same reduced-allocation executable as the preceding comparison, fresh process order B1/A1/A2/B2/B3/A3. Downloads 186 entries, density 152, actual 160 Hz/DPR 1.75, continuous wheel events every 30 ms, two four-second blocks. Per-frame recorder, census, QML profiler, WPR, and UI Automation are disabled. No tests or builds overlap capture windows. All runs validate actual budget, active window, movement, and zero model/snapshot/caption updates. All runs are retained.

| Run | Block 0 p99 / max (ms) | >9.375 ms | Block 1 p99 / max (ms) | >9.375 ms |
| --- | ---: | ---: | ---: | ---: |
| B1 | 6.2974 / 12.3971 | 1 | 6.2896 / 12.4774 | 1 |
| A1 | 6.3200 / 12.5100 | 1 | 6.3379 / 12.5037 | 3 |
| A2 | 18.7454 / 25.0003 | 36 | 6.2831 / 12.4955 | 1 |
| B2 | 6.3225 / 12.5081 | 2 | 6.2854 / 12.4895 | 1 |
| B3 | 12.5166 / 18.7297 | 28 | 12.5028 / 18.8031 | 14 |
| A3 | 12.4822 / 18.7559 | 11 | 6.3204 / 12.4978 | 1 |

Reverse totals: A 53, B 47 long display intervals. B3 now contains 42 long intervals, while A2 has 37. Both budgets therefore exhibit burst outliers. Combined with the preceding order, fresh-process totals are 110 versus 60; those sums conceal large process/run variance and do not establish that a default change will remove the bursts.

GPU active-time distributions are broadly similar between these runs. The worst reverse B3 has wheel handler p99 2.4701 ms versus 0.93–1.66 ms in the other reverse cases. There are no model or snapshot updates. These are clues about broader work/throughput variation, not proof of CPU frequency, scheduling, GC, antivirus, or shader-cache causation. No such cause is assigned without a contemporaneous attribution capture.

## Same-process crossover

A diagnostic-only command changes MemoryManager::setGCTimeLimit on the GUI thread before a run. The original budget is retained for restoration. The host is launched once, given one prespecified warm-up, then ABBA followed by mirrored BAAB. The QML engine, process, GUI/render threads, and window dimensions remain identical. Screen placement/maximization occurs on the first request only, avoiding repeated resize work. Each new run resets the scroll position and receives two seconds of settling, as in the previous harness. No forced GC or heap reset is introduced; live GC state carries between windows.

The application environment remains at 5 ms while actual engine budgets switch between 1000/5000 us. Every result validates that actual budget and records requested budget, PID and engine identity. PresentMon rows must belong to that same PID. Each case receives a separate PresentMon session and exact native input windows. Both display endpoints must lie inside the window; internal stalls remain included. The warm-up is retained below but excluded from the prespecified A/B comparison.

| Run | Block 0 p99 / max (ms) | >9.375 ms | Block 1 p99 / max (ms) | >9.375 ms |
| --- | ---: | ---: | ---: | ---: |
| WARMUP | 6.3109 / 18.7445 | 2 | 6.2890 / 12.4946 | 1 |
| A1 | 6.3105 / 12.5495 | 1 | 6.3161 / 12.5814 | 2 |
| B1 | 6.3283 / 12.5017 | 1 | 6.2962 / 6.3260 | 0 |
| B2 | 6.3378 / 12.4966 | 1 | 6.3113 / 12.5019 | 1 |
| A2 | 6.2877 / 12.4984 | 1 | 6.2736 / 6.3984 | 0 |
| B3 | 6.2703 / 12.5075 | 1 | 6.2748 / 6.3484 | 0 |
| A3 | 6.2904 / 6.4045 | 0 | 6.3030 / 12.5002 | 1 |
| A4 | 6.3129 / 6.3795 | 0 | 6.2977 / 12.4814 | 2 |
| B4 | 6.2967 / 6.3600 | 0 | 6.3399 / 12.4927 | 1 |

Compared windows total A 7, B 5 long display intervals over 32 seconds of input per budget. The two-event difference is insufficient to justify a production default change. Both settings still miss refreshes. Warmed persistent-process results also must not substitute for the unresolved fresh-folder/fresh-process bursts.

Validated PID: 45432; QML engine: `15cff8`. GUI/render TIDs: 74040/2864; window: 2194 × 1186 logical units. The final settled folder extent is 4118.142857 logical units; the fresh-process initial extent was 4101.142857 before setup completed. These protocol/cache-state differences prevent treating the two series as a clean causal measurement of shader warming alone.

## Source findings and rejected conclusions

Qt 6.11.1 GCStateMachine explicitly assumes a 60 fps target for its default, taking one third of the integer frame period and yielding 5 ms. That explains why its marking slices can compete with a 6.25 ms frame. It does not prove that selecting 1 ms fixes all misses; atomic sweeping and other GUI/render/GPU work remain.

QQuickWindowIncubationController separately computes its base slice from the primary-screen refresh rate, and doubles it when render-loop incubation is not interleaved. Actual controller slices were not measured in this experiment. This is a source-level lead for the next warm/cold attribution, not an established cause of these outliers.

The current preview implementation already retains a created renderer through temporary inactive states and defers component selection. A blanket retention rewrite is not justified merely from the remaining allocation counts. Measure actual component replacements and native delegate pool misses before changing their lifetime rules.

## Decision and next work

Keep the 5 ms production default. Stop treating the GC constant as a demonstrated fix. The next attribution should cover remaining warm-process misses and the burst condition, timing incubation, native row preparation, and GC phases against actual displayed intervals. Continue to preserve the original scope: smooth first-use and continued scrolling, not only warmed benchmark windows.

Normal application sources, defaults, SDK libraries and payload are unchanged. The diagnostic live-budget setter is confined to artifacts. The normal worktree Qt app is restored with its original settings and a hash-matching embedded host; startup stderr is empty. Existing captures remain reproducible, because the new gcBudgetMs command defaults to zero (no override).

Compact captures, same-process identity manifest, analysis/reproduction tools and source snapshots are archived with SHA256 hashes under `profiler/experiments/reversed-and-within-process-gc-2026-10-19/`. Executables, libraries, raw CSV/ETL and build outputs are excluded.

> AI assistance has been used to create this output.
