# Retained batches: lifecycle checks and current GUI baseline

## Scope

The two GUI fixes are now committed in ZoinGallery `7ee85a9`, referenced by
parent `87258b8af`. This comparison uses those same sources for both hosts.
Only the Qt Quick library differs: the control has the current production
order-accounting/blocked-opacity fixes; the candidate has the previously
archived stable-interval/batch-retention implementation. Neither library has
the experimental dynamic initial-buffer policy or geometry-buffer handoff.

The renderer remains unshipped. Constant 160 Hz is not achieved.

## Correctness coverage

The new native fixture creates and removes a nested clip branch 32 times,
reparents it between two populated root intervals, hides and restores it, and
reparents it back. Both rectangular-scissor and nonrectangular-stencil cases
compare rendered pixels at every state. A pixel assertion distinguishes the
actual trapezoid from its rectangular bounds. The stencil capture was inspected.
Every rendered state checks surviving root ranges, parent containment, and
geometry orders. The global maximum order range stays bounded after warmup.

The lifecycle fixture passes on the production renderer before applying the
prototype (four results including init/cleanup). The prototype passes all ten
native results, including the existing merged/unmerged retention, changed-order,
mutation, opacity and reservation checks. These ten also pass using an actual
OpenGL 4.6 context on the NVIDIA RTX 4090. Actual window DPR is 1.75.

The preceding focused SVG run against the same candidate library passes 26
results, including surface swaps, transparent gradients and embedded images,
source replacement/cancellation, animation visibility, mixed failure skipping,
successful-only prefetch counting, and all five panel modes. The existing
authored-animation pause warning remains; it is not a new failure.

Logs under artifacts:

- `clip-lifecycle-baseline-175.txt`
- `clip-lifecycle-candidate-175.txt`
- `clip-lifecycle-candidate-opengl-175.txt`
- `current-renderer-svg-175.txt`

The expanded test patch is archived in
`profiler/experiments/stable-renderer-lifecycle-2026-10-18`. It is not enabled
against the production dependency. No arbitrary-driver or cross-platform
correctness claim follows from these checks.

## Clean displayed-frame measurements

Both hosts use unique compiled main objects and the same basic profiling
hooks. The harness waits for the requested session path, and the runner rejects
a wrong or non-scrollable workload. All four runs confirm Downloads, 184 entries,
density 152, initial extent 4161.142857, actual DPR 1.75, active window and
160 Hz screen. Caption measurements remain zero.

Each fresh process receives wheel delta 120 every 30 ms, four seconds downward
then four seconds upward. PresentMon records displayed intervals. No build,
test, UI Automation observer, source-scope observer, or CPU sampler runs during
capture. There is no order-padding override. Display intervals over 9.375 ms
are counted as long; the actual target remains 6.25 ms per frame.

| Run | Down display p99 | Down maximum | Down long intervals | Up display p99 | Up long intervals |
|---|---:|---:|---:|---:|---:|
| Control A1 | 18.7518 ms | 25.0212 ms | 24 | 6.3474 ms | 6 |
| Candidate B1 | 12.4942 ms | 25.0024 ms | 10 | 6.3203 ms | 4 |
| Control A2 | 18.7611 ms | 37.4891 ms | 26 | 12.4933 ms | 8 |
| Candidate B2 | 12.5175 ms | 25.0120 ms | 13 | 6.3161 ms | 5 |

Downward GPU-active p99 changes from 10.8293/9.4170 ms in controls to
6.1510/6.9259 ms in candidates. This is PresentMon's GPU-active metric, not
an isolated upload timer. The result supports reducing unnecessary renderer
work but does not independently attribute every remaining stall to uploads.

The four capture stems are `current-interval-a1`, `-b1`, `-a2`, and `-b2`, each
followed by `-wheel-masonry`, under `artifacts/modes-profile`. Each has native
JSON, PresentMon CSV, and a displayed-frame summary. The hosts are
`f4-qt-host-current-interval-control.exe` and
`f4-qt-host-current-interval-candidate.exe`, built with the same guarded harness.

## Implication and remaining work

Both pairs reduce long downward display intervals and p99; this repeats the
earlier direction of improvement with the latest GUI fixes. Upward differences
are smaller and variable. Candidate maximum displayed intervals still reach
25 ms, four display periods, so these measurements contradict completion of
the 160 Hz objective.

The next implementation step is strict, reproducible dependency integration
with source-anchor and composition checks, followed by the portable build
gates. First-time large SVG geometry uploads remain a separate known source of
frame spikes. Any upload-admission change must retain complete old content,
preserve clipping/blending/order, and keep requesting frames until work is
complete; simply withholding unuploaded geometry would produce missing images.

## Restored development state

The expanded patch passes `git apply --check` against the restored parent test
source. The normal native target is rebuilt and linked to the production Qt
library again; all four normal results pass at DPR 1.75 in
`artifacts/clip-lifecycle-normal-restored-175.txt`.

The worktree app is restarted directly with `--gui=qt` after restoring the
profiled panel state. It has no diagnostic host, scale, profiling or order-padding
override. Its extracted host SHA256 is
`39de3aae0377420abedcdaeb0aa420bfce2afb27ffdaa8dcc94d03ad8451eb82`, the expected
committed-Gallery build, and the window title identifies
`zoin_branch/cached-pixel-grid`. Startup stderr is empty. Identity is recorded
in `artifacts/renderer-lifecycle-normal-identity.json`. This turn does not
replace the normal renderer or regenerate the portable payload.

AI assistance has been used to create this output.
