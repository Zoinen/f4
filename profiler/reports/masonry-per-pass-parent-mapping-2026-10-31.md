# Per-pass parent transform sharing, 2026-10-31

## Decision

Retain this diagnostic candidate for further validation. It removes repeated
ancestor mapping and reduces the median measured membership stage by about
28% in one same-host A/B pair. It does not achieve constant 160 Hz: the
candidate has one over-budget display interval in each window, and both
baseline and candidate fail the unchanged 2-pixel maximum-motion gate.
Do not enable it in production based on this pair alone.

The preceding goal turn is progress: it added fatal-only recording, verified
actual backend selection and completed two full D3D12 repeats, ruling out a
backend switch as a sufficient solution. This turn returns to the normal
D3D11 renderer and implements/tests an app-owned synchronization optimization.

## Why this target

The corrected-host warm packet still spends about 0.15 ms median discovering
and mapping retained scene membership per synchronization. Its typed registry
has already removed repeated QObject discovery, but every delegate still
calls `mapRectToItem` through its ancestor chain. Many delegates share a parent.
This is a recurring CPU opportunity, not an attribution of all refresh misses.

`ParentMappingScope20261031.h` borrows geometry only during one GUI-locked
synchronization pass. Translation-only leaves share one parent-to-target
transform per parent, with 32 stack slots and a complete Qt fallback on
capacity exhaustion. Direct children need only their local translation.
Rotated/scaled leaves, custom transforms, null targets, unsupported mappings
and cross-window mappings retain Qt's full path. This avoids incorrectly
composing projected 2D transforms after a leaf introduces Z.

Nothing survives the pass: no retained QSG pointer, geometry revision,
reparent subscription or invalidation heuristic is introduced. Layout and
pixel-fitting arithmetic are unchanged. No new rounding or raster layer is
added. All scene/source families and retained previews remain in scope.

## Correctness and rendered output

- The 150-sibling full-chain baseline fails the explicit repeated-mapping
  budget; the candidate needs one parent mapping. All six helper cases pass,
  including transformed ancestors, custom 3D leaf fallback, geometry changes,
  reparenting, direct-parent mapping and the bounded-storage fallback.
- 46 native renderer cases and seven motion cases pass on actual D3D11 at
  DPR 1.75. All 25 native and five motion PNGs are byte-identical to the
  corrected-host baseline. Existing scene-space leaf-origin/unit-vector checks
  remain unchanged. The small-caption/thin-stroke capture
  `native/shared-text-1-2-175.png` was also inspected; its secondary text remains
  sharp.
- The unchanged resource graph's isolated-import check passes three cases;
  the separately named static candidate host passes the system-DLL audit.
- A dedicated full-folder V1 run evaluates the candidate against Qt for every
  fast mapping: **698,364 comparisons, zero mismatches**. It performs 64,428
  full parent mappings across 4,956 scopes. Its mapping and registry oracles
  add work, so its timing is excluded from the performance comparison.

## Live comparison

Actual Downloads/192 entries, Masonry density 152, ordinary window, 3840x2160,
160 Hz / DPR 1.75, four decode workers. Every fresh process completes a
normal-visibility tour of indices 0..191 before two 30-second wheel windows.
The same candidate binary runs A1 with sharing disabled, then B1 enabled.
Both performance runs disable duplicate oracle evaluation. Foreground stays
stable, observer overflows and registry mismatches are zero, no measured
thumbnail cache misses occur and coverage holds remain zero.

| Scope/window | Membership median / p99 (ms) | Total postSync median (ms) | Preparation p99 (ms) | Display max (ms) | Intervals >9.375 ms | Motion max (px) |
|---|---:|---:|---:|---:|---:|---:|
| A1/0, full mapping | .1671 / .3319 | .19235 | 1.7054 | 6.4608 | 0 | 3.99061 |
| A1/1, full mapping | .1718 / .4219 | .19730 | 1.7870 | 6.5083 | 0 | 3.99282 |
| B1/0, shared parents | .1203 / .3928 | .14720 | 1.7513 | 12.4616 | 1 | 62.05837 |
| B1/1, shared parents | .1211 / .3982 | .14830 | 1.8221 | 12.5173 | 1 | 25.54859 |

Median membership falls 28.0% / 29.5%; median total postSync falls 23.5% /
24.8%. Median observed delegate count is 155 in A1 versus 154 in B1, and
recorded input counts differ slightly. This is a sequential exploratory pair,
not a replicated causal proof or an end-to-end smoothness improvement. The
first-window membership p99 is worse in B1, preparation p99 is not lower,
and strict full performance fails in every window. All outliers and missing
displayed-frame pairs remain under the existing analyzer's rules.

The normal renderer's other optimizations, queue depth 2 and original
9.375 ms / 2 px / zero-hold gates remain unchanged. No background workload,
external affinity, display setting or normal-window behavior is changed.

## Scope, evidence and remaining work

Only diagnostic sources, build/capture helpers and evidence change. The
installed Qt SDK, product sources/defaults and embedded payload remain
unchanged; this is not a portable rollout. No commit or push is made. Both
owned diagnostic process trees and the V1 verification tree are stopped;
normal Go PID 23488 and Qt PID 84780 remain alive.

Raw packets are retained with verified hashes and transparent compression.
Completed owned build/trace files were compressed with before/after hash
checks to recover disk space; only verified raw duplicates were retired.
The evidence index beside this report preserves exact sources, receipts,
the intentionally failed baseline, geometry-oracle counters, strict failures
and raw locations.

Next steps are replication of component-cost behavior, then the original
cold and quarter-height middle workloads before considering source integration.
The remaining presentation/motion failures still require separate work;
this reduction alone is not a solution to them. The full goal remains active.

AI assistance has been used to create this output.
