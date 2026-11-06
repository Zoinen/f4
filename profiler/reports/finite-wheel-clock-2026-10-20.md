# Precise finite-animation time during sustained wheel scrolling

## Finding

Sustained wheel input every 30 ms showed no missed display refreshes over the
initial 40-second capture. Producer intervals occasionally exceeded 9.375 ms
(maximum 10.9953 ms), while displayed intervals stayed below 6.381 ms. Renderer
preparation reached 4.2229 ms. These producer outliers are headroom concerns;
they are not evidence of missed displayed frames in this capture.

Movement was still uneven. A second capture records each rendered camera,
animation timestamp, segment start/from/to, duration, easing type and Qt's
precise clock. For unchanged OutSine segments, actual animation time advanced
by 6 or 7 ms while Qt's VSync time advanced by exactly 6.25 ms. Displayed curve
step error p95 was 5.35–5.43 physical pixels, despite zero missed refreshes.

## Change

Finite wheel/keyboard motion now accumulates the same optional precise Qt-clock
deltas as continuous motion. The job retains a separate integer Qt timestamp
for fallback deltas. Current time and segment start remain floating point through
easing, retargeting, geometry rebases and coverage clamping. No wall-clock
integration or per-leaf rounding is added. The existing initial request-age
handoff remains unchanged.

The hardware regression drives a linear 500-pixel/second native animation and
records actual rendered camera matrices during a GUI pause. At 160 Hz the
expected step is 3.125 logical pixels. Before the change it fails on D3D11 and
OpenGL with worst step error 0.375 logical pixels. After the change both report
zero measured step error. The test skips when Qt lacks the optional capability;
the production consumer still supports that configuration.

## Displayed-motion comparison

Downloads186, Masonry152, DPR1.75, 160 Hz; wheel input every 30 ms with direction
reversal before reaching boundaries. Cold and warm stages each have two
10-second windows: 40 seconds per before/after capture. General frame recorder
disabled, bounded material/phase probes enabled. No builds, tests, analysis or
UIA observation run during input windows.

The analyzer joins actual rendered transforms to PresentMon displayed times.
It checks that recorded timestamps and curve parameters reproduce each camera
pose. It excludes retarget, rebase and coverage-constrained pairs, then compares
observed movement to the same curve advanced by the actual display interval.

| Window | Before p95 error px | After p95 error px | After max error px | After max display interval ms |
|---|---:|---:|---:|---:|
| Cold 0 | 5.4301 | 0.0925 | 1.3113 | 6.4436 |
| Cold 1 | 5.3704 | 0.1003 | 1.2994 | 6.4242 |
| Warm 0 | 5.3557 | 0.1044 | 0.7112 | 6.4094 |
| Warm 1 | 5.4057 | 0.0922 | 0.8628 | 6.4148 |

No displayed interval exceeds 9.375 ms in either detailed capture. All compared
after-animation steps are 6.25 ms. There are zero missing presentation brackets.
A p95-motion-error <=0.5px gate fails the before capture and passes the after
capture, also requiring zero missed refreshes and complete presentation coverage.

Only 464–488 pairs per window meet the unchanged-segment criteria; approximately
1107–1132 pairs per window are excluded. This metric establishes the clock fix,
not smoothness across every retarget/rebase boundary. Display-time variation
also leaves individual errors up to 1.31px. Preserve those limits rather than
calling the full constant-160-Hz objective complete.

## Verification and deployment

- 34 native checks pass: 17 each on D3D11/OpenGL, no failures or skips. Includes
  the new rendering regression, clock-domain fallback, continuous motion,
  hide/minimize, interruptions, scrollbar tracking, metadata rewrap, shrinking
  extents and default input wiring.
- 22 checks pass against the preceding Qt Quick library without the clock
  capability: 11 each on D3D11/OpenGL, no failures or skips.
- Existing 175% resting-leaf scene-origin/unit-transform checks and exact
  restored-render comparisons pass. No static UI layout changes were made.
- Portable QML import isolation, Windows system-DLL audit, embedded extraction
  tests and CGO-disabled packaging/build pass. Only existing duplicate plugin
  object warnings remain.
- Source diff checks pass, with the existing mixed-line-ending warning for
  PIXEL_GRID.md. Its preexisting non-UTF8 byte was preserved with byte edits.

The worktree launcher was replaced only after the new package succeeded.
`D:\Code\f4-zoin\f4-zoin.exe --gui=qt` is responsive, with no host or frame-latency
override. Its extracted host hash matches the rebuilt host. The canonical
checkout was untouched. Nothing was committed or pushed.

- Launcher SHA256: `889487bbfc7215a0c3416b57be3cef5764ca381c56cb31218d3441b67019ec68`
- Host SHA256: `d7bd3ed296fe09a54b0f037e7a77bf3976d10c56a8033dcf626bfd0c1e3c54d5`

Verified compressed evidence is listed in
`../experiments/finite-wheel-clock-2026-10-20/manifest.json`. Two old raw scheduler
CSVs were losslessly compressed to recover space; decompressed SHA256 equality
was verified before removing originals. No evidence was discarded.

Next: quantify motion at command-retarget boundaries and first handoff, then
exercise varied folders/input speeds and remaining modes. The goal remains active.
