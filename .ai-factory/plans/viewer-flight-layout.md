# Viewer flight layout optimization

## Scope

Implement the confirmed viewport-refitting hotspot from the current native
profile. Preserve aspect ratio, source/target geometry, interrupted transitions,
animation-only hardware sampling, resting quality, and physical-pixel alignment.
The existing audio FIX_PLAN remains separate and unfinished. No dependency
rebuild, commit, push, or upstream update is part of this request.

## Settings

- Tests: yes, regression first; actual resting leaves at DPR 1.75.
- Docs: no (existing subsystem contracts unchanged).
- Build: existing Ubuntu 18.04 consumer tree and cached Qt/FFmpeg only.
- Benchmark: native frame-end cadence and animation callbacks, with instrument
  overhead and GPU scheduler completion reported separately.

## Tasks

- [x] Add a flight regression and demonstrate failure before the source fix.
  Three rows fail at DPR 1.75: inner viewport becomes 140x90, 170x50, or
  90x140 instead of 640x420 (`/tmp/f4-flight-layout-before.txt`).
- [x] Keep the inner viewport fixed and animate an aspect-preserving image
  presentation transform. Preserve pinch/navigation and restore identity at rest.
- [x] Run focused hardware geometry, quality, interruption, and 175% leaf checks.
  Final focused totals: hardware geometry/interruption/pinch 8 passed;
  software actual video leaves/imports at 175% 4 passed; related viewer
  primitives 8 passed; hardware quality/video-cache cases 77 passed.
  Fix the actual play-button origin at 652.5 physical pixels before accepting
  the composed overlay. Preserve wheel-zoom centering by limiting continuous
  flight centering to hardwareSampling. Offscreen hardware window readback at
  175% remains unreliable; do not count that capture as visual proof.
- [x] Inspect resource-lifetime evidence for the driver allocation stall; make
  no speculative memory/cache change without identifying its owner.
  A resource capture observed three GL texture creations and two buffer
  creations around a playing close, but incomplete Qt/Mesa stack unwinding
  prevents attribution to the specific driver allocation. No speculative
  resource-retention or memory-budget change was made.
- [x] Build/package/audit the portable application, retaining the working app
  until its replacement passes verification.
  Existing Ubuntu 18.04/GCC11 consumer build, embedded-only QML imports in
  Ubuntu 18.04, glibc 2.27/static-native/hardening audits, static Go audit,
  and embedded-host lifecycle tests pass. Qt/FFmpeg were not rebuilt.
- [x] Restart this checkout only, measure paused/playing flights and idle, then
  record the regression/fix evidence in the patch artifact.
  New canonical launcher and its matching embedded host run on Wayland/Mesa
  Apple M2 OpenGL. Closing interior animation-callback mean: 4.6635 -> 1.4239 ms;
  opening: 2.4131 -> 2.1628 ms. Viewport width/height callbacks: 20 -> 0 per
  flight. Four seconds of paused idle: zero frames and zero animation ticks.
  First-frame delay is not solved: playing closes reached 60.243 ms; the first
  post-launch callback capture reached 151.019 ms. One minimal-event capture
  also has a 34.824 ms closing-endpoint gap. See the preserved learning patch
  `.ai-factory/patches/2026-10-07-23.48.md` for evidence and test limitations.

## Remaining verification limits

Broader exploratory suites are not wholly green: path-control spacing,
details/zoom icon dimensions, software-only shader expectations, and an
unavailable viewportDoubleClickHandler property still fail. Those paths were
not changed for this fix, but no complete pre-task baseline proves that every
failure predates it. Cross-platform and shell-managed Quick View flights were
not measured in this run. Docs: no; this custom plan and the separate audio
FIX_PLAN are preserved. No commit or push was performed.
