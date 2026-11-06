# Masonry: disable competing buffer prewarming

## Outcome

Constant 160 Hz remains unachieved. A same-host control disables only the
experimental asynchronous D3D11 buffer-prewarming pool. The cold scheduling
capture no longer contains a comparable long preparation wait; its worst
preparation frames are scheduled CPU work. A subsequent verified-warm delivery
test still fails the original displayed-interval and motion requirements.
Keep prewarming disabled in subsequent controls; do not promote it into the
production renderer on the strength of earlier pool-hit counters.

The preceding goal turn made progress by identifying the render thread's
3.316 ms driver wait and the prewarming worker that readied it. This turn
intervenes on that mechanism, validates the remaining CPU work, and returns
to actual delivery measurements. It changes diagnostic configurations only.
The normal application, production source, installed Qt archives and embedded
portable payload remain unchanged. No commit or push was made.

## Controlled cold scheduling capture

Case `real-scroll-nopool-b1-20261031-wheel-masonry` retains the same host SHA256
`631b87d7976664b5f55b3400d0ef23cc48576a58b85d7aa788000b8f4c608a11`, renderer
options, frame latency 2, actual Downloads with 192 entries including parent,
Masonry density 152, 3840×2160 output, 160 Hz and actual DPR 1.75. Only
`F4_PROFILE_ASYNC_GEOMETRY` is absent. Counters verify the pool is disabled and
has zero device records. A fresh isolated gallery cache is empty at launch.
The two ten-second wheel windows are a cold/loading test; the second is not
claimed to be verified warm.

There are 1749 wheel inputs and zero inactive inputs. Native foreground
ownership passes. Before input, read-only provider enumeration confirms one
narrow Dwm-Core logger (`0x89`, all-keyword zero). PresentMon is absent. Kernel
and graphics traces are intrusive attribution evidence, not display acceptance.
No screenshots, UI Automation queries, builds or offline analysis run during
the gestures. Event/buffer loss, schema/property errors and scheduler unknown
time are zero. Present/token/surface linking retains the existing strict
single-chain and consecutive-count guards.

| Window | Preparation median / p95 / p99 / max, pool ON (ms) | Pool OFF (ms) |
| --- | --- | --- |
| 0 | 0.6646 / 2.2954 / 4.2115 / 10.1466 | 0.64795 / 2.1193 / 3.2733 / 6.0617 |
| 1 | 0.61385 / 2.1346 / 3.8539 / 6.2519 | 0.5878 / 1.9644 / 2.7328 / 5.1898 |

This is one consecutive pair with different loading/membership churn, not a
replicated estimate of a performance improvement. The stack evidence establishes
that prewarming can contend with rendering; this control supports testing
without that pool. It does not establish that all differences are caused by it.

The pool-off worst preparation is 6.0617 ms: 6.0576 ms scheduled, 0.0041 ms
runnable, zero blocked and zero unknown. Its upload phase is 1.7711 ms,
render-list phase 1.3657 ms, before-body phase 1.2825 ms, alpha-material phase
0.827 ms. The second window's worst is 5.1898 ms, with 5.1688 ms scheduled,
0.021 ms runnable, zero blocked and zero unknown.

Across all 3201 measured frames, preparation accounts for 2919.879 ms wall:
2916.836 ms scheduled, 1.903 ms runnable, 1.140 ms blocked, zero unknown.
These are summed durations, not capture wall time. Phase sums include:

| Phase | Scheduled total (ms) |
| --- | ---: |
| Alpha materials | 1022.183 |
| Before body / scene preprocessing | 604.277 |
| Geometry uploads | 596.669 |
| Render lists | 304.548 |

## Sampled CPU attribution

The existing kernel trace is decoded after input completes. The raw CPU-sample
prefix fallback is independently checked against xperf: all 454 reference
samples match. The render thread has 2790 samples in preparation; one lacks
a matching stack and remains explicitly unjoined. App symbols use the exact
code/unwind-equivalent map relink from the preceding investigation. ICF aliases
remain identified. Sample counts indicate hot paths, not exact millisecond
estimates or inclusive percentages.

Highest exclusive app sample counts include scene-node traversal (188),
material dynamic-data updates (141), merged-batch preparation (87), the
experimental shared-uniform lookup (82), pipeline-state-key construction (78),
transform-node updates (59) and buffer unmapping (59). Upload samples also enter
ntdll, D3D11 and NVIDIA code. Thus the remaining work is distributed; the trace
does not justify attributing it all to source decoding, captions, or one cache.

## Verified-warm delivery test

Case `real-scroll-warmpooloffr2-b1-20261031-wheel-masonry` uses the same host and
renderer configuration with prewarming off, without kernel or custom graphics
tracing. PresentMon supplies displayed intervals. The normal full-folder tour
visits all 192 indices at 19 waypoints in 15445.0977 ms. Every waypoint has no
pending/unready previews, ready plus definitive failures equals expected,
normal visibility is preserved, and stability lasts at least 500 ms followed
by a swap. Input delivers 1833 wheel events with zero inactive inputs.

Original limits are unchanged: zero displayed intervals over 9.375 ms, maximum
motion error at most 2 physical pixels and zero coverage holds. This test fails:

| Window | Preparation median / p95 / p99 / max (ms) | Display interval p95 / p99 / max (ms) | Long intervals | Max motion error (physical px) |
| --- | --- | --- | ---: | ---: |
| 0 | 0.44945 / 1.5403 / 1.8751 / 7.5752 | 6.2671 / 6.3324 / 12.4297 | 3 | 4.87178 |
| 1 | 0.44025 / 1.5187 / 1.8701 / 2.9587 | 6.2653 / 6.3022 / 6.6062 | 0 | 6.19028 |

The 7.5752 ms preparation spike contains alpha-material work (3.5506 ms) and
uploads (2.3748 ms). The shared-uniform pass reports zero new buffers and zero
SRB rebakes in that frame. Without scheduler tracing, these wall durations do
not distinguish execution, preemption and driver waits; do not invent that
attribution. The full-folder warm tour does not eliminate all subsequent scene
membership changes or resource work.

The worst motion pairs have actual display deltas 6.54279975 and 6.6062 ms,
while the native animation advances 6.25 ms. The maximum error against the
native curve itself is only 0.001313 physical pixels. This preserves the
distinction between correctly applying a curve and matching motion to actual
display timing. Neither fact relaxes the original 2-pixel delivery gate.

## Remaining work

The next renderer investigation must attribute the remaining sporadic warm
material/upload spike with scheduling evidence, keeping prewarming off. CPU
optimization candidates must be measured against the distributed phase costs;
optimizing another cache without reducing those costs is not sufficient.
Wheel display-time motion errors remain a separate open requirement. Original
cold wheel, verified-warm wheel/tour and quarter-height middle-scroll gates all
remain required; this turn does not rerun or claim the full middle-scroll gate.

Evidence is frozen under
`profiler/experiments/masonry-buffer-prewarming-control-2026-10-31`.
Both diagnostic process trees and display holds are stopped, all owned tracing
sessions are absent, and the normal worktree application remains alive.

AI assistance has been used to create this output.
