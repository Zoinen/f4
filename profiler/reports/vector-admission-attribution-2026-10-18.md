# Remaining upload stalls and combined retention trial

## Diagnostic attribution

The current experimental admission renderer was instrumented separately from
clean display-cadence measurements. Scopes measure wall time and include
nested work; their durations must not be added together or described as pure
CPU execution time. Upload scopes distinguish registered pending groups,
registered ready groups and geometry with no registered ancestor at that
instant. These are admission categories, not image-format classifications.

The guarded Downloads workload remains 184 entries, density 152, extent
4161.142857, actual DPR 1.75, 160 Hz and an active window. The source cache is
not purged. Each fresh process scrolls down and up for about four seconds,
using 120-unit wheel events every 30 ms.

In admission-attribution-wheel-masonry, the worst captured preparation frame
takes 22.2137 ms before render-pass recording, with a full rebuild. Its scoped
prepareRenderPass time is 21.8270 ms. Geometry upload scopes total 19.2876 ms
over 360 calls. Of these, ready registered surfaces account for 8.6571 ms over
40 calls, and unregistered geometry for 10.4170 ms over 320 calls. Buffer unmap
work totals 17.8943 ms for 11,521,944 bytes, including 12.8707 ms in buffer-create
calls. One 880-byte immutable buffer-create call takes 7.5928 ms wall time.
That call duration alone does not distinguish driver work from preemption.
Gate discovery is only 0.0030 ms in that frame. Resource-update submission is
0.0811 ms. The initial-admission limit does not cover the dominant ready and
unregistered work.

A second frame has 13.0403 ms before recording and 10.3359 ms inside material
dynamic-data updates; a single 96-byte-region update accounts for 9.6836 ms.
No conclusion about which subcall caused that isolated stall follows from the
outer scope alone.

A follow-up, admission-fine-wheel-masonry, adds uniform-shader,
sampled-image and dynamic-buffer-update scopes. It does not reproduce that
single 9.7 ms material-update stall. Its two worst preparation frames still
spend 6.9528 and 8.2210 ms in uploads, including 3.1278 and 6.2908 ms in ready
registered groups. Uniform shader callbacks total only 0.0994 and 0.0869 ms;
sampled-image callbacks total 0.2740 and 0.7758 ms. This repeat supports the
ready-upload problem without proving that the earlier material stall was an
application algorithm rather than a driver or scheduling delay.

The separate admission-owners-wheel-masonry run maps QML/scene ancestry on
every sync to identify surfaces. It confirms ready SVG previews among the
large repeated uploads. The observer itself pushes sync p95 to 42.56 ms;
its cadence is therefore excluded from performance comparisons. It does not
establish the identity of every unregistered upload in the earlier full frame.
The detailed event array is bounded; aggregate scopes remain the basis for
call totals, not the subset of saved slow-event records. Owner-scope byte fields
are capacities observed before upload and must not be treated as actual bytes
transferred; the unmap counters above measure the relevant upload sizes.

## Combined experiment

The earlier single-element unmerged retention trial did not demonstrate a
scrolling gain alone. The new ready-upload evidence motivates testing it with
admission and transform-root ownership, rather than assuming the earlier
negative result changes automatically. The combination keeps a permanently
unmergeable single-element batch's original raw geometry buffers across order
renumbering/full list rebuilds. Merged buffers and dirty geometry retain their
normal invalidation. The self-contained source patch is archived under
profiler/experiments/vector-admission-retention-2026-10-18.

The native test restores the distinct-material single-element and forced-full
rows from the earlier experiment. Against admission alone, the order-change
selection gives 6 passes and 2 failures: unmerged-order-change and
unmerged-full-order-change both upload unnecessarily. Against the combination,
all 16 native results pass on default Windows RHI and OpenGL, including the
admission, overflow, pixel-comparison, mutation and 16-cycle root-lifetime
fixtures. The 26 focused SVG results also pass, including explicit native
capability assertions. These are correctness checks, not performance proof.

Local evidence: admission-retained-before-175.txt,
admission-retained-native-175.txt, admission-retained-opengl-175.txt and
admission-retained-gallery-175.txt under artifacts.

## Clean display measurements

C is admission with root intervals; D adds single-element unmerged retention.
These hosts have no tracing/owner observer enabled. No build, tests or sampler
runs concurrently. Panel/persistence state is restored before each restart.
Caption measurements remain zero. PresentMon display-change intervals are the
headline; the diagnostic scope runs above are not substituted for these data.

| Run | Down p99 ms | Down maximum ms | Down intervals >9.375 ms | Up p99 ms | Up intervals >9.375 ms |
| --- | ---: | ---: | ---: | ---: | ---: |
| C1 | 12.4888 | 18.7579 | 8 | 6.3177 | 3 |
| D1 | 12.4914 | 18.7522 | 8 | 6.3661 | 4 |
| D2 | 12.4849 | 18.7580 | 7 | 6.3050 | 2 |
| C2 | 12.4993 | 18.7754 | 12 | 6.3241 | 2 |

The first pair ties; the reverse-order pair improves the downward long-interval
count. Neither lowers p99 below approximately 12.5 ms or eliminates spikes.
The combination remains experimental. These results do not overturn the
earlier negative standalone retention trial or justify claiming constant
160 Hz. Local capture stems are admission-combo-{c1,d1,d2,c2}-wheel-masonry;
artifacts/admission-combo-summary.json combines the display summaries.

## Remaining investigation and state

Determine why ready surfaces still need new buffers: real geometry/material
changes, root replacement, CPU Shape completions arriving after readiness, or
other invalidation. The current code latches prepared/native readiness while
cpuComplete can change independently; a timing bug is a hypothesis requiring
live transition evidence, not a verified diagnosis. Any fix must retain whole
surface presentation, animations, source-revision cancellation and bounded
image latency. Simply hiding late work or delaying it indefinitely cannot
satisfy the objective.

The normal dependency recipe and embedded payload remain unchanged. The normal
worktree app is restored after profiling, with diagnostic overrides cleared.
No new commit or push is made. Portable rollout and the all-frame 160 Hz goal
remain unfinished.
