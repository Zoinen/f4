# Masonry preparation budget attribution, 2026-11-01

## Result

The complete zero-overflow census moves the next optimization target away from
reducing the nearby-preparation timer. Visible `updateProperties` commits reach
13.0615 ms after input starts, whereas complete nearby callbacks peak at
1.9547 ms in the same period. The slowest visible commit contains four new/reused
row preparation spans totaling only 0.1129 ms. Acquiring those rows does not
explain that particular spike; the next counters should isolate identity/visual
binding, layout/finalization and RAII `endVisualUpdate()` completion.

No new product scheduling or layout change is made. Constant 160 Hz remains
unachieved, and the normal portable application is unchanged.

## Method

The diagnostic replaces only the delegate-commit and navigation/paging objects
in the existing row-replay candidate graph. It adds bounded, in-memory,
GUI-thread spans using the existing 131,072-entry census. JSON is written after
input. Original source files remain unchanged; instrumented copies and compile
commands are retained. No logging, extra rounding, geometry change or reduction
of preview coverage is introduced.

The first version records every row stage and overflows: `cold-x1` has 131,072
stored spans and **558,311 overflowed spans**, with its last stored span near
+4989.94 ms. It is rejected for complete attribution. The replacement records
every callback total, but detailed per-row timings only during nearby
preparation or when a row needs a delegate. Its `prepareNewOrNearbyRow` timer
starts after `ensureBrickAt`, so it does not claim to measure that lookup.
Detailed nearby layout/binding/finalization spans are not generalized to the
visible-row loop.

`cold-x2` records **61,682 spans and zero overflow** for the full observation.
It uses actual Downloads, 192 entries, Masonry density 152, the real 160 Hz,
DPR 1.75 desktop and 30 seconds of rapid wheel input. The gallery cache is empty
before QML; input starts with 25 frames cached and 11 pending requests. The OS
cache is not flushed. Before/after desktop-observer checks are clean.

## Inclusive timings

| Span | Calls after input start | Inclusive total | Median | Maximum after input start | Maximum including startup |
|---|---:|---:|---:|---:|---:|
| Complete nearby preparation | 1,527 | 272.7822 ms | 0.0993 ms | 1.9547 ms | 6.4789 ms |
| Inactive-delegate retirement | 4,306 | 120.7696 ms | 0.0277 ms | 0.3300 ms | 0.3300 ms |
| New/nearby row preparation, bounded subset | 3,919 | 160.2892 ms | 0.0164 ms | 1.4247 ms | 4.3733 ms |
| Complete visible property commit | 2,779 | 3,843.0026 ms | 1.0024 ms | 13.0615 ms | 42.9722 ms |
| Complete viewport-motion target call | 1,991 | 218.8765 ms | 0.0021 ms | 5.6137 ms | 5.6137 ms |

These are elapsed inclusive spans, not exclusive CPU durations. Methods nest;
totals across rows must not be added. A separate scheduler trace previously
established busy GUI execution in an early miss, but that trace is not this
capture and does not prove CPU versus waiting for every maximum in this table.

The slowest nearby callback starts before input, at −85.8569 ms, and takes
6.4789 ms. Its nested visual binding totals 6.0064 ms, with one binding reaching
5.7455 ms. This remains relevant to initial loading but is not relabeled as a
post-input stall. Only 12 detailed nearby rows are prepared after input starts;
the acquisition work for most rows occurs in visible commits instead.

## Slow visible commits

| Start relative to input | Complete commit | New/reused row spans | Their combined elapsed time |
|---:|---:|---:|---:|
| +1050.5695 ms | 13.0615 ms | 4 | 0.1129 ms |
| +7079.8184 ms | 11.1618 ms | 7 | 0.1678 ms |
| +10010.8910 ms | 10.3849 ms | 7 | 0.1085 ms |

None of these contains another recorded property commit. Their counters cover
acquisition of missing delegates, not every operation on already active rows.
The unexplained remainder therefore includes existing-row preparation and all
unmeasured binding/layout/transaction-completion work, plus possible waits.
It is not assigned wholesale to a specific QML callback without measurement.

The existing unchanged-row fast-path experiment from 2026-10-31 was inconclusive
and remains unpromoted. This finding does not justify silently reintroducing it
or reporting its high reuse rate as a measured display-performance win.

## Display and motion

`cold-x2` retains eight display intervals over 9.375 ms, with a maximum of
25.0330 ms, zero dropped presents and zero coverage holds. Its median/p95/p99
display intervals are 6.2499/6.2672/6.32963 ms. This is an attribution capture,
not a clean before/after comparison; added instrumentation and unequal startup
loading states preclude a claim of improvement or regression.

Motion analysis retains one unmatched commit, 39 frames without motion commits,
and all 4,785 excluded wheel pairs. It produces no physical wheel-error samples.
The small matrix-float error is not optical or displayed-motion verification.
The original cold/warm/middle-motion and full portable acceptance remains open.

## Build and storage failures retained

The first diagnostic SHA-256 is
`be8da8f4e439844b22114225d3aa518ee327ad8bd4a5ec6863cfe38bf078f19f`.
The accepted bounded-counter host is
`016c598edc2ee7c306f8e051f72e552f19da32427aca41e7a0383dd5543a986d`.
Windows GUI subsystem and system-DLL import audits pass. Neither is a rebuilt
embedded portable payload or a production rollout.

Two direct links to D: fail under disk pressure. The second leaves an incomplete
135,885,312-byte output that cannot be hashed because reads report ENOSPC.
After verifying the exact owned path is not running, only that failed output is
discarded, with its size/error and retained source/object/log evidence recorded.
A staged link on C: returns zero and produces the complete 68,306,432-byte host;
its receipt write initially fails for lack of D: space. That successful link is
recovered and verified rather than restarted. The published copy has the same
hash, passes audits, and the duplicate stage path is retained as a symlink.

Closed earlier hosts and a rejected trace are copied to the dedicated
`P:\f4-profiler-archive-f4-zoin-20261101` archive, checked before/after by SHA-256,
and their original paths preserved as symlinks. No raw capture is deleted.
No external application, VM, graphics setting, affinity or normal F4 process is
changed. No kernel tracing session is started during these counter captures.

AI assistance has been used to create this output.
