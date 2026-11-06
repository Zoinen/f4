# Partial-root merged-batch retention — 2026-10-21

## Status

The 160 Hz goal remains active and unfulfilled. This turn adds diagnostic evidence and an artifact-only renderer candidate. No production Qt policy, embedded payload, or running normal app was changed. No commit or push. The previous native staging changes remain in the dirty worktree, unpromoted.

## Order-space hypothesis ruled out

A fresh reversible probe records allocation failure in `Updater::visitGeometryNode` only when `f4TakeGeometryOrder` returns null. It records zero failures across qualifying cold and warm rapid-scroll runs (1816/1861 active wheel events, DPR 1.75, 160 Hz). Four isolated QML import checks and the system-DLL audit pass. Capacity remains 16384, reserved 0.

Separately verified the actual Qt 6.11.1 enum: `BuildRenderListsForTaggedRoots=1`, `BuildRenderLists=2`, `BuildBatches=4`, `FullRebuild=65535`. Thus flag 5 in the preceding report means **partial tagged-root rebuilding plus batching**, not broad from-scratch render-list rebuilding. Earlier reasoning that treated flag 5 as a full-list rebuild was wrong. No order-padding/budget change was made.

## Proven unnecessary merged upload

The existing partial-root retention predicate invalidates merged batches if any order changes anywhere in their root, even when the batch's own members and the complete intervening drawing-order span remain unchanged.

An artifact native regression creates two alpha-blue elements in one merged batch and adds a distinct green neighbor after its span. With the reference path in the same test executable, uploads of the watched two-member batch increase from 1 to 2 and the new test fails. With the candidate enabled, the upload count remains 1, and rendered pixels remain correct.

`artifacts/merged-span-retention/StableMergedSpan.h` implements a conservative experimental predicate. It requires an existing merged, clean, non-purging ordinary batch, unchanged global order limit, unchanged per-member order and root/membership, compatible material/opacity class, and exact identity of every intervening render-list element. It leaves ordinary dirty/invalidation handling in charge. This experiment adds no rasterization, pixel-grid changes, or owner cache.

The fixture also moves the neighbor into the batch's drawing span, checks the expected alpha composition, restores it, changes a member's material color and verifies that pixels update. Actual leaves have stable object names, whole physical origins and unit scene vectors at DPR 1.75. Images are saved. All 45 native renderer cases pass with no skips on Direct3D and OpenGL. The candidate host passes 4 embedded import-isolation checks and the Windows system-DLL audit.

## Same-executable timing experiment

Both arms use `f4-qt-host-merged-span.exe`. Reference selection is propagated through `F4_CAPTURE_SPAN_REFERENCE` into child `F4_PROFILE_REFERENCE_SPAN`; a static branch disables only the new retention predicate. This selection is supported by the launcher source/environment path and the same-executable failing regression, but the runtime snapshot does not yet explicitly record the selected arm. Add that field before further controlled comparisons.

All four cold/warm runs pass existing workload, focus, input-count and capacity gates. Initial order: reference then candidate. Reversed order: candidate then reference. Each stage has two 10-second blocks. All use 187 entries, density 152, actual DPR 1.75 and screen 160 Hz. Matched extents are 4075.142857 cold and 3623.142857 warm. No builds, tests, offline analysis or UIA polling run during input capture.

Initial comparison:

| Warm blocks | Reference | Candidate |
| --- | --- | --- |
| Upload calls | 87789 / 86697 | 76643 / 75817 |
| Uploaded bytes | 107.31 / 106.04 MB | 93.40 / 92.14 MB |
| Preparation median | 0.415 / 0.447 ms | 0.650 / 0.620 ms |
| Preparation p99 | 1.659 / 1.779 ms | 2.329 / 2.039 ms |

Reversed comparison:

| Warm blocks | Reference | Candidate |
| --- | --- | --- |
| Preparation median | 1.394 / 1.231 ms | 1.257 / 1.066 ms |
| Preparation p99 | 4.836 / 4.539 ms | 3.992 / 3.724 ms |
| Preparation maximum | 7.504 / 7.660 ms | 5.694 / 5.137 ms |

The direction of the timing result does not repeat, and several phases change substantially between runs. An initial progress message attributed the regression to the candidate checks; that attribution is **not established** by the reversed data. The predicate adds traversal/hash work, but its actual share and possible batch-fragmentation changes need measurement. The uploaded-call/byte reduction does not prove smoother presentation.

Full render-phase/PresentMon attribution also shows remaining failures: initial warm reference has 0/0 long presents (>9.375 ms), candidate 1/2; reversed warm reference has 100/82, candidate 79/44. Later render-to-frame-end p99 reaches 7.72 ms for reference and 6.36 ms for candidate. These are instrumented-host observations, not proof of production performance. Do not relax gates or claim 160 Hz completion.

See `merged-span-comparison.json`, `merged-span-phase-comparison.json`, `merged-span-r2-phase-comparison.json`, `merged-span-input-phase-summary.json`, archived full phase findings and raw packets/PresentMon CSVs.

## Next actions

1. Expose the selected policy explicitly in runtime evidence and count prepared alpha/opaque batches to distinguish predicate cost from fragmentation/material work.
2. Prefer alternating policies inside one diagnostic session, with both orders and clear block metadata, to reduce the observed run-to-run variation. Keep workload/focus/timing gates unchanged.
3. If predicate cost is significant, investigate reusing existing order metadata or proving an unchanged span with precomputed consecutive runs instead of repeatedly scanning overlapping spans. Do not add an unmeasured cache.
4. Require repeatable total frame-time and presentation benefit before updating the production Qt patch. Keep the passing interleaving/material regressions and add any counterexample first.
5. Obtain qualifying minimal rapid and quarter-height middle captures, finish portable gates, and replace the worktree app only after a useful candidate is verified.

Only exact owned diagnostic launcher trees were stopped. Normal launcher PID 92184 and Qt PID 103492 remain the previous packaged build. Captures remain authorized without readiness questions.
