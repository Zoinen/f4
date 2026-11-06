# Parent mapping: repeat, cold/middle checks and delivery, 2026-10-31

## Decision

The per-pass parent-mapping prototype has a repeatable median synchronization
benefit, but it does not solve the requested constant-160-Hz behavior. Keep
it experimental. The new cold, warm and quarter-height middle cases all fail
the original full acceptance. The next investigation should target remaining
presentation/delivery failures rather than assume another small preparation
reduction will fix them.

The preceding goal turn is progress: it implemented the bounded prototype,
passed geometry/native tests and measured the first A/B pair. This follow-up
repeats the pair in reverse order, exercises the outstanding workload scopes,
joins all new over-budget intervals to Qt submissions and recovers space from
completed profiling artifacts. No product source/default or normal app changes.

## Repeated synchronization cost

Same executable and source hashes, real Downloads/192 entries, Masonry density
152, 3840x2160, actual 160 Hz / DPR 1.75, ordinary window and four workers.
Each warm process first displays all indices 0..191 with normal visibility
preserved, then runs two 30-second wheel windows. Duplicate geometry/registry
oracles are off for timing. The successful sequence is A1/B1/B2R2/A2, with a
rejected B2 startup and storage recovery between the two pairs.

| Run/window | Parent sharing | Membership median (ms) | Total postSync median (ms) | Membership p99 (ms) |
|---|---|---:|---:|---:|
| A1/0 | Off | .1671 | .19235 | .3319 |
| A1/1 | Off | .1718 | .19730 | .4219 |
| B1/0 | On | .1203 | .14720 | .3928 |
| B1/1 | On | .1211 | .14830 | .3982 |
| B2R2/0 | On | .1127 | .13720 | .2671 |
| B2R2/1 | On | .1065 | .12970 | .4081 |
| A2/0 | Off | .1710 | .19615 | .4182 |
| A2/1 | Off | .1783 | .20430 | .4709 |

The reverse-order pair shows approximately 30.1% / 36.5% lower median total
postSync with sharing. Retained delegate counts and input counts vary slightly;
the data supports a recurring component-cost benefit, not an exact causal
percentage or an end-to-end improvement. The first pair ran before relocation,
the second after it; both members of the second pair use the same uncompressed
executable on D:. It is not an uninterrupted identical-environment ABBA run.

The earlier six helper, 46 native, seven motion and three import cases, 30
byte-identical PNGs and 698,364 live oracle comparisons remain inherited
verification. They are not claimed as newly executed tests in this follow-up.
Compiled inputs still match their receipt; pixel-fitting/layout logic is not
changed. The 32-slot cache remains local to one GUI-locked synchronization.

## Full workload results

All successful windows preserve foreground, zero observer overflows, zero
scene-registry/mapping mismatches and zero coverage holds. No scene family is
removed. Warm and middle windows have no new thumbnail misses; cold pass 0
has 133 new admissions and reverse pass 1 has zero.

| Scope/window | Preparation p99 (ms) | Maximum display interval (ms) | Intervals >9.375 ms | Maximum motion error (physical px) |
|---|---:|---:|---:|---:|
| Warm candidate B2R2/0 | 1.8188 | 7.0137 | 0 | 12.43734 |
| Warm candidate B2R2/1 | 1.7770 | 12.4793 | 3 | 95.50738 |
| Warm baseline A2/0 | 1.7690 | 12.5622 | 3 | 116.08661 |
| Warm baseline A2/1 | 1.8244 | 12.3731 | 1 | 111.49466 |
| Cold candidate B1/0 | 2.0198 | 12.5280 | 8 | 112.54620 |
| Cold candidate B1/1 | 1.7762 | 6.4376 | 0 | 3.09397 |
| Middle candidate B2/0 | 1.7224 | 12.5041 | 2 | Strict continuity fails |
| Middle candidate B2/1 | 1.6860 | 12.5090 | 1 | Strict continuity fails |

Cold loading starts with a fresh isolated cache at the ordinary initial
viewport, without the warm tour. Middle scrolling uses exactly one quarter of
the recorded window height and the complete 192-entry warm tour. Its unchanged
strict analyzer fails at a missing presented-motion frame (2043 -> 2045); no
pair is discarded to manufacture a pass. Supplemental native-clock analysis
has 4,787 / 4,788 comparable pairs, 11 reversal boundaries per window, zero
geometry exclusions and maximum velocity error .0009806821 px in both windows,
below .002 px. This does not replace the failed displayed-continuity gate.

All original requirements remain: displayed intervals <=9.375 ms, maximum
motion error <=2 px, native middle error <=.002 px and zero coverage holds.
Every successful scope still fails full acceptance. Outliers remain included.

## Delivery attribution

All **18** over-budget intervals in the new successful runs join to exact Qt
submission brackets. Their corresponding F4 frames are reported GPU-ready
**11.8671-12.8303 ms before** the nominal next refresh. Associated preparation
costs are .3268-2.3520 ms. This includes all eight cold first-pass intervals
and all three middle intervals; it is not a warm-only conclusion.

The deadline is the preceding reported display timestamp plus 6.25 ms, not a
direct DWM latch measurement. PresentMon timing is not optical scanout, and
these packets cannot distinguish every true driver refresh miss from delayed
display notifications. Earlier driver/scheduler evidence established that
both occur. This attribution retains the failed gates; it does not relabel
the intervals as harmless or prove that F4 has no further work to optimize.
It does establish that these selected reported misses do not line up with
late readiness of the corresponding F4 frame.

## Rejected attempts and space recovery

Warm B2 produces no packet and leaves startup state `settling`. Process
inspection confirms its Qt/Go processes are terminal. Windows event 1000
matches activation PID 0xCAB4 and reports `0xc0000006` in `ntdll.dll`; C: is
observed full and compressed-file reads fail with insufficient space. This
attempt is rejected as a performance capture. The event does not by itself
prove its complete fault cause, nor fix or explain the earlier separate
D3D12 qAbort. Fresh B2R2 is explicitly named and retains the rejected evidence.

Middle B1 is rejected before launch by the new C: headroom check. B1R2 is
rejected before app launch because the generated recipe refers to a missing
middle launcher; that helper error is corrected before the fresh B2 run.
The middle postprocessor also initially lacks a raw-location receipt and has
an old output-directory name. Those ordering/path errors are corrected and
analysis rerun on the same packet; they do not trigger another live capture
or alter the strict failure.

Recovery changes only completed artifacts belonging to this task:

- Two completed maps and a map-only executable are copied with hash checks,
  retaining original paths through file aliases.
- The completed fatal-probe and parent-mapping directories are preserved on
  D: with all 26 / 84 files matching immutable frozen hashes. Full C: prevents
  re-reading some compressed originals, so the already-completed copies are
  independently checked against every frozen hash before retiring duplicates.
  The original directory aliases remain. The candidate is uncompressed on D:.
- Three completed traces and 100 closed workspace diagnostic hosts are
  transparently compressed with before/after content hashes. No raw trace is
  deleted. Compression receipts record actual allocation savings.
- Another completed 84-file vector build is copied and verified with its
  original aliases retained. Only verified duplicate copies are retired.

Both drives end with substantial headroom (approximately C: 4.1 GB and
D: 1.9 GB at final audit). These are observations, not attribution of every
free-space change to one relocation. New captures require >200 MiB on C: and
>750 MiB on D:. OS storage/memory settings, VMs and unrelated applications
are not changed. No capture is run concurrently with compression, builds,
offline analysis, screenshots or accessibility inspection.

## Final state and next work

All owned capture Qt/Go processes, matching foreground observers and PresentMon
sessions are terminal. Normal worktree Go PID 23488 and Qt PID 84780 remain
alive at their original paths. The portable payload is not rebuilt; no
production optimization, commit or push is made. The full goal remains active.

The evidence index alongside this report preserves source/build hashes,
successful and rejected attempts, original strict failures, component timing,
native-clock checks, exact delivery joins, recovery receipts and raw locations.
Next work should investigate presentation scheduling and compositor delivery
on the normal window, preserving these workload and quality requirements.
Repeating the same geometry micro-optimization alone is not supported as the
next route to removing the remaining spikes.

AI assistance has been used to create this output.
