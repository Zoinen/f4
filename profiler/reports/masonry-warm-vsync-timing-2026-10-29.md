# Masonry: warm-wheel vsync timing attribution

## Finding and status

Constant 160 Hz without spikes remains **unachieved**. This continuation makes
progress by distinguishing a warm-wheel motion failure from the timing delay
in the event used to judge it. No animation clock, rendering policy, application
source, acceptance threshold or portable payload is changed. No commit/push.

At the worst step in this capture, PresentMon reports 6.5405 ms between display
changes, while consecutive driver-reported frame timestamps are 6.2595 ms apart.
The notification phase changes by .2810 ms. The recorded animation advances
6.25 ms. The original motion analyzer reports 4.8376 physical pixels of error;
substituting the associated driver frame timestamps for those same pairs gives
a block maximum of .5216 px. This is secondary timing attribution, **not** a
replacement acceptance gate or independent proof of displayed pixels.

The preceding goal turn made progress through tested preprocessing eligibility
reuse and cold/warm/middle captures. It left warm-wheel errors even with low
preparation times. This trace changes the next action: verify frame identity and
pixel movement against output refresh records before modifying animation timing.
Cold/loading CPU and driver waits remain separate unresolved work.

## Existing evidence was checked first

The Oct 23 desktop-marker experiment already decoded consecutive frame IDs
associated with output-specific WaitForVBlank wakeups, despite variable desktop
timestamps. It did not prove actual image displacement or physical scanout.
The Oct 21 DXGI frame-statistics experiment had stale/multi-monitor-ambiguous
refresh/QPC fields. Neither is repeated here as if it were a new discovery.

This capture adds kernel graphics events, their actual runtime TDH schemas,
driver-reported FrameNumber/FrameQPCTime fields and output/adapter identity to
the current 191-entry scene and tested diagnostic host.

## Capture

Case: `real-scroll-warm-vsync-events-20261029-wheel-masonry`, Qt PID 121732.
Actual Downloads, all 191 entries, Masonry density 152, actual 3840×2160 /160 Hz /
DPR 1.75, two ten-second rapid-wheel blocks. The normal tour visits the complete
folder before measurement. Pending bitmap work, new bitmap stores/misses and
decoded-frame count changes are all zero during measurement. This proves the
tour and bitmap quiescence, not every SVG source-generation/read event.

The already-tested `f4-qt-host-preprocess-eligibility-20261029.exe` is reused
unchanged, with eligibility reuse, bounded owner resolution and the earlier AV
diagnostic flags enabled. The capture's normal display/content/focus/input,
feature-use and resource/error guards pass. No build, tests, offline analysis,
desktop-copy observer or UI Automation polling runs during the gestures.

One owned user ETW session records Microsoft-Windows-DxgKrnl, keyword 0x1,
level 1, using QPC timestamps and no stacks. This session does not enable the
ForceVsync keyword. Ownership and exact ETL path are verified before stopping;
no unrelated logger is stopped. This is an intrusive attribution capture, not
a clean release performance comparison.

- ETL 109,051,904 bytes; gzip 13,972,867 bytes, losslessly verified.
- SHA256 `5d961c3b61ea22b96e5524ee69cfbfdf60663a3aa2b5e1855fb61ccfe8e6f779`.
- 930,757 events; zero lost events/buffers, metadata errors or ProcessTrace errors.
- Final decoder exports 113,815 selected rows; zero property errors.
- The initial generic decoder reports 4,022 property errors for zero-length
  parameter-count arrays. Runtime schemas identify their count fields as zero.
  Handling those empty arrays explicitly fixes decoding of the **same ETL**;
  no capture is repeated or selected for a favorable outcome. Initial metadata,
  source and raw data are retained.

The source reader's raw-byte, JSON escaping and zero-count tests pass. An
independent xperf dump matches all 7,829 VSyncDPC records exactly for adapter,
source/target IDs, FrameNumber and FrameQPCTime. Event QPC timestamps agree with
xperf's microsecond-format output within its one-microsecond quantization.
No published-schema fallback or guessed byte offset is used for these fields.

## Output identity

QueryDisplayConfig before/after capture agrees: one active path, MSI MAG322UPF,
GDI source DISPLAY1, adapter LUID 87088, source ID 0, target ID 200965,160/1 Hz.
The metadata reader obtains this through the documented
[QueryDisplayConfig API](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-querydisplayconfig).

The DpiReportAdapter event's AdapterLuid maps that LUID to the pDxgAdapter pointer
in VSyncDPC. Every selected DPC matches that adapter, target 200965 and source 0.
This capture has one active path; the older two-monitor report is not reused as
proof of the current topology.

All7,829 driver frame numbers are consecutive: 128284445 through128292273.
Interrupt/DPC counts match and ordering is checked. Driver FrameQPCTime is
monotonic. These are driver-reported values; no claim is made that they measure
photons or exact physical panel scanning.

## Timing results

| Block | Driver frame period p99 / max ms | DPC event period max ms | PresentMon display max ms | Original motion max px | Secondary driver-time motion max px |
| --- | --- | ---: | ---: | ---: | ---: |
| 0 | 6.2614 /6.3116 | 6.4080 | 6.4082 | 1.9161 | 1.0016 |
| 1 | 6.2601 /6.2774 | 6.5402 | 6.5405 | 4.8376 | .5216 |

Both blocks have zero PresentMon display intervals above 9.375ms and zero
coverage holds. The **original** two-pixel motion gate passes block 0 and fails
block 1. That failure remains recorded. Neither earlier clean warm-wheel
failure nor the earlier 13.3629px example is retroactively reclassified: those
captures lack this simultaneous graphics trace.

Across the complete trace, driver-frame timestamp to DPC event delay has median
.0213ms, p99 .0869ms and maximum .3723ms. Interrupt-event to DPC delay has median
.0206ms and maximum .3660ms. In the measured blocks, PresentMon's reconstructed
display times track associated DPC event timestamps within 6.4 microseconds;
median discrepancy is about .3 microseconds. All app presentation rows use
Composed Flip, sync interval 1, flags 0.

The same version's
[PresentMon consumer](https://github.com/GameTechDev/PresentMon/blob/v2.6.0/PresentData/PresentMonTraceConsumer.cpp#L1104)
uses the sync-DPC event-header timestamp in its screen-time path. The trace
measurements independently show the close correspondence here; that source
reference alone is not used to infer what was physically scanned out.

Worst block 1 pair:

| Quantity | Previous | Current /difference |
| --- | ---: | ---: |
| Driver FrameNumber |128290715 |128290716 |
| Driver FrameQPCTime ticks |10263953455398 |10263953517993 |
| DPC event QPC ticks |10263953455609 |10263953521011 |
| PresentMon reconstructed display QPC ticks |10263953455612 |10263953521017 |
| PresentMon phase after driver time |.0214ms |.3024ms |
| Driver-time interval | |6.2595ms |
| PresentMon-time interval | |6.5405ms |
| Animation step | |6.25ms |

The .2810ms phase change explains most of this pair's discrepancy relative to
the animation clock. It is not evidence that the application should change its
clock to follow DPC delivery latency.

## Secondary calculation and limits

The secondary analyzer changes only each pair's time basis to the preceding
driver-reported FrameQPCTime. It preserves the original camera positions,
curve assertions, retarget/geometry exclusions and window selection. Pair counts
remain exactly 1,027 and 1,023; coverage and PresentMon statistics are unchanged.
Every associated presentation advances the preceding driver frame number by
one in both measured blocks.

This is a **temporal association**, with raw phase offsets retained. It does not
independently establish which application image appeared in that refresh, actual
pixel displacement or physical scanout. Consequently the secondary values are
not substituted into goal acceptance, no threshold is relaxed and no original
failure is hidden. The result explains a timing ambiguity in this run rather
than proving that all earlier motion failures or cold display gaps were false.

## Next discriminating work

Use a fresh current-host marker/pixel capture associated with driver refresh
ordinals and rendered camera positions, retaining endpoint pauses and observer
overhead. The previous marker source and output-vblank observer are available,
but their old capture cannot prove the current scene's image displacement.
Only after frame/image identity is verified should timing validation or the
animation clock change. Continue treating cold renderer work and initial
video-conversion-context contention as separate real bottlenecks.

Sources, provider metadata, decoder revisions/tests, exact xperf verification,
raw capture/build identity, timing analyses and receipts are frozen under
`profiler/experiments/masonry-warm-vsync-timing-2026-10-29`. ETL, gzip, decoded
event exports and xperf dump remain on C: and are externally hashed. The normal
worktree application remains alive unchanged and all owned diagnostic instances
are stopped. The goal stays active.

AI assistance has been used to create this output.
