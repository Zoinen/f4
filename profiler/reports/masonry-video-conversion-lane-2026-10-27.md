# One video-conversion device does not remove the scrolling stalls

The bounded lane works and preserves tested pixels, but the first live pair
does not show a scrolling benefit. It remains a diagnostic experiment. No
production source or embedded payload was changed, and constant 160 Hz remains
unachieved.

## Intervention and correctness

The diagnostic host can dispatch only video frame-to-image conversion to one
dedicated Qt thread. Four general decode workers, hardware video decoding,
all media types, atlas behavior, and original Qt GPU/color conversion remain.
At most four conversion jobs may be outstanding. Callers check cancellation
before queued conversion; shutdown drains admitted jobs before quitting the
thread. The old and candidate paths share one executable and a profiling flag.
This is an experimental implementation, not a proposed final threading API.

The fixture reports **300 passes, zero failures/skips**. Its matrix checks exact
pixel identity for NV12, planar YUV and alpha BGRA; BT.601/709/2020 and full/video
range; varying chroma, crop, frame rotation and mirroring. Seven additional
metadata cases preserve the original behavior. Candidate frames have independent
backing buffers: sharing a reference frame would allow Qt's cached QImage to
hide the conversion. An RHI readback on the lane's actual thread proves the
candidate executes the GPU path. Invalid frames, queued cancellation, releasing
waiters during close, and rejecting work after close are also checked.

These synthetic frames do not prove every hardware-frame mapping or platform.
The live capture separately exercises actual Downloads videos. Import-isolation
and compiled-host tests report four passes; the Windows system-DLL audit passes.
There was no static UI layout change, so no new pixel-layout claim is made.

## Live evidence

Actual `C:\Users\xs\Downloads`, Masonry density 152, actual DPR 1.75 and 160 Hz.
Each fresh process uses a separate initially empty application cache and two
ten-second rapid-wheel blocks. Both pass the unchanged focus, input-count,
record-capacity and target-PID presentation checks. OS file cache and Go metadata
were not flushed. No build, test, offline analysis or UI Automation polling ran
during the gestures. The normal portable app stayed alive.

| Arm / Qt PID | Block | Display gaps >9.375 ms | Maximum gap | Coverage holds |
| --- | ---: | ---: | ---: | ---: |
| Lane / 111208 | 0 | 13 | 25.0026 ms | 0 |
| Lane / 111208 | 1 | 2 | 12.4959 ms | 0 |
| Default / 65064 | 0 | 8 | 13.6634 ms | 0 |
| Default / 65064 | 1 | 2 | 18.7576 ms | 0 |

The lane created exactly one offscreen conversion device, versus three in the
control. All 71 lane producer spans ran on its thread; maximum observed
outstanding jobs was two, maximum active one, and all 71 completed. At the last
preparation sample, 198 thumbnails were stored versus 202 in the control.
Producer maximum wall time was 53.8438 ms versus 67.7054 ms. These are not
per-item latency measurements or CPU/GPU execution times.

The candidate still recorded render-thread UpdateSubresource calls of 8.4436
and 8.3618 ms, CreateBuffer of 8.2616 ms, and CreateTexture2D of 7.9022 ms.
The control's largest upload was 8.1377 ms. This probe records API wall duration;
no scheduler trace was collected for this pair, so blocked time and lock owner
cannot be attributed from these calls alone.

This single ordered pair is not a randomized repeated comparison and does not
establish a statistical regression. It establishes that reducing three video
conversion devices to one is insufficient to remove the observed stalls. The
candidate is not promoted. Presentation/motion checks still fail the requested
end state; neither p99 nor an average near 160 Hz overrides the missed refreshes
or large individual motion errors.

## Next investigation

The earlier exact scheduler stacks identify graphics allocation and upload
wait paths. This intervention removes extra conversion devices but preserves
Qt's repeated per-conversion GPU resource creation. The next useful target is
resource reuse or allocation scheduling, with pixel identity and complete media
throughput preserved. A benefit must be established on actual cold scrolling,
then verified for warm rapid-wheel and middle-button motion without relaxing
the display, coverage or physical-motion gates.

The experiment snapshot contains source, build receipts, tests, capture packets,
analysis and hashes for large external binaries. The normal portable build is
unchanged. Diagnostic processes were stopped after the pair. No commit or push.

AI assistance has been used to create this output.
