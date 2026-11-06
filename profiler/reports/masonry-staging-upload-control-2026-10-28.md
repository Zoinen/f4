# Staging copies are fast, but whole scrolling still spikes

Two bounded staging-upload prototypes pass pixel/lifetime checks and exercise
the real Downloads workload. Admitted transfers avoid long measured map/copy
calls, but neither live pair establishes a frame-time benefit. Wider admission
still leaves a 13.8348 ms geometry-buffer map. Neither candidate is promoted;
production remains unchanged and constant 160 Hz remains unachieved.

## Intervention and invariants

The diagnostic intercepts ordinary D3D texture updates. Eligible RGBA/BGRA
8-bit default 2D textures use a cached CPU-writable staging texture and a queued
subregion copy. It preserves source stride, destination offsets, mip/array
subresource, format and bytes. Unsupported uploads retain UpdateSubresource.
Original source transformations reverse exactly; other experimental renderer
and video changes are absent.

An event query protects each staging slot. It is reused only after a completed
non-flushing poll. Mapping uses WRITE with DO_NOT_WAIT; busy/exhausted/error
cases retain the original update rather than waiting. This follows the API's
[nonblocking Map behavior](https://learn.microsoft.com/en-us/windows/win32/api/d3d11/nf-d3d11-id3d11devicecontext-map)
and [asynchronous region-copy contract](https://learn.microsoft.com/en-us/windows/win32/api/d3d11/nf-d3d11-id3d11devicecontext-copysubresourceregion).
Explicit source boxes preserve the intended region, including copies between
differently sized source/destination textures. No live probe calls Flush or
waits for a slot.

The first prototype has eight slots per device, each at most 1024x1024. Its
eligible fallbacks show that this is too little admission capacity for bursts.
The second permits 64 slots per device while enforcing a shared 32 MiB texel
reservation budget with atomic admission. It retains the same extent limit.
That is a bound on texel storage, not measured driver overhead/peak process
memory. Query/device metadata is also bounded. Device destruction releases
staging textures and queries. This is experimental Windows code, not a final
cross-platform abstraction or release change.

## Correctness

Initial fixture: **55 passes**, zero failures/skips. Wider/budgeted fixture:
**56 passes**, zero failures/skips. Checks include:

- 48 exact pixel comparisons covering four RGBA/BGRA UNORM/sRGB formats,
  three region sizes (including 1x1), two mip levels and two array layers;
- destination offsets, padded source pitch, differing staging/destination
  extents and varying alpha bytes;
- whole uploads and unchanged fallback behavior for R8 and oversized uploads;
- overwriting source memory immediately after submission;
- 64 ordered region updates, with a deliberately eight-slot admission limit
  in the burst fixture so its original-path fallbacks are also exercised;
- reservation-budget rejection and releasing cached resources at shutdown.

The original burst fixture records 56 exhausted-slot fallbacks while retaining
identical pixels. This is not a mocked copy path. The final fixture checks the
shared reservation limit and zero remaining texel reservations after RHI
destruction. Busy-map and device-removal failures are not injected; no claim
of exhaustive fault coverage follows.

Each host variant passes all 44 native rendering checks, no skips/failures, at
expected actual DPR 1.75/requested D3D11. Four import-isolation/compiled-host
checks and the system-DLL audit pass. No static UI layout changes or portable
payload replacement occur. Raw-copy fixture readback may block to verify bytes;
that verification code is not part of the live upload path.

## Live captures

Actual Downloads, 190 entries, Masonry density 152, actual 160 Hz/DPR 1.75.
Each fresh process uses its own empty application cache and two ten-second
rapid-wheel blocks. All four captures pass unchanged focus, input-count,
record-capacity and target-PID presentation checks. OS file cache and Go
metadata are not cleared. No builds/tests, offline analysis or UI Automation
observer run during gestures. Default four workers, hardware decoding and all
media types remain. Both pairs use the same executable within that pair.

| Arm / Qt PID | Cold / next missed refreshes | Cold / next max display gap | Last stored entries |
| --- | --- | --- | ---: |
| Eight slots / 65308 | 8 / 3 | 18.7669 / 18.7816 ms | 203 |
| Eight-slot host, disabled / 119044 | 3 / 1 | 13.8613 / 12.4672 ms | 191 |
| 64 slots + budget / 73884 | 9 / 1 | 18.8085 / 12.5091 ms | 211 |
| 64-slot host, disabled / 124500 | 2 / 2 | 12.5101 / 12.4927 ms | 228 |

Miss threshold is >9.375 ms. All blocks have zero coverage holds, but large
individual motion errors remain. Stored entries are cache snapshots, not
unique files or per-file latency. These ordered pairs are not randomized
statistical regression proof; their work completion differs. They do not
establish a whole-scrolling benefit and cannot satisfy the goal.

## Admission and costs

| Enabled variant | Attempts | Unsupported | Staged | Eligible exhausted | Peak tracked texel bytes |
| --- | ---: | ---: | ---: | ---: | ---: |
| Eight slots | 7,885 | 4,706 | 1,875 | 1,304 | 7,995,392 |
| 64 slots + budget | 8,021 | 4,708 | 2,817 | 496 | 15,958,016 |

Among eligible attempts, staging rises from about 59% to 85%. Both variants
record zero map-busy/error results. Unsupported attempts include buffer and
other format/extent updates; they retain the original path, not dropped work.
Individual exhausted and unsupported reasons are aggregate counters: the
existing packet does not tag a reason on every fallback call.

In the eight-slot capture, no admitted MapWrite reaches the recorder's 50 us
threshold; the largest recorded queued-copy call is 61.3 us. The largest
fallback is a 9.0352 ms update to a 4096x4096 BGRA atlas. Main-path texture
creation still takes 9.6928 ms. Staging-texture creation reaches 1.9095 ms.

With wider admission, neither admitted MapWrite nor queued-copy calls reach
50 us in measured input windows. This is a thresholded observation, not a
zero-cost claim. Staging creation reaches 2.2823 ms; a GetData call reaches
159.2 us; the largest recorded fallback update is 3.4768 ms. The process creates
189 staging textures over its lifetime, including replacement/growth, and
performs 62,312 completion polls. Those costs are additional work.

The wider capture's largest render calls instead include geometry-buffer maps
of **13.8348, 8.8578 and 6.8353 ms**, and ordinary texture creation of 7.8510 ms.
Its disabled control still has texture uploads of 8.1578/7.4161 ms and a geometry
map of 5.9524 ms. API records describe wall time, not pure CPU execution or
blocked time. This capture has no scheduler trace to attribute lock ownership.

## Decision and next target

The staging path is correct in the tested scope and makes admitted transfer
calls short, but does not deliver smoother scrolling. Wider slots also add
allocation/polling work. Keep both variants experimental rather than promoting
a local improvement in API timing as a full-scene performance fix.

The next intervention should target geometry-buffer WRITE_DISCARD renaming and
its waits, preserving in-flight data and all rendering. The prior GPU-progress
capture already showed a constant-buffer stall after completed earlier context
work; reducing explicit allocations or assuming that all maps merely wait for
an earlier draw has not explained the end-to-end behavior. Any alternative map
or reuse policy needs a demonstrated completion/lifetime proof and native
regressions before a live performance claim. No blind NO_OVERWRITE substitution.

The snapshot retains eight-slot sources/receipts, both passing test sets, wider
sources, all four live packets and analysis, plus external hashes for large
binaries. Diagnostic processes were stopped. The normal portable build remains
unchanged and alive. No commit or push.

AI assistance has been used to create this output.
