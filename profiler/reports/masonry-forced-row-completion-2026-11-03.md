# Masonry forced row completion experiment

Cold Downloads capture o187 used the 194-entry corpus and the same 160 Hz display and 175% scaling as control o184. The diagnostic host forced at most one newly started delegate to finish per panel per frame. The remaining construction stayed asynchronous.

| Fixed wheel input | o184 control | o187 forced completion |
| --- | ---: | ---: |
| Coverage holds | 246 | 188 |
| Skipped input slots | 197 | 187 |
| Display interval p99, ms | 6.297 | 6.322 |
| Display intervals above 9.375 ms | 4 | 7 |
| Longest display interval, ms | 50.00 | 56.24 |

Across the census, 143 forced completions took 0.968 ms median, 1.429 ms p95 and 2.932 ms maximum. The target swap chain recorded zero dropped present rows.

Joining adjacent present submissions by QPC shows forced completion occupied 2.374 ms of the 58.242 ms submission gap associated with the 56.241 ms display interval. Four of the seven long display intervals during input had zero overlap with forced-completion scopes in their corresponding submission gaps. These comparisons measure overlap and cannot attribute the remaining time to a thread or prove a causal effect on displayed cadence. The first input was delivered 244.44 ms after navigation in o187 and 224.65 ms in o184.

The experiment remains diagnostic-only. Coverage improved in this run, while the cadence criterion still failed. The largest unattributed gaps occur within two seconds of first input, including during intervals without forced work. Scheduler attribution in that window can distinguish GUI execution, render waits and other off-CPU time before testing further delegate scheduling changes.

## Frame-stage attribution

All seven input-window outliers matched a recorded frame by present QPC between its after-rendering and frame-end markers. Times below are milliseconds. The interval before frame begin is measured from the previous frame end. These are elapsed signal intervals, which can include waiting or descheduling.

| Present after first input | Display interval | Before frame begin | Frame begin to before sync | Sync | Before to after rendering |
| ---: | ---: | ---: | ---: | ---: | ---: |
| 3.10 | 25.01 | 21.01 | 0.05 | 0.57 | 0.95 |
| 61.34 | 56.24 | 48.14 | 0.09 | 8.70 | 1.01 |
| 198.37 | 18.76 | 24.66 | 0.07 | 0.56 | 1.38 |
| 598.19 | 31.24 | 0.01 | 5.72 | 25.85 | 2.01 |
| 619.34 | 12.46 | 0.02 | 5.40 | 9.77 | 1.68 |
| 1959.21 | 37.52 | 0.01 | 5.37 | 17.64 | 20.66 |
| 1989.38 | 31.25 | 0.01 | 0.09 | 0.49 | 29.14 |

A scheduler capture should span all three regions, with GUI and render thread attribution through at least 2.1 seconds after input begins. The last outlier alone spends 29.14 ms inside the rendering signal interval, so a probe limited to GUI activity before frame begin would miss it.

## Scheduler repeat

Capture o188 reused the o187 host and added context-switch and wakeup tracing. The decoded trace had zero event, buffer, and schema losses and covered from 599 ms before first input to 6.13 seconds after it. The o187 stalls did not recur at the same times. Two long display intervals within the traced input window reached 12.48 and 12.50 ms, at 1.634 and 1.672 seconds. In the corresponding submission gaps of 9.79 and 12.78 ms, the render thread was running for 9.26 and 11.30 ms and runnable without a CPU for only 0.10 and 0.04 ms. GUI running time was 5.46 and 6.32 ms, which can overlap render work. The first two inputs had the provisional 48-entry catalog and the remaining 2,826 had 194 entries. The input verifier accepted that scope, with 172 skipped slots.

The reproduced misses support sampling the render thread's executing stacks during early scrolling. This repeat cannot assign the original o187 gaps to specific call sites or explain the larger spikes that did not recur.

## Combined scheduling and CPU sampling

Capture o190 reused the forced-completion host and recorded context switches, wakeups, and sampled CPU stacks together. Both decoders reported zero event, buffer, or schema errors. The independent xperf prefix comparison matched all 426 samples. Within the trace window, three input-window display gaps were 81.26, 56.22, and 50.00 ms. The GUI thread was running for 78.18, 57.23, and 39.46 ms in the corresponding submission gaps, and the render thread was waiting for 75.51, 54.93, and 45.28 ms. These three gaps differ from the render-heavy misses in o188. Sampled GUI call paths from these same o190 intervals are the next attribution target.

## GUI stack attribution

The o187 host was relinked into a separate file with a linker map. Both binaries had identical executable section bytes, RVA 4096, and virtual size 41,850,266 bytes. The executable section SHA-256 was 5314696cc6ebc040df613bc32bae05993dfa71bbc86724883bff92aa5e91d370. This allowed mapping the original o190 sampled addresses without launching the relink.

The three submission gaps contained 78, 56, and 41 GUI samples. QML binding updates appeared on 27, 13, and 13 matching stacks. The first gap included sparse extension and Masonry rewrap on nine stacks each. The next two included applyPanelCatalogFrame on seven and nine stacks, and the third included QML garbage collection on five. Counts overlap.

In the catalog census, scope 34 wraps delegate presentation preparation. Its summed overlap was 11.09, 7.93, and 3.75 ms across the three gaps. Scope 72, which wraps forced completion, overlapped for 2.33, 1.96, and 2.10 ms. Nested scopes can overlap, and the census covers only instrumented work.

A concrete publication path to isolate is the compact presentation signal from commitPanelCatalogFrame through ShellSceneStore.applyCompactPatch and PanelsSurface.updatePanelDescriptor. It publishes the row-free descriptor into QML, where PanelsSurface serializes it to detect duplicates. The catalog signal separately feeds native model application. A controlled experiment should time these paths independently and compare identical descriptors before crossing into QML, while preserving compact layout override invalidation and the native catalog update.

## Catalog signal timing

Capture o191 enabled the existing navigation benchmark logging on the o187 host. Fixed early input and native foreground checks passed, with 2,802 of 3,000 input slots delivered. Of 25,990 prefixed log lines, one was malformed and excluded. The complete records contained nine panel catalog commits. Their synchronous native catalog signal took 1.974 ms median and 13.699 ms maximum. The compact presentation signal took 7.208 ms median and 11.293 ms maximum. The preceding compact-applying signal took 0.0013 ms median. Another 19 scene patches timed compact panel publication at 5.342 ms median and 12.765 ms maximum.

These are instrumented wall-clock intervals around signal delivery, including connected subscribers and any timing overhead. They cannot isolate JSON serialization from the downstream bindings. The separate compact-presentation path merits testing because its measured median alone exceeds the 6.25 ms frame budget.

The o191 navigation trace recorded 37 panel-session resolutions across 12 distinct combinations of side, panel identity, and catalog revision. Twenty-five repeated the previous combination for their side. All nine timed catalog presentation callbacks contained a resolution, but the 37 begin-to-applied spans were only 0.054 ms median and 0.633 ms maximum, including their trace calls. This rules out session lookup alone as an explanation for the 7.208 ms median presentation callback. The next split is the downstream work around layout override clearing, descriptor assignment, and workspace tab assignment in ShellSceneStore. Repeated side and revision do not prove descriptor equality.

## Equivalent layout override experiment

The focused gallery regression passed at 175% with the Basic style. The matched diagnostic pair used the same link inputs apart from the host adapter QML and corresponding embedded source resource. Both fixed-input captures were admitted with the 195-row panel scope. This pair is separate from the earlier 194-row captures.

| Fixed wheel input | o192 control | o193 candidate |
| --- | ---: | ---: |
| Compact catalog presentation median, ms | 8.59 | 6.12 |
| Compact catalog presentation maximum, ms | 17.37 | 10.95 |
| Skipped input slots | 225 | 196 |
| Display intervals above 9.375 ms | 7 | 2 |
| Longest display interval, ms | 25.00 | 75.00 |

The two candidate misses had 23.45 and 71.13 ms between the previous frame end and the next frame begin. Sync took 0.53 and 2.21 ms. Rendering took 0.61 and 1.03 ms. The first submission gap contained a 15.18 ms panel catalog apply. The second contained three scene patches of 31.44, 12.03, and 0.32 ms and a 13.93 ms panel catalog apply. The 31.44 ms patch included 21.93 ms in the typed commit and 8.20 ms in panel state signal delivery. The reducer took 0.45 ms. The following catalog apply included 10.95 ms of compact presentation delivery. The typed commit publishes to multiple state stores, so the current timing cannot identify the individual subscriber responsible for its 21.93 ms.

These single runs with navigation logging are attribution evidence. They do not verify steady cadence or establish a repeatable tail-latency improvement.

## Typed commit during the candidate's longest stall

The o193 sequence 51 frame was decoded off the GUI thread in 0.180 ms and waited 25.19 ms from decode completion to GUI apply. This queue delay is separate from the 31.44 ms application time. Both panels resolved their existing revision 4 sessions at 13.13 and 21.09 ms after application began, inside the 22.37 ms core stage. The right panel's layout and catalog notifications arrived at 25.41–30.48 ms, after core completion. These markers separate the two-sided QML work from the later panel state delivery. They do not identify whether a shell, workspace, or other state-store signal initiated the two-sided work. A low-overhead probe should time those individual publications rather than treating the whole typed commit as reducer cost.

## Individual state-store timings

The o194 diagnostic timed ten publication call sites in the typed commit. The capture lost foreground at 1,376.91 ms after the input window began and failed admission, so it is excluded from cadence comparisons. Before that loss, SurfaceRegistry::applyShell took 18.30 ms on the panel/1 stream and 19.01 ms on the shell stream. Across the diagnostic trace, the separate typed shell store took at most 0.011 ms, workspace publication at most 0.517 ms, and command-line publication at most 1.690 ms. This directly identifies broad shell publication as the expensive call in the diagnostic repeat. The original o193 commit was not instrumented at this granularity.
