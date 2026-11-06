# Masonry: cache counters, sibling interference and longer delivery controls

Constant 160 Hz remains unachieved. Four-counter measurements weaken the simple
explanation that an increased cache-miss count or cross-L3 migration alone
causes the stalls. Element's background renderer is observed competing on the
same physical core during several worst stalls, but a reversible isolation
control does **not** establish it as the sole cause or a sufficient fix.

The most useful new acceptance finding is that all four short controls pass,
while longer runs reproduce misses both with and without Element isolation.
The next application-side measurement should remove an identified always-on
diagnostic observer before treating its CPU cost as product work.

No production source, installed Qt archive, embedded payload, normal F4 build,
commit or push changed. Element remains running with its original CPU affinity
restored. No persistent Windows scheduling setting changed.

## Four-counter attribution

The existing typed-registry diagnostic host runs actual Downloads, 192 entries
including parent, Masonry density 152, actual 160 Hz and DPR 1.75. The full
all-entry normal-content tour completes before two ten-second quarter-height
middle-scroll passes. Foreground remains stable. No screenshots, UI Automation,
builds or offline analysis run during gestures.

Strict context-switch counters are `InstructionRetired`, `TotalCycles`,
`IcacheMisses`, `DcacheMisses`, in that order. This follows the documented
[Windows PMU recording mechanism](https://learn.microsoft.com/en-us/windows-hardware/test/wpt/recording-pmu-events)
and [ETW extended counter item](https://learn.microsoft.com/en-us/windows/win32/api/evntcons/ns-evntcons-event_header_extended_data_item).
The cache-source labels are retained as reported; no unverified L1/L2/L3
interpretation or miss-latency measurement is attached to them.

The new four-counter reader rejects wrong payload widths, item types and null
pointers, and tests distinct 64-bit values in every position. All 134,727
preflight records match xperf one-to-one. All 32 per-CPU streams are monotonic
and have continuous old/new-thread identities. There are 5765 startup switches
before counters become available, with no gap inside the established stream.

The full trace contains 4,337,310 counter records and 8410 initial switches
without counters; no measured switch lacks counters. All 5866 counter rows in
an independent 100 ms xperf export around the first miss match. Another check
matches 283 application/DWM CPU samples. Decoder loss/schema checks pass.
Final status records retain source IDs 25/19/9/8 and zero LastGoodTimestampQpc
on all 32 CPUs. Their zero-timestamp semantics remain independently unverified,
as in the preceding report. These results describe the recorded counters;
hardware-level counter integrity is not asserted beyond these checks.

Counts are differenced per CPU across complete context-switch intervals.
No counter is interpolated across a Qt or DWM phase edge. Shared intervals,
upper/lower bounds and scheduled time outside the selected phase remain explicit.
Counter totals include interrupt and instrumentation activity.

## Reproduced display failures

| Pass | Display intervals | Maximum interval (ms) | Intervals >9.375 ms |
| --- | ---: | ---: | ---: |
| First ten seconds | 1597 | 6.5434 | 0 |
| Second ten seconds | 1592 | 12.4949 | 4 |

Presents 3449, 3482, 3514 and 3593 are dropped. Strict motion verification stops
at the first continuity failure; no passing motion result is fabricated.
The surface chain has 3660 API presents and consecutive tokens, with 3658
PresentMon rows. All four affected driver ordinal steps are two refreshes.
The verifier checks 411 queue/DMA joins, 383 conserved CPU costs, affected thread
ownership and zero unknown scheduling time.

| Present preceding dropped frame | DWM cycle | App GPU-ready before missed refresh (ms) | DWM present scheduled CPU (ms) | DWM DMA completion after missed refresh (ms) |
| --- | ---: | ---: | ---: | ---: |
| 3448 | 6510 | 12.0237 | 3.5638 | 0.2271 |
| 3481 | 6543 | 12.0364 | 3.7950 | 1.9681 |
| 3513 | 6575 | 12.1542 | 6.8176 | 1.8753 |
| 3592 | 6654 | 12.1839 | 5.8462 | 3.9177 |

These are completion-notification and driver-refresh relationships, not optical
scanout measurements. Later long Qt frames are not retroactively blamed for
earlier missed refreshes.

## Cache counts do not establish the cause

The first missed DWM cycle is enclosed by one unshared CPU interval. It retires
8,032,936 instructions and records 21,362,286 cycles: CPI 2.6593. Its cache
counts per thousand instructions are 1.8428 instruction-cache misses and
18.1462 data-cache misses. Ordinary-cycle medians are approximately 1.9983 and
16.8655 respectively, with CPI 0.9029. A modest change in these counts does not
by itself explain the much larger cycle cost. Conversely, similar miss counts
do not rule out a change in miss latency or an unmeasured cache level.

The other three missed cycles have counter intervals shared with neighboring
composition work. Their enclosing counts must not be presented as exact
per-cycle work. All four misses exceed the original 0.1 ms boundary-excess
criterion, so the narrow per-cycle counter summary correctly marks its missed
group unavailable. Whole-interval records are retained separately.

For complete single-cycle CPU bursts in the same 7.5–8.5 million instruction
band, medians are:

| Entry migration | Bursts | Scheduled burst (ms) | CPI | I-cache misses / 1000 instructions | D-cache misses / 1000 instructions |
| --- | ---: | ---: | ---: | ---: | ---: |
| Same logical CPU | 843 | 1.7261 | 0.8846 | 1.9892 | 17.0553 |
| Different CPU, same L3 | 1326 | 1.7860 | 0.9049 | 2.0001 | 16.8384 |
| Different L3 | 13 | 3.1471 | 1.6089 | 2.0844 | 17.4701 |

Cross-L3 entry still correlates with slower execution, but only the first two
misses follow a cross-L3 move. The last two do not. Migration is not established
as necessary or sufficient for the failures.

## Shared physical-core interference

A read-only topology probe maps the Ryzen 9 5950X's 32 logical CPUs to sixteen
two-thread cores and two 32 MiB L3 domains. A bounded offline scheduler decode
then accounts for both hardware threads on each affected core.

During the complete unshared CPU interval enclosing DWM cycle 6510, DWM runs
on logical CPU 27 while Element PID 35708 runs for 66.72% of the corresponding
interval on sibling CPU 26. The sibling is busy for 79.18% overall. At the
other misses, Element occupies 74.60%, 63.42% and 43.17% of the examined sibling
intervals. The middle two observations cover only the first of multiple
counter intervals and are marked accordingly; they are not whole-cycle totals.

The two worst F4 render spans show similar interference:

- Frame 3336: render scheduled time 3.9238 ms. Its full counter interval is
  4.1125 ms, with 0.1887 ms outside the render markers; recorded CPI is 5.1305.
  It retires 3,306,243 instructions, below the ordinary enclosing-interval
  median of 3,911,694. Element occupies 87.62% of sibling CPU 10 while F4 runs
  on CPU 11.
- Frame 3415: render scheduled time 5.8383 ms. The enclosing interval is
  8.3624 ms, including 2.5241 ms outside the render markers; its CPI 6.3663
  therefore describes that whole interval. Element occupies 67.26% of sibling
  CPU 0 while F4 runs on CPU 1.

The scheduler has two thread IDs without known owners elsewhere in the selected
data (32 and 55956). No evaluated sibling interval contains unknown ownership.
Every queried interval conserves exact scheduled coverage. This is observational
interference evidence, not proof that Element causes every stall.

A separate attempted analysis required an entire render span to fit in one
scheduled interval and failed its coverage assertion. It was replaced by
explicit full-interval upper/lower accounting for all 3201 render spans, rather
than silently selecting only the easy single-interval subset.

An independent xperf window reports Important BAM QoS for F4's GUI/render
threads and DWM. Observed switch-in priorities are 12/24 for GUI, 11 for render,
26 for DWM. These observations do not support an EcoQoS fix; they do not prove
the QoS state of every instant outside that reference window.

## Reversible Element control

Only the observed Element renderer process is controlled, pinned by PID,
creation time, executable path and `--type=renderer`. It has no application
children. B arms temporarily restrict it to logical CPUs 30/31, one physical
core; A arms retain all 32 CPUs. The renderer remains running. No Element UI,
message, authentication or network operation is performed.

An independent bounded watchdog restores the original mask after capture or on
timeout. Owned-fixture tests verify both signal-driven and timeout restoration.
An initial fixture guard rejects its console-host child; the correction permits
that specific Windows child only for owned test fixtures, not for the actual
Element target. All real controls restore all 32 CPUs, and all watchdogs exit.
The long B arm additionally verifies the test mask at capture end and records
it immediately before restoration. No persistent affinity preference is added.

All renderer flags remain identical between A and B. In particular, the source
template's former A/B registry toggle is removed for this experiment: the typed
registry is enabled in every arm. No extra kernel/PMC/graphics logger runs in
these controls. PresentMon and existing in-host probes still impose overhead.

### Short A/B/B/A, ten seconds per pass

| Arm | Element mask | Display max, pass 0/1 (ms) | Misses | Maximum physical motion error (px) |
| --- | --- | --- | --- | ---: |
| A1 | All 32 CPUs | 6.4267 / 6.4296 | 0 / 0 | 0.5156 |
| B1 | CPUs 30/31 | 6.4497 / 6.4726 | 0 / 0 | 0.6341 |
| B2 | CPUs 30/31 | 6.3845 / 6.4298 | 0 / 0 | 0.5120 |
| A2 | All 32 CPUs | 6.4146 / 6.4517 | 0 / 0 | 0.5744 |

All eight windows pass the unchanged strict middle-motion checks, have zero
coverage holds and native velocity error below 0.002 physical pixels. Both
unrestricted baselines also pass, so this series establishes no causal
isolation benefit. Element process CPU time across the entire capture,
including preparation, is 0.0625, 0.25, 21.0625 and 0.234375 seconds. Those
totals do not locate its work specifically inside the measured windows.

### Longer normal/control pair, thirty seconds per pass

| Case | Element mask | Display max, pass 0/1 (ms) | Misses |
| --- | --- | --- | --- |
| Unrestricted A1 | All 32 CPUs | 6.4519 / 12.4776 | 0 / 4 |
| Controlled B1 | CPUs 30/31 | 12.4865 / 6.4977 | 1 / 0 |

Both longer cases fail the unchanged continuity gate. Neither failure is
hidden by the short-run passes. The longer pair is not a second complete
A/B/B/A series, and one versus four misses is not accepted as proof of a rate
improvement. Controlled B1 consumes only 0.078125 seconds of Element CPU time
across the entire capture and still misses a refresh. Element isolation is
therefore not sufficient to achieve the goal. The uncontrolled long A1 does
not include the no-op affinity watchdog used in the short A arms; that setup
difference is retained rather than claiming perfectly matched instrumentation.

## Remaining F4-side work

The four-counter trace's preparation profile contains 1367 samples, only two
without joined stacks. Self samples include `updateMaterialDynamicData` (104),
`F4SharedMergedUniform::acquire` (81), graphics-pipeline-key creation (54), and
`F4UniformSnapshot::update` (45). The worst preparation is 2.8092 ms entirely
scheduled, including 1.6716 ms in uploads. These statistical samples identify
work to inspect, not exact function-duration measurements.

Crucially, `F4UniformSnapshot::update` is an **always-on diagnostic observer**.
Its current header performs material bookkeeping and several atomic increments
on every observed uniform update before forwarding the unchanged upload.
Its measured CPU cost must not be mistaken for product behavior. The shared
uniform table also uses a serial per-byte content hash, but changing that
policy should follow a cleaner baseline.

The next application experiment should bypass only uniform-snapshot observer
bookkeeping while preserving every QRhi resource update and renderer decision.
Verify matching native results/rendered output and portable imports, then
repeat representative delivery checks. If the content lookup remains a real
hotspot, optimize it with exact byte equality retained. Do not adopt an external
application-affinity policy as an F4 feature or claim that removing this observer
alone solves the compositor misses.

The original normal-window cold wheel, verified all-entry warm wheel, middle
scrolling, motion, coverage, native-vector quality and static pixel-grid
requirements remain intact. This turn does not revalidate cold/warm wheel or
establish full goal completion.

## Preservation

The experiment manifest includes sources, helper results, compact counters,
strict failures and passes, restored-affinity receipts and hashes for raw data:
[manifest](../experiments/masonry-cache-counters-and-isolation-2026-10-31/manifest.json).
Large traces and decoded streams remain external on C:. Older build directories
were relocated only after per-file copy/hash verification; original paths are
preserved by junctions. Raw duplicate packets on D: were retired only after
checking their preserved C: copies. No unrelated data was deleted.

All diagnostic applications and owned loggers are stopped. Element's original
affinity is verified, no affinity watchdog remains live, and normal F4 Go PID
23488 / Qt PID 84780 remain alive unchanged.

> AI assistance has been used to create this output.
