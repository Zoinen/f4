# Video thumbnail conversion owns the three offscreen RHI devices

The new device/producer capture directly attributes three worker-thread QRhi
D3D11 devices to video-thumbnail frame conversion. All 382 recorded slow
offscreen RHI calls in that capture carry the conversion tag. This identifies
a concrete producer of concurrent graphics work; it does not prove that it owns
the previously observed driver/kernel locks or causes every render stall.

## Probe and checks

Diagnostic-only copies retain the current production behavior, original atlas
updates, four configured workers and full media support. The RHI probe records
device/context creation and destruction, adapter LUID, native thread, and owner
pointer for existing API calls and resource creations. It uses existing member
pointers, not additional COM queries or per-call allocations. A nested TLS scope
restores the previous owner correctly. Device records have a separate 1,024-slot
bound; calls/resources retain their 65,536-slot bounds.

The second copy additionally wraps only the existing call to
VideoFrameGeometry::displayImage(videoFrame) in VideoThumbnailRunner. A nested
TLS producer tag labels API calls and device creation during that conversion;
bounded spans record its start/end/thread. The conversion's arguments, result,
crop, orientation and execution count are unchanged. API wrapping and lifecycle
notes have explicit restoration assertions; the video-call replacement also
restores exactly. No production source or library archive is overwritten.

Validation before live input:

- Owner recorder: 12 passes; producer recorder: 13 passes, including QtTest
  setup/cleanup. They cover existing recording semantics, nested ownership,
  owner thread isolation, device/resource association and device bounds,
  producer restoration and return/execution preservation.
- Each disabled-recorder invocation: three passes including setup/cleanup.
- Each host: four import-isolation/compiled-host cases and system-DLL audit pass.
- Each RHI override: 44 native D3D11 checks pass at actual DPR 1.75 with backend
  assertion, no failures/skips.

Additional TLS stores, counters and span timing add diagnostic overhead; these
captures attribute work and are not optimization comparisons. The cache observer
maximum does not measure the full RHI-probe overhead.

## Captures and ownership

Both fresh processes use separate empty application caches, the actual Downloads
folder, Masonry at density 152, 160 Hz and DPR 1.75, with two ten-second rapid
wheel blocks. OS disk cache and Go metadata remain uncontrolled. No builds,
tests, UI Automation observation or analysis run during input. The first
diagnostic is closed before the second is launched; the normal portable app
remains alive throughout.

The owner-only capture is Qt PID 14020 / render thread 100716. It records one
render device and three distinct worker devices, all adapter LUID low=87088,
high=0. Worker Map calls at Qt RHI line 2139 are staging-texture readbacks in
finishActiveReadbacks. Distinct pointers prove separate RHI devices/contexts;
they do not establish independence of driver/global memory-manager locks.

The tagged capture is Qt PID 57620 / render thread 98536:

| Role / native thread | Device | Context | Creation producer |
| --- | --- | --- | --- |
| Window rendering / 98536 | 2e4251c0 | 2dd47be8 | none |
| Worker / 97576 | 58354620 | 59158618 | video-thumbnail-frame |
| Worker / 66368 | 4feb5470 | 6655c7f8 | video-thumbnail-frame |
| Worker / 96004 | 75e254c0 | c198c928 | video-thumbnail-frame |

All use the same adapter LUID. All 382 slow offscreen calls are tagged; the
recording threshold remains 50 microseconds, so this is not a count of every
offscreen API call. There are 73 conversion spans, maximum 93.6450 ms and
aggregate wall time 1,305.0527 ms. Concurrent spans overlap: that sum is not
elapsed time, CPU service or GPU execution time.

Two render-thread maps lasting 10.5611 and 9.3615 ms overlap conversion spans
on two worker threads. A 6.8978-ms upload and 5.3461-ms texture allocation also
overlap one conversion. A 9.7151-ms upload has no overlapping tagged conversion.
Thus temporal overlap supports a targeted experiment but cannot identify the
lock holder or explain all stalls. Hardware decoder activity outside the tagged
frame-to-image call is not fully instrumented.

Tagged capture presentation remains unacceptable:

| Block | Display gaps >9.375 ms | p99 interval | Maximum interval | Coverage holds |
| --- | ---: | ---: | ---: | ---: |
| 0 | 10 | 6.5111 ms | 25.0189 ms | 3 |
| 1 | 3 | 6.3574 ms | 12.5280 ms | 0 |

No scheduler trace accompanies these two captures. Their API wall times must
not be called blocked time by importing attribution from an earlier trace.
Constant 160 Hz is not achieved.

## Next discriminating control

The actual thumbnail helper calls QVideoFrame::toImage(), then applies existing
viewport/orientation handling. [Qt 6.11.1 conversion source](https://github.com/qt/qtmultimedia/blob/v6.11.1/src/multimedia/video/qvideoframeconverter.cpp)
shows an offscreen shader path that creates conversion resources and reads pixels
back. Its private qImageFromVideoFrame overload exposes forceCpu explicitly for
test purposes. The installed 6.11.1 private header declares that overload.

A diagnostic CPU-conversion control can remove this extra offscreen rendering
without disabling media or serializing all thumbnail jobs. Before live comparison,
verify pixel format/color-space/range, transparency, viewport, rotation/mirroring
and failure behavior. CPU mapping of hardware frames may still involve graphics
work; CPU conversion must not be assumed free or equivalent without tests.
Private test-only API use is not a production interface decision. No conversion
change has been implemented or promoted in this step.

Only exact diagnostic launcher trees were closed. Normal portable worktree
launcher 43936 / Qt 119460 remain running; no production source, dependency
archive or payload changed. No commit or push.

Evidence: `profiler/experiments/masonry-video-conversion-device-ownership-2026-10-27/`.

> AI assistance has been used to create this output.
