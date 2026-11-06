# Remaining Masonry GC pauses — 2026-10-18

## Result

Native phase timing and simultaneous CPU scheduling identify two remaining GUI
costs on the current promoted renderer: marking the retained QML object graph,
and sweeping ordinary JS heap blocks. Allocation during visual binding also
starts a collection synchronously. Reducing the nearby preparation loop's own
budget cannot bound that collection. No new production optimization is claimed
by this experiment; constant 160 Hz remains unfinished.

The preceding goal summary was a status-only response. This turn adds exact
phase evidence, a repeat with sweep subphases, and CPU attribution that changes
the next action from generic GC investigation to allocation and retained-graph
reduction. It does not repeat a GC chunk/budget trial.

## Measurement contract

Two fresh-process actual-wheel captures use Downloads, 184 entries, density 152,
Masonry, DPR 1.75, active 160 Hz, Direct3D 11, 30 ms wheel events and the normal
5000 us GC budget. Both use the current combined QtQuick upload-policy library.
Caption measurements are zero. These are instrumented attribution captures,
not clean before/after evidence of an optimization.

The matching Qt 6.11.1 memory-manager translation unit is rebuilt using the
existing original compile command and installed matching headers. Its single
object replaces the SDK object's archive member, without duplicate-symbol
overrides. The normal host, SDK, embedded payload and launcher are unchanged.

The observer records native steady-clock timestamps in a thread-local vector
reserved for 32768 records. Captures contain 2636 and 2823 records including
startup, so neither grows nor reaches the cap. Records serialize after the
measured interval. There are no per-phase messages or JSON construction during
scrolling. Clock calls and allocation trigger scopes still add overhead.
The query runs on the GUI thread. No QML profiler or UI Automation observer
runs during these captures.

Triggers distinguish allocString, allocData, event-loop continuation and other.
"Other" does not establish a specific allocation site: inline callers and
explicit GC paths are not all independently labelled. AllocString identifies
the synchronous caller, not which particular source string caused heap pressure.

## First capture

`gc-phases-wheel-masonry` records 451 GC events inside scrolling. Timed marking
steps reach 6.1837 ms; sweep steps reach 4.5992 ms. The longest nearby preparation
turn is 6.0096 ms. Its 125 GC state executions total 5.1151 ms and are entered
from allocString during visual binding. The work includes persistent and weak
value marking followed by mark draining; it is not one six-millisecond property
setter doing ordinary layout arithmetic.

Native frame periods: median 6.2301, p95 7.4970, p99 9.1607, max 15.6321 ms.
These include timer instrumentation and are not a clean cadence improvement.

## Repeat and sweep decomposition

`gc-sweep-parts-wheel-masonry` adds scopes around the seven existing sweep
operations, preserving their order and semantics. It records 617 GC events
inside scrolling. MarkDrain reaches 5.3748 ms; DoSweep reaches 4.7471 ms.
Another nearby turn takes 6.0221 ms, including 5.0368 ms across 124 GC states.

The three ordinary block sweeps take 2.3171, 0.6858 and 4.5431 ms. Identifier
table sweeps take 0.1824, 0.3497 and 0.1961 ms. Internal-class sweeping peaks at
0.0056 ms; huge-item sweeping and reset-bitmap work are much smaller. The
longest ordinary block sweep accounts for 4.5431 of 4.7471 ms in its enclosing
sweep. Thus it is a concrete target, rather than an assumed weak-wrapper cleanup
or bitmap reset bottleneck.

Simultaneous WPR CPU samples/context switches and PresentMon are captured in
this repeat. Raw QPC switch identities match 39781 exported switches with a
900 ns offset spread at 10 MHz. The trace reports zero events/buffers lost.
The 5.3748 and 5.3473 ms marking phases, 4.7471 ms enclosing sweep and 4.5431 ms
ordinary block sweep are entirely scheduled GUI execution. Their wall time
is not explained by waiting for another process.

Marking samples include QQuickItemWrapper and QObjectWrapper markObjects.
Four samples inside the longest ordinary block sweep include BlockAllocator and
Chunk sweeping; individual samples reach StringOrSymbol destruction and the
Windows heap free path. Four samples cannot establish a complete destruction
type distribution. Linker ICF aliases appear in other stack frames, so no
specific QHash insertion/detachment bottleneck is inferred from those names.

Native periods in the sampled repeat: median 6.2269, p95 7.6948, p99 9.5865,
max 13.0572 ms. Strict displayed intervals, with both endpoints inside each
direction window, reach 12.5124 ms downward and 12.4600 ms upward. GPU active
time peaks at 2.0805 and 2.7526 ms respectively. These are attribution data,
not an uninstrumented performance result or evidence of constant 160 Hz.

## Consequences and next work

The existing GC time limit is shared by the whole transition, and marking can
overshoot between deadline checks. Sweeping ordinary blocks is atomic in the
current Qt implementation. Changing a nominal budget alone does not bound all
these operations; earlier chunk/budget experiments already had mixed results.

The next optimization must reduce producers and retained work: measure the
allocation and object-lifetime contribution of visual-row reassignment and
delegate subtrees, then remove unnecessary bindings/objects with lifetime and
rendering regressions. Avoid unbounded retention to postpone collections, and
avoid changing QObject ownership solely to hide root traversal. Re-measure
clean display cadence after an actual production change. If necessary, changing
collector scheduling/sweep granularity is a separate Qt correctness project,
not a safe constant adjustment in this experiment.

## Artifacts and restoration

`profiler/experiments/gc-phase-attribution-2026-10-18/` contains both full native
JSON captures, compact summaries, source patch, observer/build/analysis scripts,
raw-QPC alignment, scheduling and sample summaries, and hashes of relevant
sources/libraries/binaries. Raw ETL/CSV, binaries and build output remain in
untracked artifacts. One attempt to relink the running diagnostic encountered
Windows' executable lock; it was corrected by using a distinct diagnostic
filename while leaving the running instance alive. No failed build was measured.

Panel state was restored through the live diagnostic before replacing only the
worktree's launcher tree. The normal portable app is restarted directly with
`--gui=qt` and diagnostic overrides cleared. Normal launcher SHA256 remains
`B510EBF344CBCD7FA1BDECB0A78C5C1BABC049E89BF2343E1D3448F44ACEB892`;
normal host SHA256 remains
`7A9C2027297AD6B84085F7911532F0BBC4D9EBD6F3B0321C1FEB2AC8F9BAE5FA`.
Live-process identity and startup-log verification are saved separately in
`artifacts/gc-phases-normal-identity.json`. WPR is stopped. Canonical F4 is
untouched. No commit or push is made.
