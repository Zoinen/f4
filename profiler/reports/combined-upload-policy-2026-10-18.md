# Combined upload policy experiment — 2026-10-18

## Composition and source verification

The isolated QtQuick archive combines current production buffer-fit/stable-interval behavior with three experimental mechanisms: span-based raw unmerged buffer retention, globally bounded drawing-order padding, and v2 initial vector upload admission with owned transform-root intervals. The existing committed gallery consumer discovers the optional v2 capability; no Go protocol or new QML import is introduced.

Composition order matters. An early attempt placed a transformation where a later whole-function replacement removed its insertion. The final builder applies span retention first, bounded reservations second, and admission last. It asserts the budget scope remains inside buildRenderListsFromScratch and that both retention and admission code remain present. Only the final verified composition was measured. No obsolete or failed executable was used for the comparison.

The interface translation unit was fetched from Qt's official qtdeclarative v6.11.1 tag and compiled against the matching static SDK, replacing the earlier experiment's stale source path. Its URL/hash provenance is archived. Generated source hashes are qsgbatchrenderer.cpp 1d82e95a365287c68f9d5e0118d69fbdc7a16f4196aa172c92e31479da24bb9b and qsgrendererinterface.cpp aff2596963d0df65c1bc3fa9c2a367b68a30e46e2764f364f45e986f63388de9.

## Correctness evidence

All 30 native fixture results pass on Direct3D and OpenGL at actual DPR 1.75. This includes bounded reservation/depth-space tests, retained overlapping/interleaved geometry, mutation and material classification changes, clip/root lifetimes, priority, cancellation, deferred presentation, and large-batch forward progress. The four native admission tests that lacked a capability in earlier retention-only builds now pass.

All 26 focused gallery vector results pass with ZOIN_EXPECT_UPLOAD_GATE=1. This explicitly confirms the optional capability is available and native readiness participates in real viewer swaps. Coverage includes source acquisition/failure/cancellation, navigation and prefetch skipping, swapping, scene reuse, all five panel modes, transparency/gradient/image content, resizing and authored animation. The completed native admission image was inspected: all 24 red cells paint on blue only after readiness/presentation. Existing 175% leaf geometry and rendered comparisons are preserved.

These are correctness checks for an experiment, not a complete portable-release claim. The archive remains outside the normal dependency recipe and embedded payload.

## Clean actual-wheel A/B/B/A

A is current production Qt behavior; B is the combined archive. Each fresh process uses Downloads with 184 entries, density 152, DPR 1.75, D3D11, active 160 Hz window and unchanged default GC budget 5000 us. Wheel interval is explicitly 30 ms: A1/B1/A2 have 267 events and B2 268. No diagnostic observer, ETW sampler, build or tests run during these measurements. Display statistics require both endpoints within the same direction window.

| Run | Down display p99 / max ms | Up display p99 / max ms | Worst Qt frame interval ms |
| --- | --- | --- | ---: |
| A1 | 12.4239 / 31.2201 | 7.8942 / 10.8720 | 24.2446 |
| B1 | 7.9457 / 14.1757 | 7.9606 / 12.5033 | 13.3005 |
| B2 | 8.0687 / 16.9921 | 8.0139 / 12.4689 | 14.6739 |
| A2 | 18.7830 / 37.5004 | 14.3238 / 18.9765 | 38.6986 |

Candidate worst native intervals and down-direction display tails are lower than both controls in this sequence. A2 is substantially slower than A1 and remains in the results; no unsupported environmental explanation is used to discard it. Up-direction results and repeatability are insufficient to claim constant 160 Hz or a general performance guarantee. The original goal remains unachieved.

## Remaining evidence gap and next action

The candidate's worst Qt intervals have render scopes about 5.4–5.6 ms and sync scopes about 0.9–1.1 ms. Other long intervals have render scopes around 1.8–4.2 ms. Per-frame scopes alone do not assign the remaining elapsed time to JavaScript, GC, scheduling, presentation or another cause. The next action is a simultaneous sampled/context-switch capture of this exact combined archive, with raw-QPC alignment, rather than applying another constant or assuming the old bottleneck is unchanged.

Before promotion, measure initial complete-paint latency as well as frame tails, repeat real scrolling comparisons, and run the normal portable import/dependency/payload gates. Initial admission is a soft work budget, not a hard per-frame guarantee; an individual driver call and unregistered/ready work can exceed it.

## State and reproduction

Reproducible Qt patch, builder and dependencies, native regressions, source hashes, compact measurements and test logs are archived under profiler/experiments/combined-upload-policy-2026-10-18. The Qt patch applies to the current buffer-fit renderer base and matching upstream interface source. The experimental archive is artifacts/qt-combined-upload/Qt6Quick.lib, host artifacts/f4-qt-host-combined-upload.exe. Raw captures are artifacts/modes-profile/combined-{a1,b1,b2,a2}-wheel-masonry.*.

The native test source and normal test linkage were restored after capture. The portable worktree app is restored to its preceding timer-fixed build. No prototype has been shipped, committed or pushed in this turn.

AI assistance has been used to create this output.
