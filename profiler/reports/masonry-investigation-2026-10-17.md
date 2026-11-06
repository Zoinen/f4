# Masonry scrolling investigation, 2026-10-17

Status: in progress. Sustained 160 Hz has not been verified.

Source-version correction: the initially inspected Conan source directory
`qt0c794cb867c61` contains Qt 6.10.1, despite the running library and installed
private headers being Qt 6.11.1. The exact 6.11.1 files have now been extracted
from the archive referenced by the current package's CMakeCache into
`artifacts/qt-6.11.1-source/`. Subsequent source reasoning uses those files.
The frame and native trace measurements come from the running 6.11.1 binary.
No product Qt library or recipe was changed by the rejected accounting probe.

The target is Masonry in `C:\Users\xs\Downloads`, at the existing density of 152,
on the 160 Hz display with actual DPR 1.75. The fixture currently has 183 entries,
including 11 SVG files. Each native measurement scrolls down for four seconds and
back for four seconds. The warm scroll extent is 3717 logical pixels. Only runs
whose native GUI samples all report an active window are used as headline data.

## Confirmed renderer cause

The static Qt 6.11.1 batch renderer forces a global full rebuild when removing a
clip root or a promoted transform batch root. An opacity-zero transition forces
one even without a clip. Local Qt source: `qsgbatchrenderer.cpp`,
`Renderer::nodeChanged()` and `Renderer::nodeWasRemoved()`.

Native dirty-node attribution in
`artifacts/modes-profile/masonry-structure-causes-masonry.json` identifies the
remaining expensive roots as SVG files. `FileIcons_Dark+.svg` has 26 geometry
nodes and 14051 vertices; `FileIcons_Dark+64.svg` has 26 nodes and 12054 vertices.
These are source-file graphics, not application interface icons.

The current candidate retains at most 16 recently used, fully clipped SVG tiles
without changing their logical visibility or input state. It preserves each tile's
source assignment and its clip tree. Ordinary bitmap/folder rows retain normal
culling. Measured full-rebuild flags fell to 7 in
`masonry-vectoraffinity-warm2-masonry.json`, compared with 34 in the earlier
`masonry-retirement-warm2-masonry.json`. Frame time did not meet the target:
p95 16.0483 ms, p99 19.3911 ms in the former run. Caption measurement count was 0.

The extended `masonryVectorSceneSurvivesNativeVisibility` regression passes at
175% scale, including one preserved tile across a long scroll, a 20-SVG working
set limited to 16 retained tiles, no orange SVG pixels outside the viewport, and
clearing retention on catalog replacement. This does not prove performance.

## Confirmed accessibility work

The WPR CPU trace `masonry-vectoraffinity-cpu-masonry.etl` contains repeated GUI
thread calls through `QWindowsUiaMainProvider::Navigate()`,
`QAccessibleQuickItem::childCount()` and recursive `unignoredChildren()`.
Qt's local source confirms that this enumeration walks non-accessible drawing
items recursively, including invisible retained SVG scenes.

The public symbol `QAccessibleQuickItem::textDocument` in the CPU report is an
approximate symbol for the adjacent static recursive helper. It must not be
reported as text-document editing work. Similarly, `QHttpNetworkReply::errorString`
is an ICF alias of `QQuickItem::childItems()` in the link map; it is not networking.

The subsequent drawing-leaf prototype still spent substantial CPU in that
enumeration (about 24.85% of the process's sampled weight, plus 11.73% in the
child-items alias). Its frame times were worse, so that prototype is not a proven
optimization. A grouped panel/list-item hierarchy was also insufficient:
`masonry-accessible-groups-warm2-masonry.json` has p95 17.4421 ms, p99 24.0187 ms.

The latest correction makes decorative accessibility leaves report
`invisible` and suppresses their geometry notifications. Qt's Windows provider
otherwise exposes `NoRole` as a custom control: its IsControlElement result is
always true, while `Navigate()` explicitly skips invisible items. A regression
failed on the missing invisible state before this correction. The strengthened
test also verifies that an explicitly accessible synthetic drawing descendant
does not escape the leaf, and that real geometry notifications still work for
the positive control while the drawing leaf emits none.

The corrected candidate passes the targeted accessibility, folder, vector
retention and five-mode vector alignment checks at DPR 1.75. Its two active-window
warm runs produced 1182 and 1193 native frame swaps over eight seconds
(147.75 and 149.125 FPS). Frame interval p95 was 14.2377 and 14.4904 ms;
p99 was 17.8128 and 18.4632 ms. Render CPU p95 was 2.8497 and 2.6597 ms,
with zero caption measurements. These results still miss the 6.25 ms target and
do not establish that accessibility traversal is the remaining dominant cause.
The corresponding files are `masonry-invisible-leaves-warm1-masonry.json` and
`masonry-invisible-leaves-warm2-masonry.json` in `artifacts/modes-profile/`.

## Other measured experiments

- Caching the opposite panel did not reliably improve frame times. No production
  opposite-panel layer cache was added.
- Retaining raster texture factories (124 resources, approximately 34.8 MB) did
  not reliably improve frame times. This remains diagnostic-only.
- Disabling Qt's Windows vblank thread improved some frame counts but did not
  reach the target. No production graphics/pacing override was added.
- Keeping SVG rendering unculled without retaining its tile assignment did not
  solve rebuilds and increased draw cost. That diagnostic is not enabled in the
  current product candidate.
- Removing Masonry's contained folder-preview clip is pixel-identical on hardware
  and software renderers at DPR 1.75, with every image bounded by its grid. Both
  regressions pass. This alone did not solve the remaining SVG rebuilds.

Intel-signed standalone PresentMon 2.6.0 was downloaded into `artifacts/` for
display-presentation checks. Its [console documentation](https://github.com/GameTechDev/PresentMon/blob/main/README-ConsoleApplication.md)
describes the display-change and dropped-frame metrics. The first timer-path
capture was inactive and is diagnostic evidence only. A final active-window
presentation measurement is now available in
`masonry-invisible-present-masonry-present-summary.json`. Both sweeps reported
an active window at DPR 1.75. They displayed 617 and 616 presents over about four
seconds each, with zero discarded presents. There were nevertheless 21 and 24
display intervals longer than 9.375 ms, and display-interval p99 was about
12.49 ms. Thus zero discarded presents does not imply uninterrupted 160 Hz.
GPU-active p95 was 1.51/1.38 ms; the missing refreshes are not explained by
steady GPU saturation.

The new WPR trace `masonry-invisible-nativecpu-masonry.etl` shows buffer-host
writes, shader-resource setup and batch uploads on the render thread. Its
export also contains work after the scroll; use the render-thread activity
window rather than treating all post-scroll GUI samples as scrolling work.

Additional stage timestamps in `masonry-record-control-warm1-masonry.json`
locate long render frames mainly in preparation (approximately 8-11 ms),
with command recording around 1-2 ms. Skipping empty-scissor draw calls in the
diagnostic host did not remove those preparation stalls. Removing redundant
raster-preview clips reduced median batches from roughly 379 to 235, but did
not remove the stalls either; neither experiment is a product change.

The native dirty-event trace attributes 1079 first BuildBatches transitions
to a QQuickMouseArea matrix change. The scrollbar probe is being corrected
to traverse visual children and record its changed-item count; its initial
QObject-only traversal did not prove that scrollbar objects were changed.

The corrected visual-tree scrollbar probes changed four handles, as recorded in
each result's `scrollHandleChanges`. Hiding them removed the continuous
BuildBatches flag (1168 frames with zero pre-update rebuild flags), but left
47 preparation stalls in `masonry-scrollbar-hidden-verified2-masonry.json`.
Isolating their clips also did not improve the repeated scroll measurements.
No scrollbar change has been adopted.

The later-stage diagnostic hook copies Qt's four-call render entry point in a
separate diagnostic executable only. Its link map confirms that override;
normal host builds do not contain it. This captures rebuild flags *after*
`QSGNodeUpdater::updateStates()`, before batch preparation clears them. In
`masonry-postupdate-warm2-masonry.json`, the flags are 38 partial (5), two
partial-plus-complete (7), and six full (65535) rebuilds. All 46 preparation
intervals over 5 ms coincide with those flags. The enum values are
BuildRenderListsForTaggedRoots=1, BuildRenderLists=2, BuildBatches=4.
The 38 events must therefore not be described as complete rebuilds.

An early accounting experiment, based on an older source tree, did not improve
performance (`masonry-orders-audit-warm-masonry.json`: p95 14.298 ms). That
experiment was superseded by the exact Qt 6.11.1 source and the combined changes
described below. Its negative result must not be treated as the final result.

## Shared scene branches and visibility gates

Masonry now has two bounded native clip branches: one for vector rows and one
for raster/ordinary rows. Partial raster updates can rebuild their own branch
without uploading the complex SVG geometry again. Software rendering and the
other panel modes keep their existing visual parents. Layout animations
temporarily restore the common parent and original slot stacking order.

The 175% native regressions cover the actual image and caption leaves, vector
source retention, clipping, accessibility, and animation parenting. A per-row
clip prototype was rejected: although it reduced long preparation intervals,
it increased ordinary render CPU/command recording and did not improve scrolling.

In `masonry-branches-warm2-masonry.json`, median render CPU is 1.30 ms, but p95
frame gap is 11.54 ms and p99 is 18.87 ms. Eighteen preparation intervals exceed
5 ms: nine complete-plus-partial rebuilds (7) and nine full rebuilds (65535).
Ordinary render work fits inside 6.25 ms; the intermittent rebuilds do not.

Native dirty-event tracing then identified avoidable opacity-node changes in
fallback and folder/mode Loader visibility. Drawing gates now hide those
subtrees in native Masonry without toggling their Loader opacity nodes. The new
fallback regression failed before the change and passes afterwards. Seven
targeted native tests, including all three folder-preview modes, pass at 175%.
At this stage warm runs still had three to four full rebuilds and roughly
151 FPS. The remaining clip removal belonged to an SVG that failed loading;
requiring a usable vector source before enabling that clip removed these
full rebuilds.

## Balanced Qt reservations and observer control

The exact Qt 6.11.1 batch renderer decrements available drawing orders in every
ancestor root when inserting geometry. On removal it returned the order only
to the immediate root. Repeated subtree visibility changes therefore exhausted
the ancestors and forced expensive complete rebuilds. The dependency source
patch now returns the reservation to the same ancestor chain.

The native regression uses two nested clipped branches and alternating drawing
visibility. Original Qt fails on the first cycle: the ancestor's available
orders drop from 1024 to 1004. The patched library preserves the initial count
and identical rendered pixels through eight cycles. Its actual leaf origins
and unit transforms pass at DPR 1.75; the captured framebuffer is 420 by 350
physical pixels for a 240 by 200 logical window.

The local normal host links a full rebuilt Qt6Quick static library, with no
symbol interposition. The immutable Conan package remains unchanged. The CI
recipe applies the same source patch and rejects stale recipe reuse. Twelve
recipe patch tests and eight portable-cache tests pass.

The computer-use observer itself also contributed Windows UI Automation work
on the GUI thread. Resetting that observer connection removed this measurement
overhead; accessibility remains enabled. Performance captures after that reset
must be distinguished from earlier observed runs.

`masonry-observer-reset-present-masonry-present-summary.json` records two active
four-second sweeps in Downloads at DPR 1.75 on the 160 Hz display. PresentMon
counts 638 and 637 displayed presents, with zero discarded presents. Display
interval medians are 6.2498 and 6.2500 ms; p95 is 6.2705 and 6.2770 ms, and p99
is 6.3752 and 8.3231 ms. Three intervals exceed 9.375 ms across the two sweeps.
This is approximately 160 FPS, not a claim that every refresh is uninterrupted.
GPU-active p95 is 0.6172 and 0.6449 ms. Warm preparation has no intervals over
5 ms and no complete/full rebuilds in the corresponding accounting capture.

The scene-branch change also required accepting the two owned branch parents
when retaining reset slots. The sparse directory replacement regression now
passes without reallocating the row slot.

## Final normal-library measurements

The final diagnostic host links the normal patched static library; only the
render-entry timestamps remain diagnostic. The first presentation capture after
the initial priming sweep displayed 630 and 635 frames in approximately four
seconds per direction (roughly 158-159 FPS), with zero discarded presents and
12 display intervals over 9.375 ms. This result is retained as
`masonry-final-normal-qt-present-masonry-present-summary.json`.

The repeat warm capture, `masonry-final-normal-qt-repeat-masonry-present-summary.json`,
displayed 639 frames in 3.99386 seconds and 638 in 3.98589 seconds: approximately
160 FPS in both directions. Display interval median is 6.2500/6.2499 ms,
p95 6.2630/6.2662 ms and p99 6.2944/6.3267 ms. Neither direction discarded a
present. Each direction has one interval above 9.375 ms (maximum about
12.49 ms); this still is not a guarantee of an uninterrupted refresh cadence.
Native render CPU median is 0.9254 ms and p95 1.8479 ms. Caption measurements
remain zero during the scroll. There are no complete or full rebuilds; one
partial preparation interval exceeds 5 ms (8.3788 ms).

## Verification and rollout

- The final native Modes run at DPR 1.75 reports **148 passed, three failed,
  one skipped**. All Masonry cases pass. Remaining failures are the Icons
  reorder-anchor cases (full and delta catalogs), plus Details' 49-row reset
  time budget (44.339 ms against 16 ms). These modes remain outside this
  Masonry optimization goal; the full suite is not described as green.
- Visibility assertions now inspect the actual drawing gate rather than
  QQuickItem::visible, which deliberately stays true for native scene culling.
- Geometry checks wait for animation completion; the old capture measured
  `folderPreviewTitle` at physical x=226.730432 during animation. The same
  actual leaves pass after settling. Group hit tests likewise use a still
  layout, and native catalog-frame timing warms the initial GPU pipeline.
- Targeted software renderer checks: **19 passed** at DPR 1.75. Vector session
  cache, native visibility, five-mode alignment, bitmap exclusion and resize
  checks: **12 passed**. Layout/delegate-pool tests: **25 passed**.
- Native Qt reservation regression: **three passed**. CI dependency recipe
  and portable cache tests: **12 and eight passed**, respectively.
- Worktree title, resource-only imports and compiled-host loading: **five
  passed**. The import-isolation test also loads an actual SVG with VectorImage
  while installed QML import paths are excluded. The `zoin` branch label is visible and centered. An added rendered
  regression caught a 22.857143-wide box for 22.953125-wide text at DPR 1.75;
  allocating the next physical pixel preserves the complete text. Leaf scene
  origins and unit transforms pass, and the final capture was inspected.
- Windows import audit passes: only system DLLs remain. Embedded extraction,
  reuse, concurrent first launch, corrupt-payload rejection and payload checks
  pass with CGO disabled and GOWORK off, using the system Go cache.
- The generated gzip payload matches native host SHA-256
  `5f1fea1b372710e3c0541e22d3a0bbeba6dacac1e788c5aaafc95b9247adc15f`.
  The worktree Go executable has been rebuilt with the embedded-host tag and
  launched directly with `--gui=qt`, without a diagnostic override or sibling
  host. Its running Qt child matches the audited host hash and lives in the
  cache directory named by the compressed payload's SHA-256
  `b2ac6b8865894096370bf34f4bad63b1aad57bb4c34898f9ade4aefc60cd40c1`.
  Runtime branch identity is `zoin`. Launch verification is saved in
  `artifacts/masonry-final-launch-identity.json`; the startup error log is empty.

AI assistance has been used to create this output.
