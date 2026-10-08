# Hardware sampling only during viewer presentation flights

**Date:** 2026-10-07 Europe/Moscow
**Scope:** ZoinGallery opening/closing and shell-managed Quick View flight.

## Evidence and cause

The existing quality path followed the changing presentation size during a
short flight: it requested changing reduction-pyramid levels, disabled the
source-sized conversion cache, and repeatedly switched the resting video
layer. Earlier live captures in `/tmp/f4-fly-diagnosis-iFoYqe/` measured paused
close GPU scheduler run-to-done p95 of 38.489--42.529 ms and playing close
p95 of 57.808 ms. These are completion latencies, not isolated GPU execution.

The user explicitly accepted cheaper scaling during the animation, but not
lower quality at rest or during ordinary playback, pan, or zoom. Use one
explicit flag rather than a resolution ladder, timers, or another cache.

## Implementation

- Propagate `hardwareSampling` from the ordinary viewer's `transitioning`
  state and the shell's actual presentation animation. Managed Quick View
  has a separate animation and does not set `GalleryViewer.transitioning`.
- Add an early image/video shader branch using the existing linear hardware
  samplers. Bypass quality pyramids, float conversion, and presentation layers
  only during flight. Retained pyramid objects are not drawn in this mode.
- Preserve existing crop, rotation, mirror, range/color conversion, opacity,
  checkerboard and rounded clipping. Packed/interleaved or straight-alpha
  formats use four decoded samples instead of filtering incompatible raw
  components. Explicit nearest-neighbor selection remains respected.
- Reuse uniform padding at byte 124. Image/video blocks remain 164/308 bytes;
  update and test both shader stages and all variants.
- Keep geometry continuous during animation and restore physical-pixel
  alignment and the original linear-light quality filter at its endpoints.
  Ordinary pan and zoom do not enable the new mode.
- Reject resting presentation caching when the effect is hidden. The former
  cache could reactivate during hidden-viewer cleanup. Final captures have
  one brief activation/deactivation pair at close completion rather than
  the provisional build's dozens of repeated switches; this is not a claim
  of zero layer transitions throughout close cleanup.
- Keep `[FIX:viewer-flight]` diagnostics behind the existing opt-in
  `F4_VIEWER_RESAMPLE_DRAW_TRACE`. Normal playback has no new per-frame log.

## Regression-first verification

`/tmp/f4-flight-regression-before.txt` records three failures before the mode
existed. `/tmp/f4-flight-hidden-before.txt` separately fails the actual hidden
effect's cache eligibility before adding the visibility guard.

`/tmp/f4-flight-final-focused.txt`: 78 passed, no failures/skips on Apple M2
hardware OpenGL. Checks include DPR 1 and 1.75, actual intermediate draw
counts after positive pyramids are retained, a new video-frame revision,
BGRA-premultiplied/NV12/YUV420P/P010 crop and all rotation/mirror combinations,
and pixel-exact restoration of the quality result. A checkerboard image
distinguishes hardware bilinear (127--128) from the resting linear-light
result (187--188); the optimized mode is not silently nearest-neighbor.
Ordinary panning keeps its exact-quality conversion behavior.

Related results:

- Full GPU: 298 passed, four known still-image failures, three real-file
  opt-in skips (`/tmp/f4-flight-gpu-full.txt`). The failures are a native
  blue-channel 1-LSB comparison and three magnified/rotated/negative-pan
  CPU-reference cases. Existing assertions were not weakened.
- Separate supplied-file decode/render/cache/benchmark: five passed
  (`/tmp/f4-flight-decoded.txt`); CPU resampling: 27 passed; reusable viewer
  primitives: six passed; interrupted-flight finalization: three passed.
- Final host embedded imports and managed Quick View retention/motion
  properties: four passed (`/tmp/f4-flight-final-host.txt`). The software
  test harness checks property wiring only; the application remains hardware
  rendered. The hardware host fixture has four passes and an additional
  failed-preview retaining the previous cyan image assertion. That failure
  path is outside this fix and its pre-existence was not established here.
- At 175%, actual viewer leaves are checked in scene coordinates, including
  unit transforms and restored endpoints. Hardware host captures include
  small secondary text and icons; no static text layout was changed.

## Final live measurements

Artifacts: `/tmp/f4-flight-fast-pyTO3I/`. Desktop: Apple M2/Mesa OpenGL,
Wayland/KWin, physical 2560x1600 at 60 Hz, DPR 1.5. Clip:
`/home/zoin/Downloads/IMG_20260527_193242+.mp4` (3840x2160).

Final ready captures include the whole flight. Filter GPU jobs by their queue
timestamp within the flag interval, then match their run/done fence events
against the complete trace. Qt QSG render TID is 788246; KWin PID is 1295.
No screenshots were taken during these final measurement intervals.

- `final-paused-close-ready`: flag interval 253204.571067--253204.763484;
  Qt 14 jobs, run-to-done median 1.351 ms, p95/max 7.045 ms;
  queue-to-done p95 7.062 ms. KWin run-to-done p95 4.027 ms.
- `final-playing-close-ready`: 253165.739981--253165.916578;
  Qt 30 jobs, median 0.662 ms, p95 3.003 ms, max 3.009 ms;
  queue-to-done p95 3.345 ms. KWin run-to-done p95 4.367 ms.
- `final-open-ready`: 253237.537965--253237.728084;
  Qt 17 jobs, median 1.005 ms, p95/max 2.868 ms;
  queue-to-done p95 2.879 ms. KWin run-to-done p95 1.942 ms.

Each final flight records 11 hardware-sampling draws and no intermediate
filter or conversion passes within the interval. Playing-close frame-end
intervals are 15.702--17.678 ms. Opening's first frame follows the flag by
27.840 ms, then intervals settle around 16.6 ms. Paused close still contains
one 40.848 ms frame-end gap despite the much smaller GPU-job latency. Its
non-GPU delay has not been causally isolated; do not claim all stutter or
cold-start resource/driver costs are solved by this patch. The provisional
build also showed a larger first-switch delay.

`final-paused-idle`: four seconds, zero accepted frames, conversions, filter
draws, or layer activations. Final screenshot `final-viewer-paused.png`
shows the real clip at 00:26 with the play button, confirming pause.

## Build and handoff

Rebuilt application/QML/test targets using the existing portable consumer
tree and cached static Qt 6.11.1/FFmpeg 7.1.5. No Qt/FFmpeg compilation,
Conan graph change, commit, or push. Native glibc <= 2.27, hardening,
embedded QML imports, extraction tests, and static Go audit pass.

Go remains `CGO_ENABLED=0` with `goffi_static f4_embedded_qt_host` and the
system Go cache. The final build initially exceeded the user's tmpfs quota;
only `GOTMPDIR`/test temporary files were relocated to an owned disk-backed
directory, removed when empty afterwards. No user files or build cache were
deleted. The last working app stayed alive until the replacement verified.

Canonical launcher SHA256:
`7642cbfd66548a603a89f8ca6c6217926a87ea7dcb7c5449dbb7d531a42e38e3`.
Native host SHA256:
`cb60475c3dc7c4c866b8ec7f42928a0492bbb66ba625bc4b0e2303efebe5f84b`.
Embedded payload SHA256/cache key:
`4ea9329dc012aeb603f6bc0feb32ec3c4ddfe41e41de871a581d533479ad875d`.

Only this checkout's previous launcher/host were terminated. Final launcher
PID 788113 and Qt host PID 788157 run directly with `--gui=qt`, Wayland and
OpenGL, using the existing isolated local-only test profile. The ordinary
profile's unrelated HC typed-nil panic was not changed. The independent
missing Qt audio-backend work remains in FIX_PLAN.md. Owned perf probes and
the input daemon were stopped; the clip remains visible and paused.

## Prevention / follow-up

Measure actual intermediate submissions, not just requested pyramid depth.
Test fresh frame revisions with retained caches and reverse/interrupted
transitions, including shell-managed motion. Keep hardware sampling explicitly
animation-only and require exact quality restoration plus scene-space pixel
alignment at fractional DPR. Separate GPU-job completion, frame cadence,
startup latency and idle redraw counts in reports. Additional Windows/Metal
hardware execution and packed/straight-alpha flight coverage would strengthen
the cross-platform regression set; they were not executed on this Linux PC.
