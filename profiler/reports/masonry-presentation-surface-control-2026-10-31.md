# Masonry: opaque surface, fullscreen and composition capabilities

## Outcome

Constant 160 Hz remains unachieved. The host already disables the default
alpha buffer and paints an opaque window color. A fullscreen diagnostic that
really covers the 3840×2160 monitor still uses Composed Flip for every recorded
application present. DXGI reports no windowed hardware-composition support on
this output, and the same direct-overlay capability for RGBA8 and BGRA8.
There is no evidence here for changing the application's alpha, format or
normal window behavior as a performance fix.

The preceding goal turn made progress by finding heap contention and testing
retained render-order storage. Its repeated capture did not establish smoother
delivery. This turn tests a different possible bottleneck: whether normal
window bounds unnecessarily keep the application on the compositor path.
The result excludes a simple fullscreen/opacity shortcut in this environment.
It does not establish why Windows declines a different path or prove that
all remaining stalls originate outside the application.

No production source, installed Qt archive, embedded payload, normal application,
Windows setting, commit or push changes. Fullscreen is an attribution control,
not a substitute for the original normal-window acceptance scope.

## Diagnostic host and safeguards

One host SHA256
`5058ea262d0017d7d7c0a44e10c0aad29264ad3c94c93bac4d170b373c5f4418`
adds after-measurement surface metadata and a diagnostic fullscreen switch.
Renderer objects, image quality, native vector rendering and resource policies
are unchanged from the preceding control host. Buffer prewarming remains off;
inline dependency sets remain on, sparse snapshots remain off and frame latency
remains 2. The unproven retained-sequence prototype is not included.

Geometry persistence is redirected to a fresh file in each diagnostic cache,
initially copied from the original geometry file when present. Theme/settings
lookup arguments remain unchanged. Thus these controls do not save fullscreen
state into the normal application's geometry file.

The full static QML/plugin/resource response file is retained. Three
import-isolation QtTest results pass and the system-DLL dependency audit passes.
An initial diagnostic compilation exposed Windows min/max macros entering Qt
headers; defining NOMINMAX before windows.h fixes it. The rejected build log
is retained. This was a diagnostic setup error, not a Qt source defect.

All valid captures use actual Downloads, 192 entries including parent, Masonry
density 152, 160 Hz and actual DPR 1.75. A fresh isolated gallery cache and the
normal full-folder tour precede each pair of ten-second wheel windows. Every
tour waypoint checks all ready/definitively failed previews, no pending work,
at least 500 ms stability and a subsequent swap. All 192 indices are visited.
Native foreground is stable, inactive inputs are zero and coverage holds are
zero. No UI Automation, screenshots, builds or offline analysis run during
timed gestures. No kernel or additional graphics logger runs during these
delivery controls; PresentMon supplies delivery timestamps.

## Actual surface state

| State | Normal A1/A2 | Fullscreen B2 |
| --- | --- | --- |
| Native client origin | (0,0) | (0,0) |
| Native client size | 3840×2076 | 3840×2160 |
| Native outer bounds | (-12,-12)–(3852,2088) | (0,0)–(3840,2160) |
| Native monitor | 3840×2160 | 3840×2160 |
| Native work area | 3840×2076 | 3840×2076 |
| DPI for window | 168 | 168 |
| Qt color alpha | 1 | 1 |
| Requested/actual Qt alpha-buffer size | -1 / -1 | -1 / -1 |
| Native extended style | 0x100 | 0 |
| Qt visibility | Maximized (4) | FullScreen (5) |

Alpha-buffer value -1 is unspecified format metadata; it is not a measurement
of the DXGI swapchain's AlphaMode. The existing host source explicitly calls
`QQuickWindow::setDefaultAlphaBuffer(false)`, the clear color is opaque, and
neither native style has WS_EX_LAYERED. Do not infer an unmeasured DXGI field.

A separate read-only native-bounds probe verifies B2's fullscreen geometry
before timed input, using the activation-handshake PID/HWND. The final packet
independently records the same native bounds after input. Its helper changes
only its own thread's DPI-awareness context and restores it. No screen mode or
OS setting is changed by this probe. Fullscreen exposes an additional 84
physical pixels vertically, so its scene workload is not identical to the
normal-window workload; no causal speedup is inferred from frame-cost deltas.

## Presentation and delivery

Planned execution order is normal/fullscreen/fullscreen/normal. B1 fails its
warm-up gate, so this is not a complete A/B/B/A performance comparison. B2 is
an explicitly named fresh repeat, not a replacement for the failed evidence.

All application-target CSV rows are Composed Flip: A1 3643, B2 3674, A2 3608.
These are filtered by the exact recorded application PID. Hardware Legacy Flip
rows in the unfiltered CSV belong to DWM, not F4.

| Case/window | Preparation max (ms) | Display interval max (ms) | Intervals >9.375 ms | Maximum motion error (physical px) |
| --- | ---: | ---: | ---: | ---: |
| Normal A1/0 | 4.5347 | 12.4837 | 1 | 109.00358 |
| Normal A1/1 | 2.4899 | 6.4598 | 0 | 2.60191 |
| Fullscreen B2/0 | 2.5202 | 6.5754 | 0 | 5.96499 |
| Fullscreen B2/1 | 2.3414 | 6.4088 | 0 | 2.54107 |
| Normal A2/0 | 2.9962 | 6.4313 | 0 | 3.58355 |
| Normal A2/1 | 3.3917 | 6.5001 | 0 | 3.80975 |

Input counts A1/B2/A2 are 1849/1841/1801. All six windows fail the original
maximum-motion limit of 2 physical pixels. The fullscreen case is diagnostic
only even if an individual delivery statistic looks favorable. No cold or
quarter-height-middle acceptance result is added by this turn.

## Rejected B1 preparation

B1 stops at the unchanged 60-second warm-up deadline with
`timeout-with-normal-content`, before timed input. The last waypoint has one
unready preview at index 144, `PlayerControl+FileIcons_Dark++.svg`:

- Vector state is `ready`, but the preview has no thumbnail source and is not
  marked thumbnail-expected.
- Preview rectangle intersects the viewport; 17 of 18 expected previews are
  ready and there are zero pending bitmap requests.
- A descendant sourceStatus is 1. This does not by itself prove the intended
  vector is rendered; a fallback child can have its own status.
- The source is a 1500-byte SVG with root viewBox `0 0 811.8 9.2`.

B1's application has 277 Composed Flip presents during preparation. Its failed
tour, wrapper errors, display-session result and raw CSV are preserved. No
definitive SVG decode failure or exact visibility/geometry handoff cause is
established. B2 completes the tour unchanged, so do not report the symptom as a
deterministic fullscreen defect or weaken the gate to hide it. Future failure
diagnostics should record the delegate's geometry/identity flags, actual vector
URL and authoritative layout-visible indexes before choosing a fix.

## Read-only DXGI capabilities

The probe selects the active DISPLAY1 output on the NVIDIA GeForce RTX 4090,
adapter LUID 87088:0. Native output bounds are 3840×2160. Device creation and
all capability calls succeed. Returned data:

- `SupportsOverlays`: false.
- Hardware-composition flags: 5 (fullscreen capability plus stretched cursor;
  windowed capability absent).
- RGBA8 and BGRA8 overlay flags: both 1 (direct support, scaling absent).

Capabilities do not guarantee that Windows will select a presentation mode.
In particular, fullscreen capability is reported while F4 still presents through
composition. Equal format results do not justify a BGRA swapchain experiment
as the next application change. An earlier helper query used DPI-virtualized
desktop coordinates; that output/source are retained and corrected by setting
only the helper thread to per-monitor awareness, then restoring it. Capability
flags are unchanged.

## Remaining work and preservation

Return to the remaining application preparation spikes and verify why a raw
presentation control, if needed, differs from Qt before altering window flags.
Keep intermittent SVG readiness failures distinguishable from real decode
failures and from a valid warm tour. The original normal-window cold wheel,
verified-warm wheel/tour, middle scrolling, image quality, alignment, delivery
and motion requirements remain in scope and unfulfilled.

Sources, receipts, comparisons and rejected evidence are frozen in
`profiler/experiments/masonry-presentation-surface-control-2026-10-31`.
Large packets, CSVs, binaries and captures remain external with hashes. The
first display hold expires at its explicit bound during offline investigation;
a fresh owned hold covers B2/A2. Both holds and all owned diagnostic sessions
are terminal. The normal worktree application remains alive unchanged.

AI assistance has been used to create this output.
