# Render-thread motion feasibility and remaining frame stalls

Constant 160 Hz remains unachieved. The running portable application retains the
previous bounded thumbnail texture cache and Qt's default 5 ms GC budget. This
investigation changes diagnostic programs only; no render-thread scrolling
prototype has been installed in the application.

## Current renderer attribution

The post-residency render capture uses the current gallery core and normal GC,
D3D11, DPR 1.75, 160 Hz, Downloads with 186 entries, density 152, a 2194 x 1186
window, and 30 ms boundary-bounce wheel input. Cold and warm stages each contain
two ten-second input windows. Despite the filenames containing `sampled`, these
captures contain native duration scopes, not CPU stack sampling or WPR traces.
Buffers have no dropped records. Analysis/builds/UI Automation do not run during
capture. Scope durations overlap and are wall time, not exclusive CPU totals.

Warm vector-gate construction has median 0.0504 ms, p95 0.0913 ms, p99 0.1441 ms,
maximum 0.3027 ms. It performs a median 5,760 ancestor lookups and visits eleven
gates (nine active median), but the measured cost does not justify rewriting it
as the primary spike fix. Gate finishing has maximum 0.0026 ms.

Warm window render time has median 0.7385 ms, p95 1.7581 ms, p99 2.3402 ms,
maximum 5.3734 ms. Full window-frame scope has median 0.9731 ms and maximum
6.3772 ms; command recording median 0.2172 ms and maximum 1.9293 ms. The scope
after rendering through frame end has maximum 0.7520 ms. These are distinct
instrument boundaries, not interchangeable measurements of one stage.

Two warm displayed misses are 12.4974 and 12.4952 ms. Their nearby producer
intervals overlap only 1.5505 and 1.5615 ms of renderer scopes. This does not
establish which display submission each scope caused, but does rule out claiming
that a long vector-gate traversal explains these misses. The cold renderer
maximum is 6.1871 ms, including 3.942 ms upload; cold upload spikes still exist.

## GC pressure hypothesis rejected

The independent GC-pressure capture records four stages, each two ten-second
windows, with the same normal 5 ms configuration. The native observers surround
tryForceGCCompletion, runFullGC, runGC and onEventLoop and record cheap allocator
counters. Committed regular/class allocator bytes are not total live heap size.
There is no full heap walk or per-allocation observer.

No tryForceGCCompletion or runFullGC call overlaps measured input in any of the
80 seconds. Forced-completion guard collisions therefore do not explain these
captured spikes. Ordinary runGC scopes reach 6.5801 ms; onEventLoop reaches
9.1889 ms. Several displayed misses overlap 6.16-7.52 ms of GC phases; others
have no GC overlap. Reducing the default budget is not promoted: earlier reversed
same-process comparisons were inconsistent, and atomic phases remain.

## Independent visual motion

The native RtScrollFeasibility harness compares GUI NumberAnimation with Qt's
render-thread YAnimator in a small striped scene. It injects a 150 ms GUI stall
both from a timer and inside a logical-position signal handler. The test selects
the actual 160 Hz screen, sets DPR 1.75, records the SG transform afterFrameEnd,
checks fifteen named visual leaves in final scene coordinates and verifies unit
transforms, and saves rendered PNG output before shutdown.

These are scene-graph callbacks and transform samples, not PresentMon displayed
cadence or an optical measurement. They do not prove actual Masonry performance.

With competing C++ position writes and intentionally skewed logical/visual
durations, the control jumps backward by 41.09 px on D3D11 and 66 px on OpenGL.
Suppressing GUI writes while the visual animation owns the transform eliminates
the completion backstep in the four tested owner cases. During the GUI stall,
24-25 SG frames advance the surface by 28.6-30 logical pixels. Maximum internal
stall frame gaps are 6.41-7.01 ms. All fifteen actual leaf origins are on the
physical grid at rest, with unit transforms, and final GUI position is -400.

## Interruption exposed a second handoff issue

Explicitly stopping the Animator invokes its finished callback. The initial
test incorrectly treated that callback as natural completion and wrote the
destination into the item, creating a large jump. A handoff guard prevents that
callback from committing a destination during explicit cancellation. Stop then
writes back a position within zero to one render frame of the latest recorded
SG position.

That corrected stop/restart path still fails the strict monotonicity regression:
the D3D11 forward-retarget capture has a 1.200012 px backward step. Its handoff
frame gaps are 6.17-6.32 ms, so this is a positional discontinuity despite regular
frame callbacks. A final aligned endpoint and no large jump are insufficient.
The failing regression is retained; this path is not production-ready.

A command-based render-thread job now passes six forward/reverse/interrupt
cases on D3D11 and OpenGL: update its destination without stopping it, choose
the segment start from its current render-thread value, and distinguish explicit
interruption from natural completion. Forward retargeting has no backward step.
The six maximum handoff frame gaps range 6.27-6.50 ms; all stall windows contain
24 moving frames, and all fifteen static leaves pass at rest. The interrupted
surface settles at its current position snapped to the physical grid.

Four further sustained-input cases each issue 62 commands over approximately
two seconds at a requested 30 ms interval. All finish after the final command,
reach -524, and have no backward step. Maximum moving-frame gaps are 6.523 and
6.669 ms on D3D11, 8.453 and 6.756 ms on OpenGL. A control with the original
2000 ms proxy duration also succeeds; extending the proxy duration to 60000 ms
is unnecessary for these cases and is not proposed as a production fix. The
native job adjusts the active segment duration when a command arrives.

The first burst regression correctly failed when a target of -518 logical
pixels mapped to -906.5 physical pixels. Burst endpoints now use physical-grid
coordinates; animation frames remain unsnapped. One interrupted-case test also
wrongly expected continued frame callbacks after the scene had become idle; it
now verifies the final submitted transform and rendered capture instead. An
inactive-window attempt was excluded and the complete active-window matrix
rerun. The final results report activeAtStall=true and screenHz=160.

This prototype depends on Qt private animation APIs and a diagnostic-only fixed
175% grid. It has not passed actual Masonry, text/icon sharpness, hidden/window
lifecycle, multiple-view, arbitrary-DPR or repeated start-after-idle tests. It is
not production-ready. The passing striped scene contains no text or thumbnails.

## Integration requirements still outstanding

Masonry currently scrolls through PixelGridScope.motionOffset, not viewport.y.
Adding a YAnimator on top would double the translation. Any integration must
give the visual transform one owner, preserve logical content position and
viewport/decode demand, account for the visual position in hit testing and
culling, maintain enough materialized content during GUI stalls, and return all
actual text/image leaves to the scene-space physical grid at rest. Retarget,
reversal, middle-button scrolling, keyboard jumps, source/layout changes, hidden
panels, screen changes and teardown all need coverage. Continuous visual movement
alone cannot guarantee fresh delegates or eliminate renderer/upload stalls.

The compact evidence archive is
`profiler/experiments/render-thread-motion-feasibility-2026-10-19/manifest.json`.
It preserves diagnostic sources, observations and captures with SHA-256 checks;
executables, libraries, raw traces and bulk CSV exports are excluded. Nothing
has been committed or pushed in this investigation.

AI assistance has been used to create this output.
