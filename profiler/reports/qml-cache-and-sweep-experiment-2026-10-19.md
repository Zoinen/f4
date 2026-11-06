# QML cache audit and incremental sweep experiment — 2026-10-19

The 160-Hz goal remains **unachieved**. The normal worktree app now uses freshly
regenerated application QML and Qt's original collector. The incremental sweep
prototype is **unsafe and not deployed**.

## Corrected build state

The source files `GalleryThumbnailShaderLayer.qml` and
`GalleryThumbnailCompactLayer.qml` contain no thumbnail lifecycle logging.
Their previously generated QML-cache C++ files nevertheless still contained
`F4_BITMAP_DESTROY`. The old normal host and both immutable offscreen-vector
comparison hosts contained `F4_BITMAP_SOURCE` and `F4_BITMAP_DESTROY` as UTF-16
literals. Attaching a debugger to the experimental host confirmed that the
handlers actually executed, rather than merely remaining unused strings.

The archived before-state records source/generated-file hashes and timestamps.
Restoring backup timestamps can leave newer generated units behind; the precise
restoration operation that caused this mismatch has not been established.
Empty redirected Windows stderr was insufficient evidence that these handlers
were absent: Qt sent their output to the attached debugger.

Every application/gallery QML source was touched without changing its bytes,
then the host and test executables were rebuilt. The new generated components
and native host contain neither logging marker. Both Direct3D 11 and OpenGL
pass the 87 selected checks at DPR 1.75, including actual leaf alignment,
rendered captions/icons, transitions, vector retention, bitmap reuse and
embedded-only imports. Rendered Masonry fallback output was inspected.

The Windows DLL audit passed. The embedded payload was regenerated, embedded
host materialization tests passed, and Go was rebuilt using `CGO_ENABLED=0`,
`GOWORK=off` and the system Go cache. Only the worktree launcher was replaced
and restarted directly with `--gui=qt`. The archive records the extracted-child
hash and verifies it against the freshly built host. The production link still
selects the SDK Qt6Qml library; no experimental collector is linked into it.

The preceding offscreen-vector report has an audit correction. Its operation
counts and display observations describe the archived binaries, but should not
be used as proof of a logging-free source deployment.

## Fresh-cache display comparison

Six fresh-launch runs use A1/B1/B2/A2/A3/B3 order, two ten-second reversing-wheel
windows per run: **60 seconds per version**. A is the prior immutable host with
stale generated logging; B is the regenerated source build. Both use the original
collector and a 5-ms budget. Detailed native probes, tests, builds, offline trace
analysis and UI Automation observers were absent during capture. The minimal
controller and PresentMon were common to both.

Each capture verifies Downloads (186 entries), density 152, 160 Hz, DPR 1.75,
active-window wheel input, reversing motion, no frame recorder, zero caption
measurements and zero model/snapshot changes. All use one Composed Flip swapchain.
Long intervals are displayed intervals above 9.375 ms.

| Run | Build | Long intervals | Maximum (ms) | Window p99 (ms) |
| --- | --- | ---: | ---: | --- |
| A1 | Old cache | 4 | 18.8229 | 6.3086 / 6.3090 |
| B1 | Fresh cache | 3 | 12.5014 | 6.2768 / 6.2821 |
| B2 | Fresh cache | 4 | 12.5080 | 6.2746 / 6.2761 |
| A2 | Old cache | 190 | 43.7590 | 18.7715 / 12.5119 |
| A3 | Old cache | 8 | 18.8038 | 6.3761 / 6.3817 |
| B3 | Fresh cache | 12 | 12.5410 | 6.3944 / 6.3863 |

A2 contains an unexplained sustained burst. No scheduling trace was recorded
for these clean runs, so its cause is unknown. Do not present the aggregate
202-versus-19 count as a reliable speedup or silently discard A2. The ordinary
runs overlap in outcome, and the fresh version still has missed display
intervals. Further attribution must use freshly generated QML.

## Collector experiment: useful latency result, failed correctness result

The experiment changes only an isolated copy of Qt's memory-manager object,
without a header/ABI change. It quarantines new regular/internal-class
allocations away from the chunks being swept, yields between chunks, defers
decommit until destructors complete, rebuilds free lists and preserves Qt's
redrain before resumed GC-state execution. Forced completion and engine shutdown
remain separate paths. This description does not establish correctness.

A standalone fixture creates 60,000 object graphs with 20,000 retained roots.
It checks mutations/new persistent roots between slices, retained WeakMap data,
QObject/value wrappers, free-list conservation/non-overlap, explicit full GC,
forced completion, twelve interrupted engine shutdowns and two interleaved
engines. The original collector passes its non-yielding control but fails the
new yield requirement. The prototype passes that requirement, with 393 slices
and a maximum observed slice of 0.1719 ms; Qt's incremental-mark cross-validation
also passes (maximum 0.2523 ms in that separate fixture run).

In live diagnostic scrolling with the prototype, DoSweep maxima at a 2-ms
budget are 2.0444 / 2.0308 ms for cold/warm captures. MarkDrain still reaches
2.7624 / 2.9095 ms because deadline checks are batched. At a 5-ms budget,
DoSweep maxima are 4.7965 / 4.8219 ms and MarkDrain reaches 6.1667 / 5.2794 ms.
These native probes perturb timing and are not clean cadence proof. Their QML
units still contain the stale logging discovered later.

The 87 selected QML/native checks pass on each graphics backend with the
prototype and incremental-mark cross-validation enabled. **Those checks missed
a real failure:** the sixth extended clean run, B3, crashed about eight seconds
after process start with `0xc0000005`. WER recorded process 73752 and a minidump.
The GUI-thread stack enters an invalid mark callback from MarkStack draining
inside `GCStateMachine::transition` / `MemoryManager::onEventLoop`. The invalid
heap/class-reference lifetime has not been resolved. Five completed runs cannot
be treated as a completed before/after comparison after that failure.

Twelve subsequent warm/debugger-attached scroll cases did not reproduce the
failure. They attached after startup and do not reproduce the failing cold
lifetime. The attempted live `qd` cleanup did not interrupt CDB; its wait timed
out, then only that exact debugger/debug-launcher tree was stopped. The verified
normal app was kept separate during those attempts. No new full crash dump was
produced; the original WER minidump and extracted stacks remain available locally.

The prototype must not be promoted merely because it bounds sweep duration.
Any continuation needs a failing lifetime regression and a correctness fix,
or should pursue other measured costs in the fresh source build. The default
GC budget and production Qt libraries remain unchanged.

## Evidence and next work

`profiler/experiments/qml-cache-and-sweep-experiment-2026-10-19/` contains helper
and prototype sources, regression results, native phase findings, clean timing
rows/summaries, failed-run logs, crash-stack excerpts, source/cache identities,
portable verification and final runtime identity, with a SHA-256 manifest.
Executables, native libraries, ETL files and crash dumps are excluded.

Next, attribute the remaining GUI/GC/render stalls on the corrected QML build.
The goal remains the actual 6.25-ms frame budget with no spikes, not passing a
small sweep fixture or merely removing diagnostic handlers. No commit or push
was made.
