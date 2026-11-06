# Masonry: creating the video-conversion context before the window

Constant 160 Hz without spikes remains **unachieved**. Startup prewarming of
the bounded video-conversion lane preserves tested pixels but does not remove
the cold-scroll stalls. This candidate is not promoted to the production Qt
payload. No commit or push was made.

## Intervention

Earlier scheduling evidence linked one presentation wait to concurrent creation
of Qt's thread-local video-conversion QRhi. The earlier lane experiment reduced
the number of conversion contexts but still created its first context during
interactive work. This experiment separates that first creation from rendering:
after constructing QGuiApplication and before creating/loading the interactive
window, synchronously ask the dedicated conversion thread to call Qt's existing
`qEnsureThreadLocalRhi`. That context remains owned by the same thread and is
used by later original Qt video-to-image conversion calls.

Both arms use the same executable and the original bounded conversion lane:
one active conversion, four outstanding permits, existing cancellation and
shutdown behavior, hardware decoding, original GPU conversion and image quality.
The only flag varied is `F4_PROFILE_VIDEO_CONTEXT_PREWARM`. The producer tag
contains `video`, preserving the existing exclusion of conversion contexts from
the render-thread asynchronous buffer pool. A diagnostic identity check records
any unexpected replacement of the prewarmed RHI during conversion.

The current preprocessing-eligibility, vector-owner-resolution, AV private-heap,
and thumbnail decoder-device-reuse diagnostics remain enabled in both arms.
The worker source is the AV-device-reuse variant from the actual baseline link
response, preserving its decoder-device tag. The initial build harness selected
an older worker object and stopped at a link-response assertion; that harness
was corrected before any live run.

This change does not cache conversion resources, precompile all media shaders,
remove GPU readbacks, or eliminate later driver allocations. It tests the
context-creation hypothesis specifically.

## Correctness and build checks

- 301 native pixel/lifetime tests passed, zero failures/skips. The matrix retains
  exact original output for NV12/YUV420P/BGRA, color spaces/ranges, crop,
  rotation, mirroring, alpha and metadata; queued cancellation and close release
  waiters. The new check confirms a D3D11 context exists on the lane before
  interactive-window construction and is reused by queued operations.
- With prewarming disabled, the startup-context regression fails with
  `Conversion RHI must exist before the interactive window`. The initially
  invalid attempt to select QtTest's initTestCase as a standalone function is
  preserved separately; it is not the regression result.
- 14 real-media tests passed. Sixty frames from all six Downloads videos match
  the original GPU reference exactly: 30 Qt-delivered frames and 30 explicit
  D3D11 hardware imports. Imported frames outlive decoder destruction;
  conversion does not grow their held hardware-frame reference counts.
- Embedded-QML import isolation and Windows system-DLL audit passed. Existing
  renderer objects/resources remain the current baseline's objects. No static
  UI layout, Qt installation, normal executable or embedded payload was changed.

The tests first exposed a stale media-log hash in the capture manifest; the
capture stopped before launch. The final manifest includes the completed media
verification outputs, and every later live launch passed its hash preflight.

## Live captures

Real Downloads, 191 entries, Masonry density 152, 3840×2160, 160 Hz, actual DPR
1.75. Fresh initially empty application cache for each process; OS file cache
is not flushed. Two ten-second rapid-wheel blocks per process. No build, test,
UI Automation observer or offline analysis runs during measured gestures.

The intended order was control A1, candidate B1, candidate B2, control A2.
A1 delivered only 1,241 wheel events, below the unchanged 1,500-event guard;
all recorded events were active, but it is rejected and retained. A further
control A3 was recorded. Therefore the four accepted captures are ordered
**B1, B2, A2, A3**, not a complete accepted ABBA experiment.

| Case | Block | Preparation max, ms | Display max, ms | Gaps >9.375 ms | Max motion error, physical px |
| --- | ---: | ---: | ---: | ---: | ---: |
| B1 prewarm | 0 | 12.6316 | 18.8352 | 25 | 119.969 |
| B1 prewarm | 1 | 7.3093 | 12.5177 | 5 | 98.815 |
| B2 prewarm | 0 | 9.9857 | 12.5705 | 22 | 106.490 |
| B2 prewarm | 1 | 14.6824 | 30.9214 | 26 | 230.496 |
| A2 lane only | 0 | 8.9532 | 18.7339 | 22 | 185.194 |
| A2 lane only | 1 | 9.3998 | 18.7196 | 17 | 124.902 |
| A3 lane only | 0 | 8.1507 | 18.7770 | 19 | 102.326 |
| A3 lane only | 1 | 7.3736 | 18.5952 | 14 | 209.502 |

Candidate/control totals: **78/72 missed-interval observations**. The sample is
small and order-confounded; it establishes neither a population-level benefit
nor a statistical regression. It plainly does not establish zero spikes.
Every block fails the original maximum-motion requirement. Motion values use
the original PresentMon-based analysis; earlier notification-time jitter is
not fitted away or used to reclassify these failures.

Accepted wheel-event counts: B1 1,559, B2 1,520, A2 1,532, A3 1,541. The
candidate contexts were ready in 207.5585/197.8606 ms at startup, before all
recorded native motion samples. Their RHI identity mismatch counters are zero.
Each candidate uses one active conversion and at most two outstanding jobs;
both finish with no outstanding jobs. Both arms have exactly one asynchronous
geometry pool owned by their render thread, with the original 100-entry bound.
No second pool was accidentally created by prewarming. Detailed RHI-call
recording remains disabled for clean timings, so its empty device array must
not be treated as a device-creation count.

## Consequence

Moving first conversion-context creation before the window is insufficient.
The earlier scheduling attribution remains valid for its particular traced
frame, but it does not explain away all cold-scroll stalls. Do not ship this
startup delay as an established scrolling fix.

The next investigation should target the remaining long preparation/recording
and allocation intervals on the current scene, distinguishing scheduled CPU
work from driver waits. The unchanged cold, verified-warm, and quarter-height
middle acceptance matrix remains required for the full goal.

Sources, tests, rejected and accepted capture packets, analyses, and hashes are
frozen under `profiler/experiments/masonry-video-context-prewarm-2026-10-29`.
The normal worktree app (PID 43936) was verified alive after stopping only the
diagnostic launcher's exact executable tree.

AI assistance has been used to create this output.
