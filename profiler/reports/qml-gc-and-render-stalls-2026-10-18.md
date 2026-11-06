# Correlating QML garbage collection and render stalls

## Current-state capture

The preceding continuation made verified source changes and measurements,
so it counts as progress. The current normal build includes icon coalescing,
idle vector geometry preservation and the integrated Qt stacking/retention
patches. Experimental native upload admission remains disabled.

The new icon-sampled-present-wheel-masonry capture records CPU samples,
scheduling and PresentMon together. Loader events and local host/Qt PDBs
provide symbols. The trace has no lost events/buffers. QPC timestamps correlate
actual Present calls to native frame completion within approximately 0.04–0.21
ms in the cited frames. Sampled stacks are nested observations, not exclusive
timing totals or proof of uninterrupted user CPU execution.

The correlation initially included the pause between scroll directions.
That was corrected to each direction's actual recorded frame interval;
pause/settling presents are excluded. The corrected result has eight displayed
intervals above 9.375 ms in this diagnostic capture. A 12.5178 ms display gap
has neighboring native periods with 9 and 6 GUI samples in GC transitions.
Another 12.5098 ms display gap has a 13.5457 ms native period with 9 GUI GC
samples and only 4.1133 ms render wall time. GC is active on the relevant GUI
critical path; it is not the only mechanism.

The worst displayed interval is 24.9721 ms. The corresponding Present spacing
is 22.4491 ms, native frame period 22.4536 ms and render wall time 14.1653 ms.
Render inclusive samples include prepareRenderPass 13, uploadBatch 10,
unmap 8 and D3D11 buffer creation 4. A separate frame spends 9.8833 ms in
rendering. These observations retain resource preparation/upload as a priority;
shortening GUI work cannot by itself eliminate those render stalls.

Local evidence: icon-sampled-present-wheel-masonry.{etl,json}, its
-present.csv, -scheduler-local.csv and -display-stalls.json under
artifacts/modes-profile. These are generated diagnostic files, not committed
source. Diagnostic cadence under sampling is not clean performance proof.

## GC time-slice experiment

Official Qt 6.11.1 qv4mm.cpp initializes its GC time limit to (1000 / 60) / 3,
which is 5 ms. The QV4_GC_TIMELIMIT override is read when the collector is
created. Separate phase captures confirm actual limits of 5000 and 1000 us
in the live engine, rather than relying only on launch environment intent.

The same host runs clean A/B/B/A with the override absent for A and 1 for B.
The workload is Downloads, 184 entries, density 152, extent 4161.142857,
actual DPR 1.75, active window, 160 Hz, delta 120 every 30 ms and about four
seconds each direction. State is restored between fresh processes. There is
no build, test, sampler, detailed observer or logging during clean captures.
Caption measurements are zero. PresentMon display-change intervals are the
headline; native periods are not substituted for them.

| Run | Down p99 ms | Down max ms | Down >9.375 ms | Up p99 ms | Up max ms | Up >9.375 ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| A1 default | 6.3704 | 18.8342 | 5 | 6.3075 | 12.4981 | 2 |
| B1 1 ms | 6.3262 | 18.7677 | 3 | 6.2769 | 12.4940 | 1 |
| B2 1 ms | 6.2946 | 18.7492 | 3 | 6.2854 | 12.4874 | 1 |
| A2 default | 6.3563 | 18.7464 | 3 | 6.3396 | 12.5030 | 3 |

Some counts improve, one downward pair ties, and maximum spikes remain.
This does not establish constant 160 Hz. No application default is changed.
Local capture stems are gc-slice-{a1,b1,b2,a2}-wheel-masonry; compact evidence
is artifacts/gc-slice-summary.json.

## Separating startup and scrolling GC phases

Qt's built-in qt.qml.gc.stepExecution category records individual state-body
durations. A file message handler adds QPC timestamps in diagnostic builds.
GUI-subsystem stderr did not provide those logs reliably, so the first empty
stderr run is not used as phase evidence. Dedicated file logs and engine budget
metadata confirm the corrected captures work.

Whole-process logs have mark operations up to 25.069 ms and sweep operations
up to 11.343 ms. Those maxima include startup and must not be described as
scrolling costs. After filtering timestamped events to actual direction
intervals:

| Budget | Phase | Calls | Maximum us | Total us |
| --- | --- | ---: | ---: | ---: |
| 5 ms | MarkDrain | 4 | 5542 | 15759 |
| 5 ms | DoSweep | 1 | 3624 | 3624 |
| 1 ms | MarkDrain | 16 | 2353 | 20142 |
| 1 ms | DoSweep | 2 | 3041 | 5242 |

These are separate diagnostic runs with different GC cycle counts, so totals
are not controlled throughput comparisons. Logging substantially perturbs
cadence; its frame percentiles are excluded. State-body timings exclude the
following log write, but still include scheduling/observer effects. The
important limit is structural: marking checks its deadline after batches of
objects, while DoSweep performs the whole sweep without such an interruption.
A 1 ms limit cannot guarantee a 1 ms GUI pause.

Evidence: gc-phases-timed-{five,one}-wheel-masonry.json and timestamped
gc-phases-timed-{five,one}.log; compact summaries are in artifacts.

## Floating QObject roots and ownership prototype

An observation-only override of the matching Qt 6.11.1 QObject wrapper mark
function counts its recursive child walks. Qt marks children recursively
when the wrapped QObject has no QObject parent. This differs from visual
parentage: a Repeater delegate can have a QQuickItem parent and no QObject
parent.

The actual retained gallery Loaders are such floating roots, as is the top-level
window. During one observed scroll GC, the window walk visits 5428 children;
four retained loader walks visit 3848, 2238, 8037 and 3530. Another capture's
largest retained loader has 10884 children. Thus inactive retained panels
participate in repeated whole-tree marking. The observation establishes the
walks, not that they can be removed safely without replacing their lifetime
guarantees.

The prototype assigns QObject parents to the window and the four retained
loaders without changing visual parents or geometry. Engine metadata confirms
five adoptions, the same 5000 us GC budget, and zero recorded floating-root
walks in its scroll interval versus five in the control. However, MarkWeakValues
work increases: total 3690 to 4909 us in this diagnostic pair. MarkDrain total
falls from 11606 to 9457 us, but its maximum rises from 5270 to 5809 us.
Removing the traversal does not remove all marking work or the slice limit.

The code and observer patch are archived under
profiler/experiments/qml-gc-ownership-2026-10-18. This remains experimental.

## Clean ownership measurements and decision

The same uninstrumented host is used for A and B. A leaves ownership as-is;
B performs the five adoptions once before settling. There is no observer
override, forced duplicate-symbol linkage, GC logging or sampler in this
executable. Metadata confirms 0/5/5/0 adoptions, actual 5000 us GC budgets,
and no GC/logging environment overrides. Workload and capture controls are
the same as the slice experiment above.

| Run | Down p99 ms | Down max ms | Down >9.375 ms | Up p99 ms | Up max ms | Up >9.375 ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| A1 | 6.3170 | 18.7256 | 4 | 6.3115 | 18.7437 | 3 |
| B1 | 6.4955 | 18.7599 | 6 | 6.2913 | 12.4921 | 1 |
| B2 | 6.3204 | 18.7626 | 4 | 6.2768 | 12.4931 | 1 |
| A2 | 6.3000 | 25.0001 | 3 | 6.3193 | 6.3362 | 0 |

The first downward pair regresses and the reverse pair does not improve the
long count. Upward results also do not reproduce consistently. There is no
reliable overall display gain. Source adoption is not promoted. The candidate
responds to a normal close request and exits, but the exit code is unavailable;
this does not prove full shutdown or all ownership semantics. Tab switching,
eviction and retained QML property behavior would need regressions before any
future production adoption. No new UI layout is shipped in this experiment.

Local stems are gc-owner-{a1,b1,b2,a2}-wheel-masonry; compact evidence is
artifacts/gc-owner-summary.json. The current application defaults and source
remain unchanged by these experiments. The 160 Hz objective is unfinished.
The next investigation should target the verified render upload/buffer resource
stalls while preserving complete SVG presentation and bounded image latency.

## Restored application

The observer patch applies cleanly to the matching official Qt 6.11.1 source.
Neither experiment changes production sources or the embedded payload. The
previously verified normal worktree executable is restored after profiling,
launched directly with --gui=qt and all GC, host and profiling overrides
cleared. WPR is not recording. The actual running extracted host matches the
normal build hash; its title is f4 [zoin_branch/cached-pixel-grid] and startup
stderr is empty. Canonical D:/Code/f4/f4.exe is untouched. Evidence is
artifacts/gc-investigation-normal-identity.json.

- Host SHA256: 7eedd3f9f8c75259a830cced02706baa4c162980132f14d309723639f9a44dc2
- Launcher SHA256: 3bbcc033ff2c1b6cb97a19a6da3eb20d7a018aa31dcd9cb1521783cc1764b9ca

No new commit or push is made. The earlier uncommitted icon-coalescing changes
remain intact. These measurements do not establish the full project test
suite or cross-platform release gates as passing.
