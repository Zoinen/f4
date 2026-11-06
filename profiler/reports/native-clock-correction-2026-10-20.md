# Displayed-motion clock correction and window suspension

## Result

The previous continuous-scroll clock refinement optimized the wrong proxy.
Steady-clock integration at render commit made committed movement agree with
wall-clock time, but produced displayed startup step errors of 10.4–12.3
physical pixels. Reverting continuous integration to Qt's animation timeline
removes those large errors. This corrects the implementation and interpretation
in `native-middle-autoscroll-2026-10-20.md`; that report and its evidence remain
unchanged as history.

The corrected capture has zero missed displayed refreshes in four 1.5-second
input windows. This is six seconds of observation, not proof that the full
constant-160-Hz objective is complete. Integer-millisecond motion quantization
remains, alongside the broader workload and presentation issues already recorded.

## Changes and regressions

- Continuous motion uses Qt animation time after its initial handoff. Initial
  request age still uses the steady clock once. Geometry limits are mapped into
  the render job's coordinate system before clamping.
- Native eligibility rejects hidden and minimized windows. Hiding/minimizing
  cancels and commits native motion, and disarms the middle-button gesture.
  Restoring the window does not integrate time spent hidden or restart a gesture.
- Four regression rows cover direct/gesture motion crossed with hide/minimize.
  Before the fix, direct-hide passes while the other three fail on both backends.
  Tests reject new hidden-window motion, verify stable poses while hidden and
  after restoring, then verify a fresh gesture works.
- At DPR 1.75, tests enumerate visible Text/Image leaves and check their final
  scene origins and unit-vector transforms. Restored rendering exactly matches
  a fresh rendering at the same committed content position. The saved
  gesture-minimize capture was inspected, including the small captions.
- After the clock correction, the focused suite passes 15 checks on D3D11 and
  15 on OpenGL, with no failures or skips. It also covers GUI-pause motion,
  scrollbar tracking, interruptions, metadata rewrap, shrinking extents and
  the default panel input route.

## Display measurement

Downloads186, Masonry152, DPR1.75, 160Hz. Middle pointer distance is
296.5 logical pixels (one quarter of the measured 1186-pixel window height).
Expected speed is 1627.433412 logical pixels/second. Cold and warm captures each
include both directions. Bounded renderer/material probes remain enabled;
the general frame recorder is disabled. No builds, tests, analysis or UIA
observation run during the input windows.

| Window | Display interval p95 ms | Maximum ms | Misses >9.375 ms | Display step error p95 px | Maximum px |
|---|---:|---:|---:|---:|---:|
| Cold 0 | 6.2633 | 6.3104 | 0 | 2.1548 | 2.2593 |
| Cold 1 | 6.2598 | 6.3197 | 0 | 2.1481 | 2.2297 |
| Warm 0 | 6.2625 | 6.3071 | 0 | 2.1534 | 2.3023 |
| Warm 1 | 6.2600 | 6.3226 | 0 | 2.1478 | 2.3385 |

Step error joins actual rendered camera transforms to PresentMon displayed
timestamps. It is not computed from requested camera positions alone. All four
windows have zero coverage stalls and zero missing presentation brackets.

The new optional analyzer gate requires maximum displayed step error <=4 physical
pixels, zero missed refreshes, native samples and complete presentation coverage.
It passes the original Qt-clock prototype, fails the wall-clock version at
10.4459 pixels, and passes the corrected build. Four pixels is a regression
ceiling separating these observed cases, not a smoothness acceptance target.
Commit-versus-wall-clock step errors are retained only as diagnostic data.

## Rejected latency experiment

One paired run requested D3D maximum frame latency 2 versus 1. The snapshot
records the requested environment value; it does not prove COM acknowledged it.
Latency 2 has no missed refreshes. Latency 1 has three missed refreshes across
the same six seconds, with gaps near 12.5ms and step errors up to 18.6 pixels.
Do not promote latency 1. This single pair is preliminary evidence, not a general
claim about every machine. The normal application has no frame-latency override.

## Build, deployment and evidence

Portable QML import isolation, system-DLL import audit, embedded-host extraction
tests and the CGO-disabled Go build pass. The worktree app was replaced only
after its corrected package was complete. Its extracted host is responsive and
matches the rebuilt host. The canonical checkout was untouched.

- Launcher SHA256: `d2a04426bf2cdfa0fe604c5a354b86c2dbf9dd32cae16cf2b9d57a0a67664752`
- Host SHA256: `40027f69125abd765597b5b05560af54ee1193b0693d694a1e75342369ddae5e`
- GalleryScrollAnimation.cpp SHA256: `5e1f19c65693782472ddb9be9bc953007a00794951cbce3dabe4920292667cc2`

`../experiments/native-clock-correction-2026-10-20/manifest.json` records verified
compressed source snapshots, tests, rendered captures, build identities, raw
capture JSON/CSV and analysis. Older prototype captures remain in the preceding
native-middle archive. Two old scheduler CSVs were losslessly compressed to
recover disk space; decompressed SHA256 equality was verified before removing
the originals. No evidence was discarded. Nothing was committed or pushed.

The next timing investigation should obtain Qt's full-precision render-animation
clock while preserving its frame pacing. A wall-clock substitute, fixed guessed
frame increments, or reducing queue depth does not establish that result. Further
longer captures and other input/workload paths are still needed for the goal.
