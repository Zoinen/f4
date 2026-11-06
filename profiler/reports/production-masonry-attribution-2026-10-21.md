# Production-library Masonry profiling after the QML signal fix

Constant 160 Hz without spikes is not achieved. The strict 2 px motion checks
fail for both middle-button and rapid-wheel captures. The deployed worktree
app still contains the verified QML signal-mask fix; this investigation adds no
further production changes.

## Rebuilt diagnostic baseline

The fresh harness uses the deployed host's current Ninja link graph, production
Qt Quick and QML archives, and current ZoinGallery library. Only its main routine
and GalleryScrollAnimation implementation are replaced for input control and
bounded timing/motion recording. The animation instrumentation reverses exactly
to the current source. No experimental vector-chunk, material, texture-factory,
renderer or RHI object is linked. Per-material and renderer timers are absent.

The current Ninja response-file expansion is saved without rebuilding or
modifying the deployed host. Four import-isolation/host-loading tests and the
Windows system-DLL audit pass. Complete link receipts include explicit objects,
archives, source and capture-generator hashes. The normal app stays running.

The workload is Downloads, 187 entries, Masonry density 152, window 2194x1186,
DPR 1.75, 160 Hz; cold/warm extents are 4075.142857 and 3623.142857. Both input
blocks retain focus. Caption measurement count is zero. Middle-button pointer
distance is one quarter of window height. Rapid-wheel events use a 10 ms input
interval. Frame/recorder bounds and presentation coverage checks pass.

## Production-path measurements

Displayed intervals over 9.375 ms, block 0 / block 1:

| Input | Cold | Warm |
| --- | ---: | ---: |
| Middle-button | 0 / 3 | 4 / 0 |
| Rapid wheel | 4 / 74 | 6 / 2 |

The phase analyzer also counts long producer-present intervals. Those counts
are different from displayed misses and must not be substituted for this table.

Middle-button preparation p99 ranges from 2.4392 to 3.8464 ms; recording p99
ranges from 1.1322 to 2.2378 ms. Rapid-wheel preparation p99 reaches 7.1323 ms,
and render-to-frame-end p99 reaches 10.3301 ms in cold block 1. These are frame
phase wall times, not CPU-only durations.

Largest displayed-step error in cold rapid block 1 is 194.7456 px. For that
unchanged curve segment, displayed frames are 18.8959 ms apart while the
recorded animation/precise timeline advances only 6.25 ms. The analyzer checks
that the curve explains each recorded camera pose and excludes retargeted,
rebased and coverage-constrained pairs. No ceiling was relaxed. This observation
does not establish that changing Qt's clock would solve the render-time stalls.

An unrelated Java process was observed after these captures using roughly ten
CPU cores. It was not stopped. This is evidence of concurrent load, not proof
that it caused any particular captured stall. The older experimental-harness
numbers are not treated as a matched baseline: the linked code and current
background conditions differ.

## Scheduler attribution

A separate owned basic ETW scheduler trace has zero lost events/buffers, schema
errors or processing errors. Target process/thread identities, input-window
coverage and decoded-state completeness are checked. The longest cold block-0
preparation span is 8.9170 ms: 8.9152 ms scheduled, 0.0018 ms runnable, zero
blocked time. That span is actual render-thread CPU work, beyond the 6.25 ms
budget; it cannot be explained solely by waiting for a CPU.

A separate submission span lasts 7.1792 ms: 0.4994 ms scheduled, 0.0063 ms runnable
and 6.6735 ms blocked. Its stack was not captured in this basic trace; it is not
labeled as a proven GPU wait. Frame pacing/synchronization waits are analyzed
separately from preparation, recording and submission.

## Fresh CPU stacks

The CPU-stack capture uses the same production-library host, Profile stacks,
module loads and basic scheduler events. It does not collect context-switch
stacks. Sampling adds overhead, so its frame tails are attribution data rather
than a matched performance comparison.

An auxiliary symbol map has byte-identical executable code (41,870,762 bytes),
image base and runtime-function table to the actual captured executable. ASLR
module ranges and chained x64 unwind entries are used for symbol attribution.
The reader reports zero trace loss or schema errors. Its SampledProfile prefix
fallback is independently checked against xperf: all 149 cold and 87 warm
reference samples match thread, instruction pointer and timestamp.

Render-thread samples total 3047 cold and 2825 warm, with zero and one missing
stack respectively. Self samples include:

| Function | Cold | Warm |
| --- | ---: | ---: |
| QSGNodeUpdater::isNodeBlocked | 194 | 169 |
| D3D11 StageUniformBufferBatches copy constructor | 185 | 145 |
| QRhiBatchedBindings<unsigned int> destructor | 87 | 90 |
| D3D11 ResourceBatches copy constructor | 72 | 73 |
| updateMaterialDynamicData | 61 | 91 |

These are sampled instruction counts, not allocation counts or exact function
durations. Qt's ancestor check walks toward the renderer root and invokes each
node's blocking predicate. Preprocess callbacks can change graph state, so a
cache would require a correctness/invalidation design rather than simple
cross-callback memoization. See
[Qt 6.11.1 node updater](https://github.com/qt/qtdeclarative/blob/v6.11.1/src/quick/scenegraph/coreapi/qsgnodeupdater.cpp).

The earlier offset-only snapshot-reuse candidate addressed only one recording
case and did not show a repeatable end-to-end gain. The full resource-copy cost
remains visible here. Next work should measure and reduce copying of inactive
shader-stage data, with native command/GPU regressions, or validate a visibility
optimization against mutation and custom-node behavior. Neither is implemented
or promoted by this report.

Evidence: artifacts/production-profile-build-receipt.json,
artifacts/production-middle-phases.json, artifacts/production-rapid-phases.json,
artifacts/production-middle-motion.json, artifacts/production-rapid-motion.json,
artifacts/production-middle-strict-gate.log,
artifacts/production-rapid-strict-gate.log,
artifacts/production-scheduler-attribution.json,
artifacts/production-stack-symbol-attribution.json,
artifacts/production-profile-symbol-map-verification.json,
artifacts/production-cpu-prefix-verification.json and matching case files under
artifacts/modes-profile.
