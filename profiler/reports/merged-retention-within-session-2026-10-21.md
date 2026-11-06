# Within-session merged retention attribution — 2026-10-21

## Status and scope

The previous turn made progress by reproducing an unnecessary upload and testing an artifact renderer candidate, but its separate-process timing result did not repeat. This turn improves the experiment and changes the next target. The overall constant-160-Hz goal is still unfinished. No new production Qt policy or packaged app change; no commit/push. The existing native staging worktree changes remain unpromoted.

## Diagnostic design and validation

`artifacts/span-abba-probe` uses one executable and alternates the existing reference and merged-span candidate between measurement blocks. The normal sequence is reference/candidate for cold, candidate/reference for warm; a second run reverses it. Each block explicitly records `mergedSpanEnabled`. Each render pass snapshots the policy once; helper checks use that pass-local value. Runtime verification rejects policy mismatches and requires candidate calls/retentions and zero candidate calls/timing in reference blocks.

The bounded 16384-row probe records prepared alpha/opaque batch counts, predicate calls/retentions, visited members/span entries, and aggregate predicate wall time. The wall time includes observer costs and possible preemption; it is not claimed as scheduled CPU time. A linked thread-local scope restores the owning renderer when render passes nest.

Five counter-fixture cases including init/cleanup pass: nested ownership, pass-local policy stability and capacity overflow/stack restoration. All 45 native renderer cases pass, no failures/skips, at 175% on Direct3D. Four isolated QML import tests and the Windows system-DLL audit pass. The preceding uninstrumented candidate also passed the 45 cases on both Direct3D/OpenGL.

Both alternating live captures pass unchanged workload/focus/input-count/capacity gates. Each block has 1600–1601 contained render passes. Actual DPR is 1.75 and screen refresh rate 160 Hz. No concurrent builds, tests, offline analyses or UIA polling during live input.

## What the counters establish

| Candidate block | Calls | Kept batches | Median checking time, active frames | p99 checking time |
| --- | ---: | ---: | ---: | ---: |
| normal / cold | 78336 | 12204 | 0.0061 ms | 0.0162 ms |
| normal / warm | 82462 | 13651 | 0.0060 ms | 0.0178 ms |
| reversed / cold | 79328 | 13920 | 0.0063 ms | 0.0164 ms |
| reversed / warm | 73921 | 11205 | 0.0060 ms | 0.0190 ms |

The maximum is 0.0214/0.0256/0.0260/0.1149 ms respectively, including measurement/preemption overhead. Reference frames record no checks. This rules out the earlier claim that the observed 0.2–0.3-ms median regression was simply predicate checking cost.

Prepared alpha batch medians are 248 versus 254 in normal cold, 255 versus 255 in normal warm, 248 versus 248 in reversed cold, and 253 versus 249 in reversed warm; opaque medians remain 5. There is no evidence of large accumulating batch fragmentation in these runs. Do not treat this as a proof about every scene or long-duration workload.

The candidate reduces uploads in the warm blocks: normal candidate/reference 78472/90449 calls, 95.43/110.26 MB; reversed reference/candidate 86903/72399 calls, 106.32/88.46 MB. Preparation median/p99 do not show a consistent useful benefit: normal warm candidate 0.549/1.867 ms versus reference 0.478/1.795; reversed warm reference 0.432/1.613 versus candidate 0.467/1.857. Therefore do not promote it on upload savings alone. No further per-member cache or predicate rewrite was implemented based on the disproven cost assumption.

## Remaining presentation evidence

Presentation-call spacing and displayed spacing are distinct. The phase analyzer's `longPresents` counts producer intervals above 9.375 ms, not displayed gaps. Those long producer intervals often combine roughly 5–6 ms pacing/synchronization with up to 5.5 ms render-to-frame-end work. This does not alone prove a missed physical refresh.

The unchanged motion analyzer still finds failures using reported displayed timestamps. Representative pairs have 5.084/7.414-ms display deltas with exact 6.25-ms animation increments, or 2.823/9.680-ms deltas with the same exact animation increments. Some blocks satisfy the existing two-pixel bound, others fail substantially. In the normal warm candidate block, the existing analyzer records 13 displayed intervals above 9.375 ms. No limit was weakened and no animation-clock change made.

Next evidence must cross-check presentation independently rather than infer a rendering bottleneck from these timestamp pairs. Local Qt headers provide render-thread access to the owned `QD3D11SwapChain` through `QQuickWindowPrivate::swapchain`; no RHI library rewrite is necessary merely to query counters.

Microsoft documents flip-model windowed presentation statistics, including present IDs and refresh counters, as a way to detect presentation glitches: [DXGI flip model](https://learn.microsoft.com/en-us/windows/win32/direct3ddxgi/dxgi-flip-model), [frame statistics fields](https://learn.microsoft.com/en-us/windows/win32/api/dxgi/ns-dxgi-dxgi_frame_statistics). These counters are not automatically authoritative in every environment: [GetFrameStatistics](https://learn.microsoft.com/en-us/windows/win32/api/dxgi/nf-dxgi-idxgiswapchain-getframestatistics) explicitly warns about multiple-monitor/fullscreen-app scenarios. Record HRESULTs, swapchain identity/generation, submitted/completed counts, refresh/QPC counters and display inventory; do not call blocking DwmFlush in a timing run or silently substitute a new clock. If counters cannot establish the distinction, independent pixel/presentation observation is still required.

## Next actions

1. Add a bounded render-thread DXGI counter observer to a fresh minimal diagnostic; test its reset/identity/error handling and record actual display inventory.
2. Correlate successful completed-present/refresh counters with existing PresentMon, animation commits and phase brackets. Treat disjoint, unavailable, stale and multi-monitor-ambiguous samples honestly.
3. Validate the actual displayed motion before changing animation sampling or timing gates. Keep the two-pixel limit and displayed-gap checks intact.
4. Revisit renderer changes only if they provide repeatable total frame/presentation benefit. Cold upload work and normal-app portable rollout remain unfinished.

Evidence: `span-abba-counter-analysis.json`, both preparation analyses, `span-abba-preparation-comparison.json`, full phase and motion snapshots, slow-present attribution, raw packets and focus/build receipts. Owned diagnostic trees were stopped; normal launcher PID 92184 / Qt PID 103492 were kept alive. Captures remain authorized without readiness questions.
