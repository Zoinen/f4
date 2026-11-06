# Precise Qt animation time for continuous Masonry motion

## Finding and implementation

Qt 6.11.1 already accumulates VSync animation time in a double, then exposes it
through integer-millisecond `elapsed()`. At 160Hz this quantizes 6.25ms ticks
into uneven movement. Unlike the rejected steady-clock-at-commit experiment,
the solution preserves Qt's own VSync timeline and its existing timer-mode
decisions. Source inspected:
[Qt qsgcontext.cpp](https://raw.githubusercontent.com/qt/qtdeclarative/v6.11.1/src/quick/scenegraph/qsgcontext.cpp).

The optional `f4.animationClock.v1` renderer resource exposes a versioned function
table. A thread-local sample exists only during the default driver's synchronous
VSync animation callbacks. It is restored after callbacks, including nested
calls. Timer mode, other threads and unsupported drivers have no valid sample.
The Qt recipe patch checks unique source anchors and validates all files before
saving either one. The generated recipe is self-contained and refreshable.

ZoinGallery resolves the capability during render synchronization and caches it
for that job. Only continuous auto-scroll uses precise deltas. Finite wheel and
keyboard animations retain their existing timebase. `GalleryAnimationClock`
checks version, table size, callback, sample availability and finiteness. It
falls back to Qt's ordinary delta across unavailable periods, provider changes
and driver restarts, avoiding accumulated hidden/domain-transition time.
Invalidation clears both capability and previous sample. First-request handoff,
coverage constraints, priority and pixel-grid behavior remain as before.

## Measurements

Same Downloads186 / Masonry152 case as the preceding clock-correction report:
160Hz, DPR1.75, middle-pointer distance296.5logical pixels, expected speed
1627.433412logical pixels/second. Four1.5-second windows per capture, cold/warm,
both directions. General frame recorder disabled; bounded renderer/material and
phase probes retained. No builds, tests, analysis or UIA during input windows.

| Capture | Displayed step error p95 physical px | Maximum physical px | Missed refreshes |
|---|---:|---:|---:|
| Previous production integer clock | 2.148–2.155 | 2.338 | 0 |
| Direct precise-clock prototype | 0.045–0.073 | 0.737 | 0 |
| Counted repeat | 0.040–0.083 | 0.716 | 0 |
| Versioned-interface prototype | 0.046–0.177 | 0.726 | 0 |
| Final production consumer | 0.046–0.089 | 0.716 | 0 |

Final display interval maxima:6.3831,6.3379,6.3347,6.3397ms. There are zero
coverage stalls and zero missing presentation brackets. The gate requiring
maximum displayed step error <=1px, zero misses and complete capture coverage
passes. This is a regression gate for this workload, not the full goal criterion.

Counters confirm the counted prototype uses precise samples530times cold and
1058times cumulatively warm, with2/4unavailable initial callbacks. The versioned
prototype records528/1059precise samples and2/4unavailable callbacks. The
remaining worst errors are near gesture startup; subpixel startup handoff is
still measurable. These captures cannot establish unlimited spike-free behavior.

Forcing Qt's simple elapsed-timer driver produces0precise samples and532/1062
unavailable callbacks. The existing clock remains active. Its captured startup
errors remain large (up to17.8px); this is fallback compatibility evidence,
not an improvement claim for timer-driven presentation.

## Regression and portable-build checks

- 32 native checks pass:16each on D3D11/OpenGL, none failed/skipped. Includes
 precise fractional deltas, missing API/version/size/callback, invalid samples,
 driver restart, reset/provider replacement, hide/minimize, GUI-pause movement,
 scrollbar tracking, geometry changes and default panel input.
- 18 additional checks pass with the final consumer linked against the preceding
 Qt Quick library that has no clock capability:9each on D3D11/OpenGL.
- 36 Python Qt-recipe tests pass, including new source-drift atomicity and recipe
 ordering/refresh/idempotence checks.
- 175% resting checks enumerate actual visible Text/Image leaves, verify final
 scene origins and unit transforms, and compare restored rendering exactly to
 a fresh rendering. The gesture-minimize capture was inspected, including its
 small captions. There are no new visual layouts in this change.
- Portable QML import isolation, Windows system-DLL audit, embedded-host extraction
 tests, and CGO-disabled packaging/build pass. Existing duplicate QML-plugin
 object linker warnings remain; no new failure was ignored.

## Deployment and remaining work

`D:\Code\f4-zoin\f4-zoin.exe --gui=qt` is running the new portable payload.
Its responsive extracted host matches the rebuilt host. No host override or
frame-latency environment override is active. The canonical checkout was untouched.

- Launcher SHA256:`4ef6e306f06e29be521f1a7360eb1b02b2d79d11997a0a76828b74d263be3758`
- Host SHA256:`953a10bd569285f679cc2ab80839af810a437ffdb18117e926ed140cb40c6b72`

Verified compressed sources, identities, logs, test results and raw captures are
listed in `../experiments/precise-animation-clock-2026-10-20/manifest.json`.
Preceding baseline evidence remains in the native-clock-correction archive.
No commits or pushes were made.

The constant160Hzgoal is still active. Longer varied workloads, rapid wheel
input, finite animation timing, and startup handoff require further measurement;
other view modes and forced timer-driver presentation are not proven by these
short continuous-Masonry captures. No cross-platform runtime performance claim
is made from Windows checks.
