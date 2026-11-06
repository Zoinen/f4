# Cold-scroll uploads after native batch retention

## Workload

The baseline is the integrated stable-interval renderer with the latest
ZoinGallery GUI fixes. All clean captures use Downloads, 184 entries, Masonry
density 152, initial extent 4161.142857, actual DPR 1.75 and an active 160 Hz
window. Wheel delta 120 repeats every 30 ms for four seconds down and four
seconds up. Path and nonzero-extent guards validate the selected panel.
Caption measurements remain zero.

Clean runs use only the basic frame hooks and PresentMon. There is no compiler,
test run, source observer, CPU sampler or UI Automation observer during capture.
Each comparison uses fresh processes, with panel state restored before restart.
Runs are ordered A1/B1/B2/A2 to reduce ordering bias. Diagnostic timings below
have additional scopes and are not clean frame-rate measurements.

Neither experiment is shipped. The 6.25 ms target remains unmet.

## Current attribution

`current-upload-probe-wheel-masonry` adds bounded render-thread scopes to the
current renderer, including whole prepareRenderPass and resourceUpdate calls.
No hardware-driver internals are instrumented.

One slow frame spends 12.052 ms in render CPU work and 10.286 ms inside
prepareRenderPass. It uploads 398 batches, with 7.599 ms total in uploadBatch,
6.509 ms in unmap, and 3.868 ms in 480 buffer creations. Upload memory recorded
by unmap is 10,977,344 bytes. These scopes are nested and must not be added.

Another frame spends 10.659 ms in render CPU work. A single new 511,896-byte
immutable vertex-buffer create takes 6.352 ms wall time; its enclosing unmap
takes 6.517 ms. This does not prove all that time is CPU execution or all future
allocations of that size will stall. It does show why a byte budget alone cannot
guarantee a hard upper bound on a driver call.

Across this diagnostic capture, resource-update recording peaks at 0.162 ms,
pipeline creation at 0.223 ms, and partial-list rebuilding at 0.502 ms. Full-list
building occurs twice and peaks at 0.877 ms; its consequence includes subsequent
geometry uploads, which are counted separately. The remaining bursts are not
explained solely by the CPU list traversal or source acquisition.

## Initial large dynamic allocations

Only initial D3D11 vertex/index allocations of at least 64 KiB use Dynamic in
the candidate. Other backends, small allocations, pooling and ordinary update
logic are unchanged. The intent is to test the allocation/upload hint without
changing which complete image is drawn.

| Run | Down display p99 | Down maximum | Down intervals >9.375 ms | Up display p99 | Up intervals >9.375 ms |
|---|---:|---:|---:|---:|---:|
| Control A1 | 12.5050 ms | 18.7742 ms | 12 | 6.2997 ms | 2 |
| Candidate B1 | 12.5033 ms | 18.7866 ms | 12 | 6.3481 ms | 2 |
| Candidate B2 | 12.5259 ms | 18.7611 ms | 19 | 6.3153 ms | 2 |
| Control A2 | 12.5099 ms | 18.7904 ms | 14 | 6.3259 ms | 2 |

The first pair ties and the second has more long downward candidate intervals.
There is no demonstrated benefit sufficient to justify a new memory policy.
The change is archived rather than integrated. Capture stems are
`large-dynamic-a1`, `-b1`, `-b2`, `-a2`, each followed by `-wheel-masonry`.

## Single-element unmerged retention across order changes

Qt's unmerged upload path copies raw geometry. Unlike merged buffers, it does
not bake world transformations or normalized drawing-order depth into vertices.
The trial therefore retains an unchanged single-element unmerged batch when
it still belongs to the same root and render partition and is permanently
unmergeable (32-bit indices or NoBatching). Dirty geometry, removed elements,
changed roots/partitions, render nodes and visualizer modes remain invalidated.
The batch keeps its own buffers and updates its last-order bookkeeping.

This handles both partial renumbering and a full list rebuild. It is distinct
from the rejected geometry-buffer handoff/cache experiments: no buffer moves
to a new batch, and merged depth data is not reused across normalization changes.

The initial native fixture grouped its two same-color rectangles into one
multi-element unmerged batch, outside the deliberately limited optimization.
Different colors make the fixture exercise single-element batches. The refined
order-change and forced-full-rebuild rows fail on the normal renderer (six
passes, two failures) and pass on the candidate (eight passes). All 12 expanded
native results pass on the default Windows renderer and OpenGL at DPR 1.75,
including pixel comparisons, geometry/color mutation, clipping and lifetime
checks. The focused SVG suite passes 26 results. Its existing authored-animation
pause warning remains. Native runs use `-maxwarnings 100000` so the lifecycle
fixture does not suppress later renderer diagnostic output.

| Run | Down display p99 | Down maximum | Down intervals >9.375 ms | Up display p99 | Up intervals >9.375 ms |
|---|---:|---:|---:|---:|---:|
| Control A1 | 12.5026 ms | 25.0066 ms | 13 | 6.4049 ms | 6 |
| Candidate B1 | 12.5155 ms | 18.7523 ms | 18 | 6.2829 ms | 1 |
| Candidate B2 | 12.5088 ms | 24.9622 ms | 16 | 6.2888 ms | 2 |
| Control A2 | 12.4988 ms | 25.0183 ms | 15 | 6.3291 ms | 2 |

The native regression proves unnecessary uploads can be removed in those
specific scenarios. It does not prove a cold-scroll improvement: both clean
pairs have slightly more long downward intervals and essentially unchanged
p99. The warm return results are mixed. This trial remains archived too.
Capture stems are `single-unmerged-a1`, `-b1`, `-b2`, `-a2`, each followed by
`-wheel-masonry`.

## Next mechanism

These results narrow the next change to bounded first-time preparation/upload
admission, rather than another allocation hint or general retention expansion.
The existing one-source-assignment-per-frame rule does not bound later worker
completions or render-thread uploads. Readiness currently reports completed CPU
Shapes, not completed native uploads. Zero-opacity pending viewer scenes do
not warm their GPU buffers.

A correct admission mechanism must keep an outgoing viewer surface (or the
ordinary panel icon) until the replacement can be presented completely. It must
keep requesting frames for deferred work, prioritize the visible viewer and
current viewport, reject stale revisions, and cancel detached sources without
leaking leases or buffers. Hidden preparatory scenes must actually reach native
upload without painting over the visible scene. It must measure source-to-first-
complete-paint latency alongside frame tails, so improved frame statistics
cannot hide unbounded image delays. This is unfinished design/implementation,
not a claim that simply skipping unuploaded batches would be acceptable.

## Reproduction artifacts

Renderer deltas and the native test patch are archived in
`profiler/experiments/cold-upload-policy-2026-10-18`. Native logs include
`single-unmerged-before-refined-175.txt`, `single-unmerged-after-refined-175.txt`,
`single-unmerged-native-all-175.txt`, `single-unmerged-native-opengl-175.txt`, and
`single-unmerged-svg-175.txt` under artifacts. Each clean capture has native JSON,
PresentMon CSV and a displayed-frame summary under artifacts/modes-profile.

## Restored state

The native test source is restored to the preceding integrated fixture and its
modification time refreshed so Ninja recompiles it rather than linking the
newer experimental object. All ten normal native results pass at DPR 1.75 in
`artifacts/cold-upload-normal-restored-175.txt`. The session test executable is
also relinked with the normal recipe library. The local imported library remains
`artifacts/qt-stable-intervals-recipe/Qt6Quick.lib`.

After restoring the profiled panel state, the worktree app restarts directly
with `--gui=qt` and no diagnostic host, profiling, backend, order-padding or
scale overrides. The extracted host SHA256 is still
`a6a5aa0c5951a58621278e55de0396ec85fdddb8f2583ae74ac4ff2e749629b4`, matching the
preceding integrated portable build. Its title identifies the worktree and
startup stderr is empty. Identity is saved in
`artifacts/cold-upload-normal-identity.json`. Neither experimental allocation
nor expanded retention policy enters the payload or running normal application.

AI assistance has been used to create this output.
