# Render-job startup registration boundary

## Cause and diagnostic change

Qt 6.11.1 starts pending render jobs in
`QQuickAnimatorController::beforeNodeSync()`. Starting the top-level animation
queues `QQmlAnimationTimer::startAnimations()`, which then queues
`QUnifiedTimer::startTimers()`. Meanwhile the controller can commit its running
jobs. The render loop advances the animation driver only when it is running.

The diagnostic candidate finishes the QQml registration stage after starting
pending roots, then delivers pending MetaCall events to that render thread's
specific QUnifiedTimer object before rendering. It does not integrate another
clock or pump the general application event queue. The old QQml queued call
later sees its pending flag cleared and becomes a no-op.

This is still a diagnostic Qt object override. Before production integration it
needs a strict source-recipe patch, a render-thread scope guard/basic-loop
compatibility decision, and broader generic-animation checks. The candidate has
not been embedded into the normal worktree app.

## Direct regression

A separate test source adds `renderJobRegistrationFinishedAtFirstSync`. During
the first render synchronization with the new native job, it records whether
the render thread's QQml timer still has registration pending.

- With the original Qt controller: pending is 1, expected 0; fails on D3D11/OpenGL.
- With the candidate: pending is 0; passes on both backends.
- The candidate suite passes 16 checks per backend, no failures/skips. Also covers
  retarget/reversal, identical-command deadlines, first handoff, precise finite
  motion, rewrap/shrinking extents, interruptions and hide/minimize, including
  the existing 175% resting leaf/transform/render comparisons.

The test and Qt controller are compiled as separate artifacts. Their original
archive members are removed for linking; duplicate-symbol suppression is not
used. The test's first build caught Windows' default text decoding of UTF8
source; explicit UTF8 reads/writes fixed it.

## Captured startup result

All runs use the same velocity-preserving prototype, Downloads186/Masonry152,
DPR 1.75, 160 Hz, 30 ms wheel events and boundary reversal. Each capture has four
10-second input windows. General frame recorder disabled; bounded phase/material
probes retained. No builds/tests/analysis/UIA during input windows.

| Capture | Native jobs, cumulative warm snapshot | Repeated zero-time commit pairs | Commits without precise sample |
|---|---:|---:|---:|
| Previous prototype | 13 | 11 | 24 |
| Candidate first run | 12 | 0 | 0 |
| Same-binary repeat | 10 | 0 | 0 |

Cold/warm JSON snapshots are cumulative; do not add their job counts. Across
the candidate's 80 seconds of input and 22 recorded jobs, the startup repeat is gone.
This corroborates the direct registration regression and the source ordering.

## Remaining late miss—retained in the result

The first candidate capture fails the overall motion guard: one warm-window
display interval is 12.4816 ms, followed by a dropped presentation/catch-up. The
joined camera-step error reaches 41.018 px. It occurs about 5.1 seconds into job 12,
not at startup. At the relevant producer frames there is no GUI synchronization;
the registration patch's beforeNodeSync body is not executing in those frames.
Pacing/sync portions are 6.14–6.54 ms and render-to-frame-end portions about 4.2 ms.
GPU active time around the affected presents is 0.41–0.68 ms. These distinguish
the late event from the startup registration stall, but do not establish its
ultimate scheduler/driver/render-work cause.

The same-binary repeat passes the p95<=0.5px / maximum<=2px motion guard, requiring
zero missed refreshes and complete presentation coverage:

| Repeat window | Max display interval ms | Motion error p95 px | Max error px |
|---|---:|---:|---:|
| Cold 0 | 6.3579 | 0.0912 | 0.7417 |
| Cold 1 | 6.4042 | 0.1248 | 1.1013 |
| Warm 0 | 6.3435 | 0.1038 | 0.6181 |
| Warm 1 | 6.3667 | 0.1533 | 0.8637 |

Both candidate captures and the failed guard output are preserved. A passing
repeat does not erase the first miss or prove the constant 160 Hz goal. The late
spike and broader coverage remain work.

## Workspace state and evidence

The diagnostic launcher tree was closed by exact executable path. The normal
worktree app remains responsive with the previously verified host hash:
`c27b27dad94a944219a9716131e71ff184b7773ba63a6700dd804512226fa81b`.
No production source or portable payload was changed, and nothing was committed
or pushed. Two older scheduler CSVs were losslessly compressed with verified
decompressed SHA256 equality to recover build space; evidence was retained.

The source snapshots, generated test/moc, actual-link receipts, test logs and raw
captures are indexed by
`../experiments/render-job-start-registration-2026-10-20/manifest.json`.
Next make the startup fix suitable for production integration and investigate
the late frame independently, while keeping velocity/reversal and scene bounds
correct. The original goal remains active.
