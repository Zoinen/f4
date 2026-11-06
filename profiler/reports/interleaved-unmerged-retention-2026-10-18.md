# Interleaved unmerged geometry retention — 2026-10-18

## New attribution

The rejection observer ran on the preceding consecutive-member prototype with real 30 ms wheel input. It records 50 large-batch rejections: 24 four-member, 22 twelve-member, two six-member and two eleven-member groups. All recorded rejections are reason 3, the non-consecutive-members predicate. Two frames overflowed the bounded detailed event array, so counts are not asserted complete and missing geometry invalidation events cannot establish unchanged geometry.

## Regression and implementation

Two new native rows create an alpha batch with two blue members and an unchanged, non-overlapping translucent green item interleaved in the render list. Membership is asserted. The consecutive-member prototype fails both partial and forced-full rebuild cases at cumulative upload count: actual 3, expected 2, at actual DPR 1.75. An earlier fixture used an opaque green item, which was in a different render list and did not reproduce the condition; the final regression explicitly uses translucent green.

The new isolated prototype snapshots the old and new render-list sequences and their positions. It retains raw unmerged buffers only when every member survives, ownership and root remain valid, pending geometry uploads are absent, unmerged characteristics persist, material compatibility and opacity classification remain valid, alpha member order remains increasing, and the entire sequence spanning first through last member is identical. Intervening elements are included; a new, removed or reordered outsider cannot bypass normal rebatching. No persistent pointer cache, per-frame ancestor traversal, sorting pass or cross-owner buffer handoff is added. Sequence snapshots occur on rebuilds, not every frame.

Expanded native invocations on Direct3D and OpenGL each pass 24 results, including setup/cleanup. They cover the new interleaved cases, moving the existing outside item into overlap, inserting another overlapping translucent item, member reordering, geometry changes, recoloring, alpha/opaque material transitions, removal, clip lifetimes, root reservation accounting and pooled-buffer reuse. Leaf scene origins and unit transforms are checked at DPR 1.75; the rendered overlapping leaves and small non-overlapping green item were inspected. These are focused tests, not a full-suite or cross-platform claim.

## Source probe

The first span capture lost focus partway through down scrolling and stayed inactive during up scrolling. Its timing is excluded from active-window comparisons and the data is retained. The repeat was active throughout, 268 wheel events at 30 ms, Downloads 184 entries, DPR 1.75, D3D11, 160 Hz and default 5000 us GC budget.

Aggregate upload counters (not only the saved slow-event subset):

| Attribution capture | Upload calls | Vertex bytes | Index bytes |
| --- | ---: | ---: | ---: |
| Earlier current renderer | 10080 | 27965896 | 3086732 |
| Consecutive-member prototype | 9830 | 23870424 | 2705820 |
| Span prototype, active repeat | 9534 | 22329616 | 2544220 |

The active span repeat has no repeated large first-node groups in the saved event records, including the preceding recurring 12/4-member groups. Upload event records below 50 us are suppressed; therefore absence from this subset does not prove absence of every fast repeated upload. Aggregate counters include those calls. None of these three captures overflowed their detailed event arrays. Different-run byte totals are diagnostic evidence, not a clean cadence comparison.

The span repeat's worst render scope is still 9.8385 ms, with a full render-list rebuild. Its upload scopes include 367 calls, 5,056,252 vertex bytes and 476,712 index bytes, totaling 5.1366 ms inclusive. PrepareRenderPass is 7.7662 ms; buffer growth/recreation scopes total 1.4094 ms and dynamic copies 2.4445 ms. These nested wall times must not be summed or called pure CPU time. This points to both initial uploads and collateral merged-batch work on full order rebuilds remaining after large-group retention.

## Clean A/B/B/A

A is current production renderer with timer-settling and buffer-fit fixes; B adds span retention. No observer, sampler, build or tests run during these captures. All windows active; actual 30 ms wheel input (A1/B1/B2 268 events, A2 267), default GC 5000 us, D3D11, DPR 1.75, 160 Hz, 184 entries. Both actual display endpoints must fall within the same direction's capture window.

| Run | Down display p99 / max ms | Up display p99 / max ms |
| --- | --- | --- |
| A1 | 9.2593 / 19.6553 | 9.2758 / 12.5174 |
| B1 | 11.5147 / 18.7569 | 9.3321 / 9.4232 |
| B2 | 9.3702 / 21.8619 | 9.3845 / 12.5306 |
| A2 | 9.4626 / 21.9329 | 7.3763 / 12.4969 |

No repeatable cadence improvement is established. The prototype is not promoted to the normal Qt archive, dependency patch, host or launcher. Constant 160 Hz without spikes remains unachieved.

## Next experiment and restoration

The evidence supports combining this tested retention invariant with bounded initial upload admission and the previously tested bounded drawing-order reservation policy. Admission alone did not cover ready-group reuploads; retention alone does not bound initial multi-megabyte uploads or eliminate global merged-depth reuploads when order space is exhausted. The combination needs its own correctness and clean performance checks, rather than inheriting a performance claim from these separate pieces. A future counter probe should retain every large upload regardless of duration to verify per-node recurrence completely.

The native test source was restored after archiving the patch; 18 focused normal-renderer results pass again at 175%. The normal timer-fixed portable worktree app is restored, with no diagnostic override. Sources, patches, compact measurements, native logs and the rendered fixture are archived under profiler/experiments/interleaved-unmerged-retention-2026-10-18. Raw capture data remains under artifacts/modes-profile.

AI assistance has been used to create this output.
