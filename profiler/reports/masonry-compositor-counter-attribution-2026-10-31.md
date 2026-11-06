# Masonry: compositor instruction/cycle counters and cache-domain migration

Constant 160 Hz remains unachieved. The preceding trace established missed
refreshes after F4's GPU work was already ready. This follow-up adds hardware
instruction/cycle counters and finds a different explanation than simply doing
more desktop drawing: the complete DWM CPU burst enclosing the miss retires
roughly the normal number of instructions but consumes many more cycles.
It follows a move between the processor's two L3 cache domains.

This is evidence for a cache-locality investigation, **not proof that migration
caused the miss**, a verified clock-speed measurement, or a production fix.
No application source, installed Qt archive, portable payload, Windows setting,
DWM scheduling policy, normal application instance, commit or push changed.
The experimental renderer remains unpromoted.

## Workload and measurement

The same typed-registry diagnostic host runs actual Downloads, 192 entries
including parent, Masonry density 152, actual 160 Hz and DPR 1.75. The normal
all-entry warm tour completes before two ten-second quarter-window-height
middle-scroll passes. Foreground is stable throughout measurement, inactive
checks are zero and the original full-workload gates remain unchanged.
No UI Automation, screenshots, builds or analysis run during timed gestures.

The capture retains the prior scheduler, CPU stacks, interrupts, GPU queue/DMA,
surface-token and PresentMon tracing. It adds strict `InstructionRetired` and
`TotalCycles` sampling on context switches. This is an intrusive attribution
capture, not a clean acceptance run. Preparation observer maximum is 68.2 µs.

The machine exposes both sources through `xperf -pmcsources`. Microsoft's
[PMU recording documentation](https://learn.microsoft.com/en-us/windows-hardware/test/wpt/recording-pmu-events)
documents the strict context-switch configuration. Counter data comes from
the documented [ETW extended counter item](https://learn.microsoft.com/en-us/windows/win32/api/evntcons/ns-evntcons-event_header_extended_data_item).
Source names/order are independently verified in xperf output; they are not
inferred from the numeric values.

## Counter validation and limits

The reader's self-test covers 64-bit values, exact two-counter payload size,
extended-item type and null rejection. All 303,531 preflight counter pairs,
thread IDs and timestamps match xperf one-to-one. Per-CPU streams are continuous
on all 32 logical CPUs and counters do not decrease. There are 27,141 initial
context switches before counters become available, with no missing values
inside the established preflight range. The original preflight assertion that
every startup context switch already had counters failed; the gap is retained
explicitly rather than filled with zeros.

The full trace contains 3,520,022 counter pairs and 7587 initial switches without
counters. No measured-window context switch lacks counters. All per-CPU
old/new-thread chains are continuous, and no counter decreases occur.
An independent 100 ms xperf export around the actual miss matches all 7608
counter records. Another check matches 277 application/DWM CPU sample prefixes.
Scheduler/graphics/stack/interrupt decoders report no lost events, buffers or
schema errors. Affected thread ownership is checked against process rundown.

At trace stop, xperf emits `PmcCorruptionStatus` records for all 32 CPUs, with
source IDs 25/19 and zero `LastGoodTimestampQpc` values. The same records appear
in the short preflight. Their zero-timestamp semantics have not been independently
established; the records are preserved as an explicit reliability limitation.
The results below describe the recorded, independently decoded counters. They
do not establish hardware-level counter integrity beyond these checks.

Counters are differenced **per CPU between consecutive context switches**.
The delta belongs to the outgoing thread's complete scheduled interval.
Counters are never prorated across Qt or DWM phase boundaries. The enclosing
DWM CPU burst includes wake/sleep bookkeeping outside its composition markers;
that extra scheduled time and any sharing with another cycle are recorded.
Counters also include interrupts and context-switch instrumentation. Reported
cycles divided by scheduled time is not an APERF/MPERF or core-frequency sensor.

## Display result and critical path

| Window | Display intervals | Maximum interval (ms) | Intervals >9.375 ms |
| --- | ---: | ---: | ---: |
| First middle pass | 1595 | 12.4957 | 1 |
| Second middle pass | 1596 | 6.4897 | 0 |

Present 907 is dropped between displayed-motion records 906 and 908. The
unchanged strict motion analysis fails at that continuity check, so no passing
motion result or full acceptance is claimed.

The full surface chain has 3690 API presents and 3690 consecutive tokens;
PresentMon contains 3689 rows. Startup rows are separate from measured native
phase recording. The affected token joins DWM composition cycle 3884. Driver
frame ordinals step by two and the driver period is 12.5041 ms: this particular
gap is a genuine missed refresh, not just timestamp notification jitter.

F4's affected frame is GPU-ready **12.0894 ms before** the missed refresh.
The DWM present scope consumes 3.6622 ms scheduled CPU. Matching DWM queue/DMA
work completes **0.1779 ms after** the intervening refresh notification. The
timeline verifier checks 101 queue/DMA joins and 95 conserved CPU costs, with
zero unknown scheduling and no affected TID reuse. Later expensive Qt frames
are not labeled the cause of an earlier missed refresh.

## Complete DWM CPU bursts

The ordinary-cycle summary initially permits at most 0.1 ms of counter-boundary
excess. The missed cycle has 0.1454 ms, so it is correctly **unavailable** in
that narrow per-cycle summary. The following comparison instead uses whole,
uninterpolated CPU bursts. The missed burst contains exactly one composition
cycle and is not shared with a neighboring cycle. This changes the attribution
unit; it does not relax any performance or motion gate.

The missed composition cycle takes 5.1460 ms, entirely scheduled. Its complete
enclosing CPU burst is 5.2914 ms on logical CPU 20:

- Recorded instructions: 8,021,359.
- Recorded cycles: 22,899,587.
- Recorded cycles per instruction: 2.8548.
- Reported cycles per scheduled nanosecond: 4.3277.

Immediately preceding complete bursts retire 7.80–7.93 million instructions,
consume 5.78–7.20 million cycles and have CPI 0.73–0.92. The similar instruction
count with higher cycles points away from a large instruction-work increase.
The reported cycles/time value does not decrease at the miss, which gives no
support here for the simple lower-clock hypothesis. Cache misses, memory
stalls, other interference and counter limitations remain distinguishable
possibilities.

## Processor topology and migration correlation

A read-only `GetLogicalProcessorInformationEx` probe reports 16 physical cores,
two logical processors per core and two 32 MiB L3 caches. Logical CPUs 0–15
share one L3; CPUs 16–31 share the other. No Intel P/E-core interpretation or
affinity change is involved.

The DWM composition thread runs the preceding cycle on CPU 14, then the missed
cycle on CPU 20. They are in different reported L3 domains.

For comparison, select complete single-cycle CPU bursts retiring 7.5–8.5 million
instructions. This band makes recorded instruction work comparable; every
cycle remains in the detailed output, including those outside the band.
Medians are independently computed by category:

| Entry relative to preceding cycle | Bursts | Scheduled burst (ms) | Instructions | Cycles | CPI | Reported cycles/ns |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Same logical CPU | 898 | 1.6337 | 7,928,404 | 6,300,017 | 0.7937 | 3.8763 |
| Different CPU, same L3 | 1232 | 1.7256 | 7,937,378 | 6,731,007 | 0.8449 | 3.9296 |
| Different L3 | 11 | 2.9704 | 8,002,151 | 12,843,702 | 1.6081 | 4.3031 |

This is observational, with only eleven cross-L3 cases in the band. It does not
control CPU load, SMT siblings, code/data cache residency or background work.
Cross-L3 migration correlates with worse instruction efficiency; it is not yet
proven causal. Do not turn this table into a global affinity policy or change
the system compositor on that evidence alone.

## Next experiment

Add available instruction/data cache-miss counters to the same bounded
diagnostic, then compare the same complete CPU bursts and migrations. Establish
counter validity and boundary accounting before using the result. This can
distinguish a cold-cache explanation from other cycles-per-instruction growth.

Application render-thread spikes remain a separate optimization target. If its
own instruction/cycle/cache evidence supports a locality problem, test an
application-only, reversible scheduler preference control before proposing a
product change. Do not infer that a change to F4's own thread can guarantee
Windows compositor delivery. Preserve the original normal-window cold wheel,
verified all-entry warm wheel and middle-scroll gates, native vector quality,
coverage, motion precision and static physical-pixel alignment.

## Preservation and cleanup

The work drive filled during offline export after timed input and trace capture
had finished. A byte-verified duplicate raw packet was retired, then diagnostic
result/build folders were copied with per-file hash checks and replaced by
junctions to their C: archives. No raw trace or unrelated user file was deleted.
The interrupted generated helper was regenerated successfully. Existing paths
remain usable. This setup/storage failure is not a scrolling result.

Sources, checks and compact results are in
[the experiment manifest](../experiments/masonry-compositor-counter-attribution-2026-10-31/manifest.json).
Large decoded data, ETLs, CSVs and binaries have external hash receipts.
Only the owned diagnostic app tree and trace loggers were stopped. Normal Go
PID 23488 and Qt PID 84780 remain alive unchanged.

> AI assistance has been used to create this output.
