# Actual Masonry render-thread camera experiment

Constant 160 Hz remains unachieved and the camera is not production-ready. This
turn adds an isolated native diagnostic host. The normal portable worktree app
is unchanged and remains alive. No commits or pushes were made.

## Scope and acquisition

The diagnostic uses the current production gallery/core and Qt libraries, adding
only a host-side hook and a QQuickAnimatorJob subclass. It captures the complete
PixelGridScope item-to-parent matrix during synchronized scene access and changes
only its scroll translation during render-thread animation. It never introduces
a layer or rasterizes the panel. Logical NumberAnimation initially remains in
place. Later diagnostic variants redirect its target to a dummy property and
feed the camera position back into the actual layout.

Captures use D3D11, the actual 160 Hz screen, DPR 1.75, Downloads with 186 entries,
density 152, a 2194 x 1186 window, 30 ms wheel input with boundary reversal, and
the default 5000 us GC setting. Each stage contains two ten-second measured
windows. Builds, analysis and UI Automation do not run during these windows.
All captured wheel events report the window active. Native bounded buffers have
65,536 slots; the final cumulative sample counts are approximately 6,512.

Initial scroll extent is 4101.142857; warm extent is 3649.142857 in all five
processes discussed below. The cold extent changes during metadata settlement,
so an initial equal extent is not proof of identical geometry throughout input.
The minimal recorder has no general per-frame scene observer. The camera's own
native sample buffer is implementation instrumentation and adds some overhead.
It records transform commits, not GPU completion or optical movement. PresentMon
independently records displayed changes within explicit native input windows.

## Results, including rejected shortcuts

| Variant | Cold display misses / 20 s | Warm misses / 20 s | Warm displayed p99, two blocks | Motion findings |
| --- | ---: | ---: | --- | --- |
| Ordinary control, same host hook disabled | 3 | 1 | 6.347 / 6.309 ms | No camera instrumentation |
| Initial parallel camera | 0 | 0 | 6.291 / 6.316 ms | 334 stationary transitions per ten-second block |
| Continuous retarget, parallel clocks | 0 | 0 | 6.287 / 6.272 ms | One stationary transition per block |
| Camera-authoritative, GUI afterAnimating | 0 | 0 | 6.372 / 6.383 ms | One stationary transition per block |
| Camera-authoritative, coalesced queued GUI update | 1 | 0 | 6.331 / 6.297 ms | One stationary transition per block; GUI lag remains |

A miss here is a displayed interval above 9.375 ms. Control miss maxima are
approximately 12.5 ms. The queued variant's cold maximum is 10.5269 ms. These are
single-process cold/warm samples per variant, not randomized repeated A/B trials
or a proof that all application spikes are eliminated.

The initial parallel camera falsely appeared smooth if judged only by PresentMon.
Each arriving command restarted interpolation at the previous frame's value and
thus repeated that value for one frame. There were exactly 334 stationary
transitions among approximately 1,600 samples in each block. The corrected job
first advances its existing segment to the current render time, then applies
the new destination from that position. It also publishes commands only after
the logical animation restart has its complete target/duration. The four
continuous-retarget blocks have only one stationary transition each, consistent
with the initial segment boundary. This is direct motion evidence, not merely
repeated presentation of an unchanged scene.

Nevertheless, independent logical and visual clocks diverge. The first parallel
camera's warm maximum difference reaches 105 px and the corrected version 129 px.
Cold corrected motion reaches 414 px during a changing layout. The matrix itself
preserves the non-scroll base offset to about 0.00013 logical pixels, so that lag
is not accidental double translation or discarded padding.

Redirecting the GUI NumberAnimation to a dummy property makes native camera
position authoritative; a GUI afterAnimating callback copies its latest value
into layout.contentY. This keeps existing visible-range/decode work on the GUI
thread. Inner vector/raster branch clip rectangles are corrected on the render
thread from that same camera. This variant still has warm logical/visual maxima
around 101-121 px. It does not solve current-pose hit testing or all culling.

The coalesced alternative queues at most one logical update, replacing pending
position data rather than adding key/event debt. Its four input windows plus
settlement produce 6,512 native commits and 3,308 delivered notifications; none
remain queued at the final snapshot. Warm median logical/visual lag is about
48.4 px, with maxima 101-124 px. This is not evidence of a lag reduction. The
cause of the delivery cadence is not yet attributed; do not infer it from the
notification count alone. The queued variant is not promoted.

## What this proves and what it does not

Real Masonry camera motion can continue at render cadence while its existing GUI
work occasionally stalls. There are 120 measured seconds across the first three
enabled variants without a displayed miss, and the continuous variants actually
change position on essentially every recorded frame. This supports continuing
with a native camera design; it does not establish the full 160 Hz goal.

GUI-owned sceneVisible/subtree-blocked state and prepared delegate coverage still
follow GUI layout positions. An outer clip or a corrected inner clip does not
make a missing/blocked entering row visible. Input must resolve against the
actual presented camera rather than an independent NumberAnimation. Cursor and
sticky/group chrome need the same pose. Metadata reflow and maximum-offset
changes need an explicit camera transaction. Natural settlement, interruption,
middle-button scrolling, keyboard jumps, path/mode switches, hidden tabs,
arbitrary DPR, source replacement and teardown remain unverified.

The screenshot inspected here is a normal full-window rendered capture at rest,
including small captions. There is not yet a full affected-leaf scene-space and
unit-transform regression for this camera integration, nor motion-frame image
comparison against a reference pose. Existing static layout geometry was not
changed. These missing gates prevent installing this diagnostic as the normal
portable app.

The next implementation should make the native camera a real component with one
position owner and a defined GUI/model handoff, rather than retain the hook's
dummy target and parallel legacy lifecycle. Rendering already-materialized
overscan must use the actual camera for culling, with bounded prepared coverage.
Then it needs actual-leaf geometry/image regressions and repeated display/motion
captures before rollout. Cold upload/driver/display spikes remain in scope.

## Evidence

Sources, captures, PresentMon data and analysis are preserved with SHA-256 checks
under `profiler/experiments/rt-panel-camera-2026-10-19/manifest.json`. The latest
diagnostic executable is identified by hash but not archived. Intermediate
executables were overwritten as the diagnostic evolved; source snapshots retain
the dual-clock and afterAnimating versions. Initial-version sources can be
reconstructed from the recorded two-line retarget change, but an original binary
hash was not captured, so bit-identical reconstruction is not claimed.

Four obsolete diagnostic launcher copies were removed only after their SHA-256
matched the preserved working launcher and their exact executable paths were
confirmed inactive, recovering approximately 626 MiB. Executables/libraries,
raw traces, bulk build outputs and unrelated temporary files are excluded from
the evidence archive.

AI assistance has been used to create this output.
