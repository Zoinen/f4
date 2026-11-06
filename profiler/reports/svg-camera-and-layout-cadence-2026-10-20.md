# SVG camera correctness and GUI layout cadence

The native scrolling camera is still experimental. Constant 160 Hz without
spikes is not established. No native camera or layout-throttling experiment
has been enabled in the normal embedded application.

## Correctness change

Cached SVG surfaces had an independent GUI viewport-membership presentation
gate. Moving only the native camera and delegate culling roots could leave a
ready SVG hidden. The regression reproduced this: the camera capture contained
the background color `#d35d4b` where the orange SVG was expected.

`GalleryVectorUploadGate` now supports an owner-scoped native camera binding.
The render thread uses the exact visible camera region for SVG presentation
and upload priority. A separate conservative region can be used for delegate
root culling without causing offscreen SVGs to submit draws. A binding snapshots
the source generation and readiness. Stale generations, incomplete CPU
preparation, failed generations and unready native surfaces remain hidden.
Ordinary unbound behavior is unchanged.

The diagnostic camera binds existing SVG gates during synchronization and
updates their presentation on native frames. It does not walk generated SVG
shape geometry. This is a correctness seam, not a completed scrolling component.

## Verification

At DPR 1.75, native D3D11 and OpenGL each passed the selected hit-testing,
raster-camera and SVG-camera regression set: five checks including setup and
cleanup, zero failures or skips. The SVG camera image exactly matches the
settled GUI reference. Geometry checks cover actual named native text leaves,
small secondary text, a checker raster and their effective scene transforms.
The captured SVG includes a thin authored white stroke.

Additional SVG checks cover readiness, CPU completeness, out-of-viewport
presentation, owner-checked detachment and stale source generation. The
generation replacement uses the native gate's source key, not an actual VFS
content replacement. No new source acquisition occurs in these guard checks.

Both backends also passed the neighboring rendering set: vector/raster scene
independence, retained nearby preparation, and viewport/brick pixel alignment
(five checks including setup and cleanup, zero failures or skips per backend).
This is selected coverage, not a passing full-suite claim. The previously
recorded Details visual-contract failure remains outside this work.

The freshly linked static host test passed `qmlImportsWithoutInstalledQt`
(three checks, zero failures or skips). The current diagnostic executable
passed the Windows system-DLL import audit. These do not replace packaged
embedded-launch/extraction verification; no payload was rebuilt or deployed.

Evidence: `artifacts/viewport-svg-before-d3d11.log`,
`viewport-svg-after-{d3d11,opengl}.log`,
`viewport-camera-neighbors-{d3d11,opengl}.log`,
`camera-imports-svg-175.log`, and `audit-rt-svg-host.log`.

## Display measurements

Instrumented D3D11 captures used the Downloads folder with 186 entries,
density 152, DPR 1.75, a 160 Hz display, wheel commands every 30 ms, and the
default 5000 us GC budget. Each case contains two 10-second cold blocks and
two 10-second warm blocks. General per-frame recording was disabled; bounded
native camera samples and PresentMon supplied the timing evidence.

A display interval above 9.375 ms is counted as a missed-refresh interval.
Counts below are intervals, not the number of frames lost in longer gaps.

| Case | Cold misses, blocks 1 / 2 | Warm misses, blocks 1 / 2 |
| --- | --- | --- |
| Exact viewport culling, SVG gates connected | 13 / 2 | 1 / 4 |
| Keep all prepared delegate roots active, same executable | 49 / 13 | 45 / 18 |
| New instrumented executable, unthrottled GUI layout | 0 / 0 | 0 / 0 |
| Same new executable, GUI layout interval at least 16 ms | 0 / 0 | 0 / 0 |

The prepared-root alternative is rejected as a performance improvement: it
missed 125 intervals versus 20 in the initial exact-culling capture. These
single runs do not isolate one expensive function or prove repeatability.

The later cadence comparison used executable SHA256
`ac4f125de7d275ad61b0a1975e7aa5e6c877b6ece3fb055024a32aaac8782a6d`
for both cases. It added coverage counters and an optional GUI update interval;
the earlier captures used the previous diagnostic executable. Do not attribute
the later zero misses to throttling: its same-binary baseline also had zero.
The four baseline display p99 values were 6.285, 6.296, 6.300 and 6.299 ms;
the throttled values were 6.278, 6.283, 6.274 and 6.296 ms. Run-to-run variation
remains material, so these samples do not establish a sustained guarantee.

All these samples have one stationary transition per motion block, rather
than a pause per wheel retarget. Native matrix correction error remains below
0.00013 logical pixels. That does not prove complete pixel correctness of
production chrome, cursor, sticky groups or lifecycle changes.

## Cadence tradeoff and remaining work

The 16 ms minimum interval reduced GUI update deliveries from about 2157 to
904 over the first cold capture including unmeasured settling. It increased
GUI/native pose lag: measured medians were about 58-65 logical pixels, with
maxima up to 189.5, versus baseline medians about 36-44 and maxima up to 132.4.
Counters describe GUI delivery calls, not exclusive layout CPU time.

Prepared-row bounding-union telemetry detected a 24.965 logical-pixel bottom
coverage deficit at camera Y 2507.251 while the GUI position was 2390.602 in
the throttled cold capture. This is inside the folder, not its terminal blank
padding. The union metric does not prove a visible hole or full coverage of
every column; it is a warning requiring rendered verification and stronger
per-row coverage checks. Normal folder edges show smaller expected padding
deficits in both cases. Throttling must not be promoted on timing results alone.

Next work is an explicit prepared-content safety boundary for the native
camera, with source/layout-change rebasing and rendered checks during fast
reversals. The prototype still redirects a logical animation to a dummy
target. A production component also needs cancellation, natural settlement,
hidden-tab behavior, resize/DPR transitions, software fallback, chrome/cursor
coordination, and static alignment after every stop. Branch clip work and GUI
synchronization remain hypotheses for further attribution, not proven causes
of the remaining missed intervals.

Capture inputs, source snapshots, results, images and identities are preserved
under `profiler/experiments/svg-camera-and-layout-cadence-2026-10-20/manifest.json`.
Executables and libraries are represented by hashes, not copied build outputs.

> AI assistance has been used to create this output.
