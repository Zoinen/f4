# Masonry: correlated delivery capture without sampling or PMC

The remaining middle-scroll failures contain two different effects: a real
missed compositor refresh after F4 was already GPU-ready, and a delayed display
notification with one normal driver refresh interval. The original strict
performance checks remain failed. No metric was relaxed, and constant 160 Hz
without spikes has not been achieved.

## Capture and validity

This follow-up to the [word-hash experiment](masonry-uniform-word-hash-2026-10-31.md)
used the same separately named host with the snapshot observer disabled and the
tested word-hash candidate enabled. No renderer or UI code changed this turn.

The real Downloads folder still contains all 192 entries. The capture used
Masonry density 152, actual 3840×2160 / 160 Hz / DPR 1.75, a fresh isolated cache,
a verified all-entry warm tour, and two 30-second quarter-window-height
middle-scroll passes. Native foreground remained stable throughout. All prior
native vector, text, geometry, motion and visibility settings were retained.

The kernel trace recorded process/thread lifecycle, switches, ready events,
dispatcher activity, DPCs and interrupts. It recorded **no CPU sampling, stack
walks or performance counters**. The existing verified graphics controller
retained inclusive event filters for queue/DMA, driver refreshes, compositor,
DXGI and composition tokens. PresentMon and the existing in-host probes still
contribute overhead; this is attribution evidence, not an uninstrumented claim.

The first attempt was rejected before app launch because its derived wrapper
referenced an existing cache. The guard worked. Its short traces and rejection
receipt were preserved, and its owned sessions stopped cleanly. The valid B2
attempt uses a corrected fresh cache and new case identity.

Disk bounds were explicit: at least 6 GiB initially available on C:, kernel
trace limit 1,024 MiB and existing graphics-controller limit 4,096 MiB. Graphics
decoding required additional size-based free-space headroom. The resulting
traces were 462,159,872 bytes (kernel) and 675,020,800 bytes (graphics).

The trace readers report:

- Kernel: 11,333,113 events, no lost events/buffers, no schema errors.
- Graphics: 1,347,634 events, no lost events/buffers or property/schema errors.
- Both traces cover both complete measurement windows.
- Exact event-ID filtering passed. All observed DWM threads were decoded.
- 9,998 application presents match 9,998 composition tokens. 9,997 PresentMon
  rows are joined by unique API calls within native Qt submission brackets;
  the unmatched startup presentation does not enter a measurement window.
- 190 selected CPU intervals conserve scheduled, runnable, blocked and unknown
  time. Unknown time is zero. Nested interrupt unions are subtracted separately.
- 205 queue/DMA identities match by context, submit sequence and completion ID.
  Affected thread ownership is checked against lifecycle records, with no
  affected TID reuse.

These checks verify the relevant joins. They do not turn a driver event into an
optical scanout measurement, nor remove tracing overhead.

## The two failures

| Pass | Over-budget notification interval | Driver refresh step | Driver interval | Classification |
| --- | --- | --- | --- | --- |
| 0 | 12.4412 ms | 2 | 12.4875 ms | Actual driver refresh missed |
| 1 | 10.2042 ms | 1 | 6.2480 ms | Notification interval inflated |

The complete strict middle test fails at nonconsecutive displayed-motion
frames. Both passes remain rejected under the original ≤9.375 ms display,
≤2 px physical motion, ≤.002 px native velocity and zero-coverage-hold checks.
The driver classification supplements those checks rather than replacing them.

### Actual missed refresh

PresentMon presentation 2924 maps through its composition token to DWM cycle
5899 and a two-step driver refresh interval. F4's GPU completion preceded the
missed refresh by **12.0844 ms**. The associated DWM present work took:

| DWM interval component | Time |
| --- | --- |
| Wall | 3.7499 ms |
| Scheduled | 3.7453 ms |
| Runnable, unscheduled | .0046 ms |
| Blocked / unknown | 0 / 0 ms |
| Interrupt union within scheduled time | .0059 ms |
| Scheduled excluding interrupts | 3.7394 ms |

The matching DWM GPU DMA completed **.2616 ms after** the missed refresh and
before the eventual display notification. Thus this event is not explained by
late F4 submission or by DWM waiting unscheduled for several milliseconds.
The long interval is largely DWM's scheduled work, followed by late compositor
GPU completion. Function-level CPU attribution is unavailable in this capture
because sampling and stacks were deliberately absent.

The CPU-occupancy follow-up splits the DWM interval at its actual processor
migrations. Sibling logical processors are derived from the measured physical
core topology. Their occupancy covers every evaluated scheduled interval, with
no unknown owners. `vmmemWSL` PID 25016 occupied those sibling threads for
69.0545% of DWM's scheduled time; total sibling busy time was 69.6633%.
The process's recorded start precedes the capture, so the later name lookup is
not assigned to a newly reused PID. This is a contention correlation, **not a
causal proof**: no instruction/cycle counters or controlled VM intervention
were collected here. The VM was neither stopped nor reconfigured.

Nearby F4 frame 2926 is also slow: 10.216 ms elapsed, 4.103 ms scheduled,
.269 ms runnable and 5.844 ms blocked. Other nearby frames are much cheaper.
Temporal and token identities must be retained: a nearby later Qt frame cannot
be labelled the cause of an earlier compositor refresh miss.

### Inflated notification interval

Presentation 9408 has a 10.2042 ms notification interval, but the associated
driver counters advance once and their timestamps differ by 6.2480 ms. The
notification interval exceeds that driver period by 3.9562 ms. This confirms a
timing-path effect distinct from the first pass's missed driver refresh.

There is also a nearby Qt frame with 9.960 ms wall time but only 1.341 ms
scheduled work, .006 ms runnable and 8.612 ms blocked. Its successor is short.
The existing motion/presentation checks still reject this sequence. Driver
timestamps alone do not prove the exact pixels displayed or authorize silently
discarding the failure as harmless jitter.

## Consequences for the next work

The first event reproduces the earlier compositor-side miss with the expensive
uniform observer, CPU sampling, stacks and PMC all absent. Those observers are
not necessary for this failure to occur. It also explains why the faster hash
cannot by itself rescue an F4 frame that was already ready 12 ms in advance.

Further small hash tuning is not the next priority. The app's original cold
wheel and verified-warm wheel cases still need current full-scope validation;
those may expose remaining app-owned creation/upload costs worth fixing.
Keep compositor delivery and notification-driven motion failures distinct when
interpreting those results. Do not stop a VM, reduce the workload, change normal
window behavior or loosen the thresholds to manufacture a pass.

## Preservation and final state

The [experiment manifest](../experiments/masonry-lean-delivery-stall-2026-10-31/manifest.json)
contains compact sources, receipts, strict failures, verification and hashes
for the external raw/decoded traces. The raw Qt packet was copied to C: and
hash-checked before retiring its D: duplicate. No unrelated data was deleted.

All owned diagnostic apps and loggers are stopped; normal F4 Go PID 23488 and
Qt PID 84780 remain alive. Only read-only WSL/process inspection was performed;
no VM, external application, OS policy or normal F4 binary was changed. No
commit or push was made. Future captures remain authorized without readiness
questions.

> AI assistance has been used to create this output.
