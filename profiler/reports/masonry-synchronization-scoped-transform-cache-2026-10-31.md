# Masonry: synchronization-scoped target transform cache

## Outcome

Constant 160 Hz remains unachieved. A direct probe confirms scene discovery and
mapping take about 90% of `ScrollJob::postSync`, but caching just the repeated
target transform does not improve whole-frame delivery. The cold A/B/B/A
candidate has more displayed-frame misses, especially on the return direction.
Keep this implementation experimental and disabled by default. Do not promote
it based on its smaller transform-query count or correctness tests.

The previous goal turn made progress by attributing synchronization CPU samples.
This turn measures that path directly, implements a bounded-lifetime cache,
passes geometry and native regressions, checks the live result against Qt's
original mapping, and rejects a production rollout after the performance test.
No product source, installed Qt archive, embedded payload, normal app, commit or
push changes.

## Direct attribution

The diagnostic host adds a fixed-capacity 32768-record probe with five timestamps
per synchronization pass and local counters for delegates, SVG discovery visits,
gates and mappings. Both enabled and disabled helper checks pass, including two
records beyond capacity. The timestamp-only enabled test averages 133.62 ns per
scope; this excludes real scene traversal and its per-item counters and is not a
claim of zero observer overhead in the application.

The first cold Downloads capture has 782 / 806 measured synchronization passes:

| Phase | First direction median / p99 / max (ms) | Return direction median / p99 / max (ms) |
| --- | --- | --- |
| Entire `postSync` | 0.2614 / 0.7515 / 1.3249 | 0.2625 / 0.7273 / 2.1352 |
| Scene membership and mapping | 0.2340 / 0.6698 / 1.1796 | 0.2359 / 0.6420 / 1.9887 |
| Surface and scrollbar setup | 0.0177 / 0.0559 / 0.1445 | 0.0176 / 0.0632 / 0.1471 |
| Coverage | 0.0075 / 0.0230 / 0.0521 | 0.0074 / 0.0240 / 0.0502 |

Motion-state setup has a 0.0004 ms median. Membership accounts for about 90% of
summed `postSync` time. Each pass typically enumerates 152 / 154 delegates and
317 SVG discovery items, finds 11 gates, and performs 138 / 139 rectangle maps.
The worst entire sync is 8.2848 ms while that pass's `postSync` is 1.9391 ms;
this function does not explain every synchronization spike.

The source of repeated work is concrete: Qt 6.11.1 `mapRectToItem` calculates
both the source item-to-window transform and the target window-to-item transform
on every call. Every delegate and gate in this loop uses the same target. The
new diagnostic mapper calculates the target inverse once immediately before the
membership loop. It retains Qt's source-to-window calculation, transform
multiplication order and `mapRect` implementation. It never persists across
`postSync`, so changed target ancestors are naturally refreshed on the next pass.
This experiment does not cache delegate membership or SVG child discovery.

## Correctness and build checks

The 1000-mapping regression first fails with caching disabled: 1000 target queries
against a budget of one. It passes with caching enabled. Four QtTest results pass
at actual DPR 1.75, checking exact rectangle equality through 240 rounds of
ancestor, target and source changes, rotation, scale, singular transforms, a
custom 3D transform, cross-branch mapping, null targets, and source-only mutation
inside a valid pass. The mapper uses the same Qt path even for singular cases;
it does not replace that behavior with a new inverse convention.

The diagnostic header includes counters and a separate verification mode. A
live cold capture checks **229245 mappings** against the original Qt function
with **zero mismatches**. Its verification work is excluded from the performance
comparison. The runtime flag is `F4_PROFILE_SYNC_TARGET_CACHE`; default is off.
`F4_PROFILE_SYNC_TARGET_VERIFY` enables the live oracle. Both are diagnostic flags.

The same candidate/control host also passes:

- 46 native renderer results in each arm, no failure or skip.
- 25 byte-identical PNG files: 23 rendered captures and two fixture inputs.
  The inherited-transform and small-caption/thin-stroke captures were inspected.
  The existing 175% leaf-origin and effective-transform assertions remain.
- Three installed-tree-independent QML import results.
- The Windows system-DLL dependency audit.

The host is `artifacts/f4-qt-host-sync-target-cache-20261031.exe`, SHA256
`4fa41771ab5f7ac8a8e2db0fb16d2dca55967b82e7c6ad400862fe34d7bc2745`.
It is a worktree diagnostic host, not a new portable release. The normal
worktree app remains running with its existing embedded host.

## Cold A/B/B/A

All four arms use the same host and pinned Go launcher, fresh isolated gallery
caches, real `C:\Users\xs\Downloads`, 192 entries, Masonry density 152, actual
DPR 1.75 and 160 Hz. Both directions are cold-normal-viewport workload blocks;
the reverse direction is **not** a verified warm tour. Shared ancestry, retained
geometric storage and inherited-transform skipping are enabled in both arms;
asynchronous buffer prewarming is off. The target cache is the differing flag.

No screenshot, accessibility snapshot, build or offline analysis runs during a
gesture. Native foreground observations verify each measured interval. Every
wheel event is active; input counts are 1779 / 1729 / 1761 / 1810. Foreground
observers have no overflow and do not write during capture. The preparation
observer's whole-capture maximum is 63.9 / 94.4 / 50.3 / 52.4 microseconds.
Those are instrumentation limits, not precise locations of every observer cost.

| Arm | `postSync` median, directions 0 / 1 (ms) | `postSync` p99, directions 0 / 1 (ms) | Display max, directions 0 / 1 (ms) | Misses, directions 0 / 1 |
| --- | --- | --- | --- | --- |
| A1, original | 0.2508 / 0.2225 | 0.7287 / 0.4914 | 12.5606 / 12.5050 | 19 / 2 |
| B1, cached | 0.2358 / 0.2192 | 0.7662 / 0.6985 | 18.7231 / 12.7392 | 35 / 52 |
| B2, cached | 0.2305 / 0.2242 | 0.6763 / 0.7595 | 18.6132 / 12.8259 | 41 / 53 |
| A2, original | 0.2496 / 0.2150 | 0.7940 / 0.4127 | 12.8204 / 12.0432 | 31 / 1 |

Cached target queries fall from roughly 230000 per capture to 1596 / 1601 for
223950 / 225125 mappings, about one query per 140 maps. First-direction median
`postSync` is modestly lower; the return-direction median and tails show no
consistent benefit. The reduction in repeated queries is not enough to infer
better scrolling. The repeated worse return-direction results oppose promotion;
this experiment does not establish the precise causal mechanism behind them.

The original unchanged thresholds remain: displayed intervals no larger than
9.375 ms, maximum motion error no larger than two physical pixels, no coverage
holds, and the original middle-scroll velocity gate. All cold arms fail display
and motion limits. Motion maxima are A1 117.52 / 87.70, B1 195.62 / 127.45,
B2 129.81 / 125.07 and A2 126.83 / 34.00 physical pixels. Coverage holds are
zero in all blocks. No verified-warm or middle-scroll performance claim is made
for this rejected candidate. Those remain required for full-goal acceptance.

## Next target and preserved evidence

Avoid expanding this inverse-only cache or claiming it solves synchronization.
The new counters show the remaining loop enumerates around 150 retained
delegates and around 300 SVG descendant items on each pass. Next investigate a
bounded membership registry or generation snapshot that refreshes for actual
delegate/Loader/source/scene changes, and measure dirty-node churn separately.
Any cached geometry must retain all relevant transform and ancestor revisions;
raw QSG node pointers cannot survive scene invalidation. The independent direct
traversal oracle, live comparison and original end-to-end gates remain necessary.

Compact sources, native results, captures, counter summaries, live results and
hash receipts are preserved in
[the experiment manifest](../experiments/masonry-synchronization-scoped-transform-cache-2026-10-31/manifest.json).
Raw packets remain hash-verified on C:. Five old diagnostic directories were
copied and hash-verified there, then replaced with checked junctions to recover
workspace disk space; original paths remain usable. Only exact owned diagnostic
process trees and display holds were stopped. Normal Go PID 23488 and Qt PID
84780 remained alive. No acceptance threshold, image quality or SVG rendering
mode changed.

> AI assistance has been used to create this output.
