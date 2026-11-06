# Current remaining scroll stalls — 2026-10-18

## Scope and evidence quality

Current gallery source includes empty-header notification suppression, native/identity-guarded cursor intersection, and scalar cached icon alignment. Both diagnostic hosts use exactly this source. A uses the normal buffer-fit QtQuick archive; B uses the previously verified combined upload-admission/span-retention/bounded-order archive. Hashes and compact evidence are archived in profiler/experiments/current-render-stalls-2026-10-18. No production source edits or experimental renderer rollout occurred in this investigation.

Actual wheel interval 30 ms, Downloads/184 entries, Masonry density 152, DPR 1.75, active 160 Hz window, unchanged GC budget 5,000 us. Clean PresentMon captures have no QML profiler, UIA observer, CPU sampler, build or tests. CPU/sample/context-switch captures are separate and are for attribution, not headline cadence.

Normal sampled trace: 49,001 identical context-switch events aligned raw QPC and exported times, spread 900 ns. Candidate: 46,596 matches, spread 900 ns. Both report zero lost events and buffers. Both recording sessions were stopped successfully. Scheduled running time includes kernel execution; GUI and render execution can overlap, so do not sum them as sequential time. Stack counts are inclusive samples, not exclusive durations. ICF-folded leaf names may be aliases; broad upload/GC call chains are the reliable evidence.

## Normal renderer: large upload bursts are still a primary bottleneck

| Native period ms | Render scope ms | Render scheduled running within period ms | GUI scheduled running / waiting ms |
| ---: | ---: | ---: | ---: |
| 23.8890 | 17.0769 | 20.1880 | 3.9676 / 19.8714 |
| 22.3838 | 20.6085 | 22.3748 | 4.6180 / 17.7298 |
| 18.4526 | 15.3506 | 13.7236 | 4.3473 / 13.9993 |
| 16.4996 | 2.6134 | 3.7736 | 10.5025 / 5.8890 |

The two longest render scopes were entirely scheduled running (17.0769 and 20.6085 ms). The 22.3838 ms period contains 18 prepareRenderPass samples, 15 uploadBatch, 14 unmap, 6 memcpy, and 2 buffer-create samples; these overlap. The longest native period similarly contains upload/unmap/copy and kernel page-zeroing paths. GUI waits dominate those upload periods. The separate 16.4996 ms frame instead shows GC transitions/runGC and delegate creation/binding on the GUI thread.

Ready-delay p99/max: GUI 0.025/0.145 ms; render 0.010/0.042 ms. This does not support a thread-priority fix for the large upload bursts. The display-aligned analysis includes approximately 18.7–25.0 ms display gaps associated with the upload sequence; there are additional smaller GC/delegate-related gaps.

## Clean A/B/B/A comparison on current source

Display statistics require both endpoints in the same scrolling-direction observation window.

| Run | Down display p99 / max ms | Up display p99 / max ms | Worst native / render scope ms |
| --- | ---: | ---: | ---: |
| A1 | 9.2962 / 24.9956 | 6.3384 / 12.4991 | 22.0357 / 16.4155 |
| B1 | 6.3357 / 12.5092 | 6.3190 / 12.5153 | 14.5581 / 7.3834 |
| B2 | 6.3113 / 12.5002 | 6.3011 / 12.5019 | 14.0866 / 4.7008 |
| A2 repeat | 6.3225 / 12.5040 | 6.3204 / 12.4903 | 18.9680 / 10.8209 |

All four tabled captures have all GUI frames active and 268 wheel events. Original A2 lost focus (1,087 of 1,310 GUI frames active), so it is retained in the evidence but cannot serve as the active-window control: native max 23.2219 ms, down display p99/max 12.3585/18.7540 ms, up 13.7297/575.0026 ms. It was repeated because the protocol was violated, not because of its unfavorable result. The original has 267 wheel events.

The candidate lowers the largest native/render bursts in this sequence. A2 repeat has comparable display tails to B, so display-cadence improvement is not uniform. Constant 160 Hz is still not established, even though most display intervals and p99 values are near 6.25 ms. PresentMon display changes are distinct from native frame periods and are not optical measurements.

## Candidate sampled trace: the remaining limit shifts toward GUI work

The candidate sampled run has native max 13.5836 ms, render-scope max 7.0709 ms; the latter includes 5.7779 ms scheduled running, 1.132 ms waiting, and 0.161 ms ready. Next render scope is 3.8075 ms. No 15–21 ms upload scope was reproduced in this capture.

Several long periods now contain 8.94–10.73 ms GUI running time:

- 12.1249 ms native period: GUI running 10.5589 ms, including GC/onEventLoop and prepareNearbyDelegates/createComponent samples; render scope 3.6521 ms.
- 11.2038 ms period: GUI running 10.7318 ms, including 9 GC transition samples, 6 runGC/allocate/string-allocation samples and 4 QObject-wrapper marking samples; render scope 3.1118 ms.
- 12.5632 ms period: GUI running 6.241 ms with nearby delegate preparation/bindings; render scope 3.0568 ms.

The candidate's GUI ready-delay p99/max is 0.020/1.102 ms and render 0.009/0.067 ms. Preserve the 1.102 ms scheduling outlier rather than claiming scheduling is universally negligible. It does not explain 9–11 ms GUI execution in the highlighted periods.

## Next actions and current state

Before promoting the combined renderer, measure first complete-paint latency and rerun its native/gallery correctness gates on current source. Admission must not appear smooth merely by delaying complete previews. The archive's earlier correctness coverage is documented in combined-upload-policy-2026-10-18.md, but it does not fill the current latency gap.

Then address the independently verified GUI/delegate/GC path. prepareNearbyDelegates currently creates up to two delegates synchronously in a nominal 2 ms loop, with retirement/release work occurring before that timer starts. Fresh evidence should distinguish synchronous delegate construction, forced allocation GC, event-loop GC phases and retirement; do not repeat the previously inconclusive GC-budget/chunk constants blindly.

The normal embedded worktree application is restored. The combined archive remains experimental and outside the embedded payload. No commit or push was made. The original constant-160-Hz objective remains active.

AI assistance has been used to create this output.
