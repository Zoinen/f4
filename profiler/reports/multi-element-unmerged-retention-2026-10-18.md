# Multi-element unmerged buffer retention experiment — 2026-10-18

## Reproduction and prototype

The native fixture now has multi-unmerged-order-change and multi-unmerged-full-order-change rows. Two same-material translucent leaves form a real two-element unmerged batch; membership is asserted. At actual DPR 1.75 the normal renderer uploads that batch a second time after an unrelated drawing-order change: actual cumulative uploads 2, expected 1. Both rows fail at this assertion before the experiment.

The isolated renderer prototype keeps raw unmerged buffers only if the batch has no pending upload or removed members, every member still belongs to the same root and batch, material/index characteristics force unmerged drawing, opacity classification remains compatible, and members are consecutive in the new render list. Alpha member order must remain increasing. Full list rebuilds preserve eligible old batches instead of recycling their buffers; partial rebuilds apply the same eligibility check. Merged geometry retains its existing path. This creates per-rebuild position maps, not a persistent pointer cache or cross-owner geometry handoff.

## Correctness evidence and limits

The expanded unchanged-subroot test passes all eight rows plus setup/cleanup on the prototype. It checks unchanged pixels after partial/full rebuilds, geometry mutation, recoloring, deletion, unrelated root growth/removal, interleaving a green translucent leaf between the two blue leaves, and reordering members. Interleaving/reordering must invalidate uploads. The composited interleaving pixel is checked numerically. Affected geometry leaves have physical-grid origins and unit scene transforms at 175%; the rendered overlapping blue leaves were inspected in retained-unmerged-root-175.png.

Full native fixture invocations on Direct3D and OpenGL each report 22 passes and four failures. The four failures are the existing upload-admission capability fixtures, which the current renderer and this prototype do not provide. This is not a green full-suite claim. No final promotion is justified yet: opaque membership, material/blending reclassification, and wider real-SVG behavior still need explicit coverage before broadening or shipping the policy.

## Clean wheel comparison

Baseline includes the production timer-settling fix and buffer-fit renderer; candidate adds only this retention prototype. Normal default GC budget 5000 us, Downloads 184 entries, active 160 Hz, DPR 1.75. Every run has explicit 30 ms wheel input: A1/B1 have 268 events, B2/A2 have 267. No sampler, observer, build or tests ran during these clean captures. Display intervals use strict same-direction endpoint windows.

| Run | Down display p99 / max ms | Up display p99 / max ms |
| --- | --- | --- |
| A1 baseline | 9.4475 / 18.7454 | 12.4900 / 15.6662 |
| B1 candidate | 12.4794 / 18.7423 | 9.5012 / 18.7576 |
| B2 candidate | 9.5564 / 12.4972 | 9.5581 / 12.5137 |
| A2 baseline | 9.6130 / 18.7427 | 9.6160 / 12.5029 |

Results are mixed and do not establish a repeatable display-cadence gain. A launch-time attempt to reset command.json was briefly denied by a file-sharing conflict before B2; the actual capture completed, has the correct ID/path/mode and 267 accepted wheel events. It is retained rather than silently discarded.

## Source-probe result

The separate candidate attribution capture multi-unmerged-upload-wheel-masonry has 268 wheel events and the membership observer enabled. Large 36-element uploads occur three times, versus five in the earlier current-renderer attribution capture; the repeated 36-element first-node identities disappear. Repeated 12- and 4-element first-node identities remain. Recorded upload events are 56 with 13,134,336 vertex-buffer bytes; these are observer records, not an OS allocation or universal transfer total. The candidate's two worst render scopes are 11.4161 and 9.9817 ms with recorded vertex sizes of 4,855,752 and 5,644,268 bytes. Initial geometry still costs work; source-probe durations are not substituted for clean cadence.

This supports that the implementation reaches the largest repeated groups, while its conservative consecutive-member requirement may exclude interleaved groups. It does not yet prove why each remaining group is rejected or whether its geometry is unchanged.

## Next work and state

Instrument rejection reasons and geometry/material invalidations for the remaining groups. A possible broader invariant is that the entire render-list sequence spanning a group's first and last member remains identical across the rebuild, even when other unchanged batches interleave. That requires a new failing regression and robust alpha-order/material tests; simply removing the adjacency check would be unsafe.

The prototype is not in the portable host, normal Qt archive, dependency patch, or launcher. The native test source was restored after archiving its regression patch; the worktree app is restored to the existing timer-fixed build. The persistent 160 Hz objective is unfinished.

Evidence and reproducible prototype sources are in profiler/experiments/multi-element-unmerged-retention-2026-10-18. Raw captures remain under artifacts/modes-profile/multi-*.

AI assistance has been used to create this output.
