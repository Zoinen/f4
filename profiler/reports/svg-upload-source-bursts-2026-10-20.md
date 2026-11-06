# Whole-source SVG upload bursts, 2026-10-20

This is an unpromoted renderer prototype. The normal worktree app remains the
curve-preprocess build (launcher 500be90b, Qt host 2e58e03b); no production recipe,
embedded payload or running app was replaced in this investigation. Constant
160 Hz without spikes remains incomplete.

## Implemented prototype

The previous chunking prototype split unmerged SVG uploads above 512 KiB into
512 KiB static-buffer updates, with 1 MiB of admission per frame. This work:

1. Raises chunk eligibility to payloads above 1 MiB. Moderate sources retain the
   normal upload path, avoiding unnecessary static staging.
2. Starts a timer at batch preparation entry and checks a 3 ms soft deadline
   during admission, so earlier list/batch/upload work consumes that budget.
3. Allows the first visible source a bounded 4 MiB burst, still in <=512 KiB
   steps. Small-only and background admission retain the normal 1 MiB cap.
4. Keeps trailing small batches from that same source within its burst. Other
   sources do not inherit the enlarged cap. Priority/sequence/order sorting
   remains in effect. Incomplete sources stay hidden until fully prepared.

The deadline is not a hard bound on full frame time: admission permits one
minimum progress step, individual driver calls cannot be preempted, and later
material preparation/recording are outside its checks. This distinction matters
in the final outlier below. No raster fallback, CPU geometry shadow, source-read
change or expansion of the materialization window was added.

## Three controlled iterations

Each pair uses one executable and matching successful-link receipts; the only
switch is F4_PROFILE_DISABLE_VECTOR_CHUNKS. Different pairs use different builds
and must not be treated as a single causal before/after measurement.

All captures: Downloads, 186 entries, Masonry, DPR 1.75, 160 Hz, middle-button
pointer approximately one quarter window height below the origin, bouncing at
folder bounds, four 10-second input windows. No builds, tests, offline analysis
or UIA observer ran during input. All 24 windows across these three pairs passed
strict middle-motion gates and had zero missed display intervals (>9.375 ms).
Producer-present outliers remain distinct from actual display misses.

| Pair | Preparation max, disabled/enabled | Warm eligibility median, disabled/enabled | Warm eligibility p95, disabled/enabled |
|---|---:|---:|---:|
| chunk-deadline | 4.6327 / 3.3987 ms | 18.5355 / 24.9466 ms | 37.1130 / 43.0651 ms |
| chunk-burst | 4.5379 / 3.2978 ms | 18.3632 / 23.4486 ms | 31.5277 / 36.7320 ms |
| chunk-tail | 4.1990 / 5.0498 ms | 18.2383 / 6.0235 ms | 36.3102 / 23.2114 ms |

Eligibility is the first priority<=100 demand event to first ready+present event
on the render thread. It is not file-open-to-monitor latency. The probe records
only the first episode per source sequence, not every later invalidation. Each
phase has 82 completed demands and zero censored demands in all three pairs.

The first two revisions reduced observed preparation maxima while adding roughly
one frame to warm eligibility median. Inspection found some roots remained
unready 12-19 ms after their last chunk completed. The burst cap was applied only
to a large batch, then reverted to 1 MiB for its small trailing batches. Because
the source had already exceeded that cap, those batches waited another frame.
The final revision keeps the same source within its bounded burst until its tail
is handled. This is a queueing fix, not an increase to the 4 MiB ceiling.

## Regression evidence

A focused native regression fails on the preceding burst renderer on both D3D11
and OpenGL: the large transfer completes in frame 2, but source readiness is
published in frame 3. With the fix it completes in the same frame. It checks final
pixels for both the large red geometry and small yellow tail, scene origins and
unit transforms at DPR 1.75, and saves the rendered output.

The final prototype passes 50 QtTest checks per backend. Coverage includes:

- Whole-source visibility, painter order, source priority and readiness.
- Large transfer completion, replacing the source during transfer, multiple
  unmerged nodes, and in-place geometry mutation while a transfer is partial.
- A 640,024-byte moderate source that creates no chunk staging records.
- Existing pool, cache, subroot-order and hidden/visible regressions.

The enlarged-burst test initially retained a 3 MiB multi-node fixture while
asserting at least three frames. That fixture now fits inside one burst. It was
increased above 8 MiB so the original >=3-frame assertion still tests a real
multi-frame transfer. The failed intermediate test log is retained. A temporary
const-pointer compilation error was corrected before successful builds; those
logs are also retained. Neither failure was hidden by relaxing the assertion.

## Final pair: result and remaining spike

Final public preparation p99 (disabled -> enabled):

- Cold block 0: 2.0014 -> 2.0173 ms.
- Cold block 1: 1.6542 -> 1.8687 ms.
- Warm block 0: 1.7349 -> 1.8126 ms.
- Warm block 1: 1.6193 -> 1.7785 ms.

Loading eligibility improved substantially, but preparation p99 did not. The
final enabled cold preparation maximum reached 5.0498 ms, versus 4.1990 ms in
its control. No general frame-time improvement or full-goal completion is claimed.

The correlated cold outlier contains 4.7481 ms / 15,980,340 CPU cycles inside
batch preparation. Its admission scope took 1.4791 ms / 4,898,380 cycles, ordinary
alpha uploads 0.9354 ms / 3,171,860 cycles, alpha-batch construction 0.5699 ms /
1,915,900 cycles, and later alpha/material preparation 1.3503 ms / 4,588,980 cycles.
Scopes nest, so these must not be summed indiscriminately. Three chunk copies
covering 1,466,640 bytes took 0.0904/0.0890/0.0854 ms. These facts rule out calling
the remaining spike a single oversized copy; the later preparation stage needs
more detailed attribution. CPU cycles are not converted to wall time.

All recorded final transfer steps stayed <=524,288 bytes. The largest per-frame
chunk total was 2,241,216 bytes, within the 4 MiB ceiling. Cold/warm chunk CPU
maxima were 0.4902/0.1690 ms. These cover consolidation/queueing, not subsequent
GPU/driver execution.

## Remaining gates

Before any promotion: attribute the later material/preparation tail, repeat the
performance comparison, exercise rapid-wheel input, and verify real SVG features
and live viewer priority/latency. Then integrate strict portable source patches
and tests, rebuild the real library/embedded payload, run isolation/dependency/
extraction gates, and measure that exact production source. The earlier rapid
wheel phase-error failure and isolated OpenGL heap incident remain recorded.
The native geometry tests do not replace these broader gates.

Evidence prefixes are real-scroll-chunk-deadline-*, real-scroll-chunk-burst-* and
real-scroll-chunk-tail-*. Source snapshots before later edits are preserved in
chunk-deadline-before-burst/ and chunk-burst-before-tail-fix/. The final detailed
outlier is chunk-tail-cold-preparation-outlier.json; transfer bounds are in
chunk-tail-transfer-bounds.json. Owned old CSVs were losslessly compressed and
hash-verified to recover space. No commit or push was made.
