# Current-graph motion versus display deadlines

## Result and next action

The current production scrolling logic is accurate against its native clock in
the verified-warm middle fixture, with no coverage holds. Intermittent missed
display intervals still cause approximately 17.8 physical pixels of camera
timing error at the requested quarter-height pointer displacement. The remaining
160 Hz objective is not achieved. Do not add rounding or retargeting changes to
address this evidence: the next trace should follow presentation/composition
deadlines, including DWM, rather than assume another expensive tile traversal.

No production behavior, SDK, dependency recipe, embedded payload, or ordinary
running app was changed. These are diagnostic host builds and measurements.

## Scope and observer

`artifacts/current-motion-20261031/` contains the read-only native commit probe,
source/build receipts, launchers, measurements, and analyses. The animation
source is the current worktree's `GalleryScrollAnimation.cpp`, SHA-256
`f80a9e0d6b154e66d4e60afca045de7d6a2028af41a4748e7035d495df0b4d6f`.
Its diagnostic copy only records pre-coverage requested pose and post-commit
state into a bounded, prefaulted 16,384-sample array. There is no per-commit JSON,
allocation, or additional locking. Overflow is checked after capture. The
existing main-only warm observer supplies frame stages and input driving.

The native probe records camera matrix, logical/visual pose, job/revision,
animation time, velocity, geometry revision offset, and coverage bounds. This
is native-model evidence, not optical measurement of panel pixels. No rendering
policy or native-default setting is forced. The middle fixture uses the real
AutoScrollController with an isolated pointer map and restores its pointer source
when finished; native input hit testing is outside this fixture's proof.

Every run uses all 192 Downloads entries, density 152, actual DPR 1.75, 160 Hz,
normal maximized window geometry and foreground focus. Each timed run follows a
verified tour of 17 quiescent viewports covering all entries. Cache frame,
store, and miss deltas during input are zero. No screenshots, UIA queries,
compiler, or offline analyses run during timed input.

## Captures

| Capture | Native commits | Render p99/max ms | Missed display intervals | Dropped presents |
| --- | ---: | ---: | ---: | ---: |
| wheel-a1 | 4762 | 2.654065 / 5.1492 | 0 | 0 |
| middle-a1, initial fixture | 4801 | 2.574 / 4.6392 | 2 | 2 |
| middle-b2, original reversal margin | 4800 | 2.581016 / 4.8328 | 1 | 1 |

All three have zero coverage holds and no probe overflow. The wheel camera
matrix float error is at most 0.000233 physical pixels. Its 36 frames without
motion commits are retained; this does not prove complete wheel motion
acceptance, nor a wheel curve-error bound.

The initial middle fixture mistakenly reused the wheel's 8-physical-pixel
reversal guard. At 1627.433 logical pixels/s the camera reached the folder
boundary before GUI reversal, producing five boundary-limited pairs. Preserve
the original result and failure: `middle-a1/motion-analysis-original.json` reports
17.800053-pixel raw native error. A separate boundary-aware model explains it
to within 8e-13 pixels; the original error was not erased.

The repeat restores the earlier fixture's 64-logical-pixel reversal guard; this
changes test input only. Its pointer displacement is exactly windowHeight/4
(296.5 logical pixels). It has no boundary-limited pairs. Across 4,781 accepted
constant-velocity pairs, maximum native-clock error is 3.61e-13 physical pixels,
and maximum matrix float error is 0.000204 pixels. All 4,800 matched frames have
a motion commit. Thirteen reversal pairs are explicitly retained as exclusions,
one first-frame present boundary is unmatched, and one dropped-present gap is
retained. No missing frame is replaced with a nearest neighbor.

Despite that native accuracy, maximum camera/display timing error is 17.805995
pixels. At the missed display interval, the preceding pose remains displayed
for about 12.47 ms while native time advanced by 6.25 ms; after the dropped
present, the camera advances two native ticks in one display period. Both the
hold and catch-up errors remain in the analysis. This fails the 2-pixel display
motion threshold; it is not a successful end-to-end motion result.

The analysis has seven executable fixture tests, including constant velocity,
display delay versus native-clock delay, a true hold, reversals, geometry/job
changes, and preservation of the raw boundary failure.

## GPU-timed follow-up

`middle-b2/middle-gpu-c1*` is a separately named 30-second run, with a fresh
verified warm tour in the same process. It enables PresentMon 2.x GPU tracking;
keep it distinct from the preceding lighter captures. No events are excluded
at capture time. `gpu-analysis.json` retains raw neighbors for every display miss.

The analyzer joins 4,797 PresentMon rows to Qt afterRendering..afterFrameEnd
using CPUStartQPCTime + CPUBusy, with a 1-microsecond tolerance. Two tail rows
cannot be joined and remain explicit. One missed interval and one dropped
present are retained. Metric definitions and their CPU-start origin were
checked against the [PresentMon 2.6 console documentation](https://github.com/GameTechDev/PresentMon/blob/v2.6.0/README-ConsoleApplication.md#csv-columns).

Across matched rows, GPU busy p50/p99/max is 0.4724/1.915524/2.9179 ms. GPUTime
includes substantial GPU wait, so its approximately 5.37-ms median must not be
called 5.37 ms of active application shader work. CPUBusy similarly includes
the elapsed pre-present envelope and is not exclusive CPU execution time.

For the missed interval at about +29.535 seconds (row 4724):

- Previous frame display timestamp: 1227620538.7015 ms in QPC milliseconds.
- Current frame display timestamp: 1227620551.1749 ms; interval 12.4734 ms.
- Qt render signal range: 1.0794 ms.
- Reported GPU busy: 0.4584 ms.
- Reported GPU completion: CPUStart + GPULatency + GPUTime =
  1227620532.8996 ms.
- A nominal next slot, previous display +6.2499 ms, is 1227620544.9514 ms:
  approximately 12.05 ms after reported GPU completion.

Thus this particular miss is not explained by the frame's tile rendering or
reported application GPU busy exceeding a frame budget.

The unfiltered CSV also retains 5,194 DWM rows. The follow-up
`dwm-correlation.json` finds unique matches for both the previous and current
application display timestamps (within 0.001 ms). DWM itself has the same
12.4734-ms gap. Its reported GPU completion is 1227620545.5801 ms, approximately
0.6287 ms after the nominal next slot; its GPU busy is 1.3795 ms and GPU wait is
7.6718 ms. This directly correlates the visible miss with a missed compositor
slot, while the application's reported GPU completion precedes that slot by
12.05 ms. The next trace should inspect DWM GPU packets/fences and scheduling.

This does not establish what delayed DWM, nor prove an exclusive driver or
other-process cause. PresentMon timing accuracy can depend on GPU scheduling,
and ETW loss was not independently audited in this lightweight follow-up.
Preserve these limitations rather than treating the derived completion
timestamp as an optical deadline guarantee.

## Remaining acceptance

Cold initial-viewport loading, complete latest wheel curve/physical-motion
verification, quarter-height end-to-end physical motion, static pixel alignment,
normal UI behavior, and a sustained no-miss/no-drop display result are still
required. Existing rendering/correctness tests and native-camera accuracy do not
replace these checks. The ordinary application remains the previously verified
portable production build; neither diagnostic host was embedded or rolled out.
