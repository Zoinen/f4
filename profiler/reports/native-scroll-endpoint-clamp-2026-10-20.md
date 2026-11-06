# Preserve forward motion when metadata clamps a scrolling endpoint

An additional native-scroll discontinuity was reproduced and fixed. When a
metadata batch shortened Masonry content, clamping only the animation endpoint
could move the camera backward before it resumed forward motion. This work
does not establish constant 160 Hz; ordinary native scrolling remains disabled.

## Reproduction

`nativeScrollClampsShrinkingGeometry` starts a forward native scroll near the
end of a 200-entry catalog. It lets the native camera advance while GUI work is
stalled for 25 ms, then publishes cached metadata with very wide aspect ratios.
The resulting extent is less than half its previous size and the translated
destination must be clamped. The test records all subsequent GUI pose handoffs
and checks their minimum against the restored viewport anchor.

Before the fix, OpenGL failed with an anchor of 2272.029814 logical pixels and
a later minimum of 2174.992380: a 97.037434-pixel backward displacement. D3D11
passed that initial run, so the failure is timing-sensitive rather than claimed
universal. Both used actual DPR 1.75 and the real QML scroll controller.
Logs are `scroll-shrink-before-d3d11.log` and `scroll-shrink-before-opengl.log`.

## Change

The native job now preserves its current translated pose when either the pose
or endpoint is clamped during render synchronization. It solves the segment's
origin at the current eased progress, keeping the existing job, start time and
duration. It does not restart the curve. Near completed progress it uses the
endpoint directly, avoiding division by a vanishing remainder. Ordinary
unclamped geometry translations keep their existing path.

This work is local to geometry synchronization. It adds no pixel-grid traversal,
leaf rounding pass, source read, or frame allocation. No claim about measured
frame-time improvement is made from source inspection alone.

## Rendered regression exposed a second defect

The shrink test's final rendered comparison initially failed on OpenGL even
though every named caption had a whole physical-pixel origin and unit transform.
13,204 pixels differed, primarily along three overlapping caption/row edges.
Very thin panorama rows allow caption paint to extend above their cell. Recycled
slots retained creation order, so scrolling away and back could change which
neighbor overpainted that caption. The before images and amplified difference
are preserved with the evidence.

Independent Masonry scene branches now assign each slot its current row index
as a stable Z-order key. Assignment/rebinding updates that key; no per-frame
ordering pass was added. Qt updates its child-order cache when the key changes.
The original common-parent
order is retained during layout animations and outside these branches. The
shrinking endpoint's native and GUI reference frames now match exactly on both
renderers. This addresses deterministic paint order, not a redesign of caption
layout on extreme panoramas.

## Verification

The metadata-continuity regression was extended from one transaction to three:
portrait, landscape, then square dimensions above the visible entries. Each
transaction lets render motion lead GUI bookkeeping and checks the visible
entry phase, native pose adoption, accumulated target translation, one job and
absence of false running-state transitions before final completion.

Selected hardware runs cover endpoint shrink, repeated metadata rewraps,
retargeting, stop/fallback, raster camera culling and SVG camera culling. The
SVG tests cover complete/ready/generation gates and stale-owner cleanup; they
remain lower-level camera tests rather than proof of every mixed-media geometry
transaction. Resting native frames are compared against GUI reference frames,
with named caption leaf origins and effective unit transforms checked at 175%.
The shrinking-endpoint test also captures and compares its specific final pose.

All seven selected behavioral tests pass on D3D11 and OpenGL with no skips:
the six checks above plus independent raster/vector scene branches. Each
backend reports nine checks including setup and cleanup. The static host also
passes the system-DLL import audit and embedded-only QML import regression.
The full suite and the existing unrelated Details pixel-contract failure are
not claimed resolved.

## Fresh repeated measurements

One fresh diagnostic executable ran native, GUI, native repeat, GUI repeat,
each with two 10-second cold and two 10-second warm blocks. Conditions remain
Downloads/186 entries, density 152, 160 Hz, actual DPR 1.75, 2194 x 1186 logical
window, 30 ms wheel repeat, bouncing direction, default 5000 us GC budget and
the diagnostic 2 MiB pixmap-cache cap. General frame recording and UI Automation
observation stayed off. No build, test or analysis ran during input windows.
PID and build-identity checks passed for every stage; bounded native buffers
did not overflow. The instrumented controller still has recording overhead.

| Capture | Cold long intervals, blocks 1 / 2 | Warm, blocks 1 / 2 |
| --- | --- | --- |
| Native | 0 / 0 | 0 / 1 |
| GUI | 1 / 1 | 1 / 1 |
| Native repeat | 1 / 3 | 1 / 2 |
| GUI repeat | 3 / 2 | 1 / 0 |

Long means a displayed interval over 9.375 ms. Native recorded eight and GUI
ten over 80 seconds each. The worst native display interval was 12.464 ms;
its repeated cold blocks had p99 approximately 8.343 ms. The earlier zero-long-
interval native result did not repeat. These runs do not prove a dependable
timing advantage or constant 160 Hz. They also do not isolate Z-order work as
the cause of variation across revisions and runs.

Every native block still used one job, with no cancellation, coverage-limited
frame or unready anchor. The native input/geometry continuity fixes survive
this real workload, while frame cadence remains unfinished.

## Correlating the remaining producer delays

`analyze-scroll-clamp-spikes.py` correlates QPC timestamps from native commits
and PresentMon for the exact process and single swapchain. For each Present
call it identifies the preceding native commit. The interval between adjacent
Present calls equals the interval between their associated commits plus the
change in commit-to-Present age; that identity is checked within 0.001 ms.

Across the eight native blocks there were 133 producer intervals over 9.375 ms.
Their associated commit intervals never exceeded 8.246 ms. Block medians of
the additional post-commit age ranged from 4.514 to 5.367 ms; the worst growth
was 9.265 ms. Those samples therefore put a significant part of the variation
after the controller commit, rather than inside its bounded application scope.
One display spike also had a nearby native commit gap of 11.743 ms, so the
controller's surrounding cadence is not claimed uniformly smooth either.

Several displayed spikes occurred with nearby native commit gaps around
6-7 ms, and Present API durations were short. These are correlations, not proof
of a particular driver, compositor, collector or Qt renderer function. The
next attribution should bracket post-commit render preparation/recording and
presentation phases before choosing another optimization.

## Packaged worktree build

The host was rebuilt, audited and embedded in a regenerated compressed payload.
Embedded-host tests passed using the system Go cache; the staged launcher's
build metadata confirms `CGO_ENABLED=0`. The worktree app was restarted directly
with `--gui=qt`, clearing the host override and build-machine import paths.
Its responding cached child is the rebuilt host, verified by SHA-256
`ca753821781a7fce4718cd9fbcb3038e2421d0e57e803489dfbee92e6fe5d4ee`.
The launcher is
`28a42852fc16635ca4f66a7add63e1f46da5348341830218309dd3364303b7ae`.
The previous working launcher was backed up before stopping its exact process
tree; the canonical checkout was not modified or terminated.

## Remaining integration

The native camera still needs complete cursor-overlay, sticky-heading and
scrollbar synchronization, middle-button integration, mixed-media transaction
coverage and broader cadence measurements before default rollout. GUI/QML
decorations currently derive their poses from the GUI-side content offset;
those bindings are not proof that they follow intervening render-only frames.

Evidence is preserved under
`profiler/experiments/native-scroll-endpoint-clamp-2026-10-20/manifest.json`.
No commit or push was made.

> AI assistance has been used to create this output.
