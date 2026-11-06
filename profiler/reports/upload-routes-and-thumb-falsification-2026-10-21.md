# Masonry upload routes and scrollbar-thumb falsification — 2026-10-21

Follow-up: `batch-invalidation-and-thumb-isolation-2026-10-21.md` identifies the thumb as the dominant source of invalidation calls. The freeze below suppressed only its native matrix write; GUI-side positioning continued. It did not establish that the thumb was stationary. The follow-up distinguishes those invalidation calls from the larger population of content uploads.

Status: diagnostic attribution. No production behavior, dependency recipe, embedded payload or running normal app was changed. Constant 160 Hz remains incomplete.

## Question

The preceding stage probe found a 12.07 ms preparation tail with 6.02 ms in upload/admission and 2.64 ms in batching. This capture distinguishes ordinary opaque uploads, ordinary alpha uploads, and budgeted pending-vector uploads. It records each actual transfer after the renderer's unchanged/invalid/render-node early exits, with separate map, copy and unmap timestamps. Gate identity/readiness/presence and material type are sampled alongside exact requested buffer sizes (`Buffer::size`, not pooled QRhiBuffer capacity).

The timer excludes ancestor lookup and metadata collection. Those operations still add observer cost; this instrumented executable is for attribution, not a claimed production speed comparison. Map/unmap scope wall time includes allocation, resource-update bookkeeping, copying and possible preemption. It must not be called GPU execution time or a GPU wait without more evidence.

## Upload-route results

Accepted prefix: `real-scroll-upload-route-20261021`, Downloads, 187 entries, density 152, DPR 1.75, 160 Hz, quarter-window middle input, cold/warm two approximately ten-second blocks each. Focus, bounded recorders, source hashes and route classification checks pass. No build, test, offline analysis or UIA observer runs during input.

| Pass/block | Actual upload calls | All measured upload time | Unmap time | Copy time |
| --- | ---: | ---: | ---: | ---: |
| Cold / 0 | 15,901 | 93.68 ms | 81.64 ms | 9.52 ms |
| Cold / 1 | 16,901 | 80.21 ms | 68.58 ms | 9.18 ms |
| Warm / 0 | 16,701 | 78.37 ms | 66.49 ms | 9.43 ms |
| Warm / 1 | 16,486 | 80.22 ms | 68.09 ms | 9.71 ms |

Every observed curve fill/stroke transfer is in pending-vector admission. There are no recorded ordinary ready-gate curve transfers in these input windows. The large SVG transfers are therefore not bypassing the gate in this trace.

Ordinary alpha uploads dominate call count: smooth-color geometry, text masks, texture quads and shader-effect quads have no vector gate. For example, cold block 0 has 6,070 smooth-color, 4,630 text-mask, 2,701 opaque-texture-material and 2,298 shader-effect uploads. Here “opaque texture material” is the material class, not the opaque upload route; these calls are in the alpha route.

The largest warm preparation frame in block 0 is 3.5182 ms. It includes 155 ordinary small uploads totaling 0.4491 ms and one pending vector stroke upload taking 1.5092 ms. That stroke contains 2,054,448 vertex bytes and 186,768 index bytes. The production admission policy permits the first progress batch even when it exceeds the nominal 1 MiB aggregate limit; it cannot preempt that transfer. Thus admission is a soft budget, not a hard per-transfer ceiling. The earlier chunking prototype remains unpromoted and is not part of this host.

The cold trace's stage-zero rebuild flags are `BuildBatches` (4) in 1,486 frames and `BuildBatches | BuildRenderListsForTaggedRoots` (5) in 114 frames. There are no full render-list rebuilds in that input block. This directs the next investigation toward batch invalidation during ordinary transforms, geometry/material updates and opacity updates; a new full-list merged-batch cache would not address most of these frames.

## Thumb-movement falsification

A second diagnostic executable uses exactly the same renderer and instrumentation. An environment switch suppresses only the native scroll job's scrollbar-thumb matrix write. This intentionally freezes a visual control and is not a candidate product fix. The experiment tests whether that transform is the primary source of ordinary upload churn.

Cold layouts match exactly between arms (extent 4075.142857). Ordinary alpha upload counts per block:

| Arm | Cold block 0 | Cold block 1 |
| --- | ---: | ---: |
| Normal thumb | 15,669 | 15,743 |
| Frozen thumb | 17,832 | 17,373 |

Removing the thumb write does not suppress the churn. No causal performance benefit is established. Warm extents differ (normal 4093.142857, frozen 3623.142857), so warm timings are not used as a controlled comparison. The experiment is not promoted and no assertion was relaxed to accept the mismatch.

The original upload attribution's warm extent is 4093.142857, rather than the older captured 3623.142857. These are current cache/layout states, not an equivalent before/after baseline. The report does not attribute their difference to a source change.

## Verification and next action

- Probe tests: 4 passed, no failures/skips (route restoration, ordered timestamps, payload publication and overflow bounds).
- Actual D3D11 checks at DPR 1.75: 44 drawing-order and 6 curve tests passed, no failures/skips.
- Upload-route and thumb diagnostic hosts: 4 embedded-import/compiled-host tests each passed; only system DLL imports.
- Source hooks reverse exactly to the pinned production renderer. No SDK header/archive was modified.
- Only owned diagnostic launcher trees were stopped after capture. The normal worktree application remains running.

Next record batch-invalidation call-site causes, including cascading invalidations of overlapping render-order ranges. Identify the changing visual or transform before modifying reuse/invalidation. Preserve geometry/material/opacity updates and painter order; do not retain merged buffers across depth/order/transform changes on the basis of unchanged node membership alone. Oversized first admission and separate display-clock/submission tails also remain open.

Source, receipts, tests, capture packets, presentation CSVs and analysis are archived with SHA-256 manifest under `profiler/experiments/upload-route-attribution-2026-10-21`. Executables, object files and raw traces are excluded. No commit or push was made.
