# Native Masonry scrolling enabled in GalleryPanel — 2026-10-20

## Authoritative behavior

GalleryPanelMotion explicitly enables its GalleryScrollAnimation native path.
The C++ eligibility checks still require hardware rendering, visible window and
layout, clipped non-contained Masonry, contentY, finite bounded target and a
positive duration. Other modes and software rendering retain GUI animation.
There is no new setting, module, Go wire change, or generic application switch.

A new integration regression initially failed on both D3D11/OpenGL because the
panel required an opt-in. After the QML change it uses panel wheel handling
without setting nativeEnabled in the test, reverses a running sequence in the
same job, settles at zero, starts again, and interrupts with setPanelContentY.
The explicit jump remains at300 after pending events drain. Switching to Grid
then uses GUI animation. The older fallback test now explicitly disables native
mode for its ordinary-GUI check rather than assuming the panel default is false.

## Verification

- Input/startup/interruption/thumb suite:10 passed per backend,0failed/skipped.
- Wheel/middle/scroll-alignment suite:29 passed per backend,0failed/skipped.
  All five modes, grouped middle motion, stale viewport restoration, idle/stop
  behavior, and actual resting scene origins/unit transforms at DPR1.75.
- SVG integration:12 passed per backend,0failed/skipped, including all panel
  modes, viewer swaps, cache reuse, gradients/transparency and progress.
- Software fallback:3 passed,0failed/skipped.
- Current resting Masonry capture inspected, including the small captions.
- Portable import isolation, DLL import audit, embedded extraction tests and
  packaging passed. Parent/submodule diff checks pass with existing CRLF warnings.

## Default-path performance capture

Diagnostic configuration now supports F4_PROFILE_KEEP_NATIVE_DEFAULT. In this
capture it does not call setNativeEnabled; both capture identity and controller
snapshot assert the default was preserved and found enabled. Actual-link/source
hashes and bounded probe capacities pass. No builds/tests/analysis/UIA/scheduler
trace run during input windows.

Downloads186,Masonry152,DPR1.75,screen160Hz,defaultGC5000us,16MiB pools. Wheel
bursts200ms per second at30ms repeat,20cold+20warm starts over40seconds. Cold
extent4101.1429,warm4118.1429; metadata/layout extent differs from some earlier
captures. This is validation of the new default, not an isolated A/B speedup.

Added two requested idle frames during the existing2s pre-input settling period,
after PresentMon starts. Combined with the consumer lead-in, the first present
is now covered for every one of40 jobs. Startup analyzer confirms zero missing
first-presentation brackets, no no-motion jobs, no coverage stalls, and zero
within-job displayed intervals above9.375ms. Intentional idle gaps are reported
as startup latency rather than misclassified as missed animation refreshes.

| Metric |Cold|Warm|
|---|---:|---:|
|First motion commit median ms|5.4462|5.4012|
|First motion commit maximum ms|8.6862|7.8020|
|First motion displayed median ms|17.5090|16.8173|
|First motion displayed maximum ms|27.3283|20.5670|
|Measured startup presentations|20|20|

This workload tests repeated starts, not continuous high-speed scrolling. The
preceding continuous opt-in captures remain limited evidence, not proof of a
universal exact6.25ms bound. The earlier cold displayed miss is retained.

## Remaining objective

Middle-button AutoScrollController still advances contentY from GUI FrameAnimation.
Correctness checks pass, but it can still stall with GUI collection/layout work.
Its performance has not been solved by enabling the wheel/keyboard animation.
Next work must measure this path and move continuous motion off GUI frame ticks
while preserving speed, direction, dead zone, stop, bounds and rest alignment.
The constant160Hz without spikes goal remains active and incomplete.

## Portable deployment

Only exact diagnostic/worktree launcher trees were stopped; canonical binary was
untouched. Fresh worktree executable directly launches --gui=qt, no host override.
Live responsive extracted host matches rebuilt SHA256:
3480691b61cdae3ac90d29d8580df8c3a0cb4c9ce3089c0d8f7b632210d6898e.
Launcher SHA256:
a19f7b67eff4be736ce03dbb343fbd255d9e20b475876120d5f5a66d56c60d32.
No commit/push. Evidence archived with round-trip SHA256 validation.

AI assistance has been used to create this output.
