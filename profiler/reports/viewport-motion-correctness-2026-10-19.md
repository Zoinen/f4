# Native viewport motion: input, culling, and remaining SVG presentation

The constant-160-Hz goal remains unachieved. This turn implements and verifies
the shared camera correctness seams in ZoinGallery. The normal application still
uses its existing scrolling component; the new camera is not deployed. No commit
or push was made.

## Implemented behavior

GalleryViewportMotionState holds an active flag and committed vertical camera
position in atomics, plus a render-thread-owned culling rectangle. A Masonry
layout may attach this state. Its point hit test and rectangle selection then
correct the GUI scope's actual applied motion offset against the committed
camera position. Inactive state and other modes keep their original mapping.

Prepared GalleryDelegateItem roots may bind their native GallerySceneItem node
to the same state. The render thread updates the blocked bit when the root's
bounds enter or leave the declared culling region. Intrinsic child gates keep
their existing thumbnail/readiness/mode semantics. Clearing a binding verifies
its owner, preventing stale cleanup from removing a replacement camera binding.
The ordinary blocked state remains available for detachment or inactive motion.

The diagnostic camera now attaches these APIs, snapshots only prepared delegate
roots during synchronization, and updates their culling bits during native
motion. Its normal viewport matrix, physical offsets, and clip corrections are
preserved. These hooks are diagnostic code, not the final scrolling component.

## Reproduced failures and passing regressions

The tests ran against no-op culling hooks and unchanged hit testing first. At
DPR 1.75 on D3D11, hitTestingUsesCommittedViewportCamera returned row 4 where the
camera showed row 24. The native culling test found the entering row blocked at
the older GUI pose. Both failed, with no skips. Those original logs are retained.

After the fix, both regressions pass on D3D11 and OpenGL: four QtTest checks per
backend including setup/cleanup, zero failures/skips. Point and rectangle queries
cover three positions and reversal while logical contentY stays zero. Disabling
the camera restores the previous mapping. The native test also verifies that a
stale owner cannot clear the current binding and that owner detachment restores
ordinary culling without depending on a visibility-property change.

The rendered camera image equals the settled GUI reference image exactly on
both renderers. The fixture explicitly checks the actual camera-transformed
origins and unit transforms of three named leaves: the normal caption, the small
secondary caption, and the raster checker icon. Their physical coordinates are
whole pixels at DPR 1.75. The rendered small caption and checker capture were
inspected. This fixture contains no SVG; it is not complete coverage of every
actual Masonry caption/style or the final scrolling component.

The neighboring tests masonryVectorAndRasterScenesAreIndependent,
nearbyPreparationSettlesWithRetainedVectorScenes, and
viewportAndBricksStayOnPhysicalPixelGridAt175Percent pass on both renderers:
five checks per backend including setup/cleanup. The last test exercises all
five modes and fractional ancestor placement. This is a selected suite, not a
claim that the entire existing suite passes. The previously documented unrelated
legacy Details assertion is not changed.

Qt's meta-object pass emitted an empty test moc when the new fixture bodies were
inline. Keeping their declarations in the test class and the bodies out of line
behind Q_MOC_RUN exclusion restores generated slot metadata. Both slots execute
in the native runs; they are not omitted or skipped by this workaround.

## Actual-panel timings: correctness does not yet meet the goal

The camera-aware Downloads capture uses D3D11, DPR 1.75, 160 Hz, 186 entries,
density 152, 2194 x 1186, 30 ms boundary-bounce wheel input, and normal 5000 us GC.
Each cold/warm stage has two ten-second native measurement windows. No build,
analysis, or UI Automation observer runs during measurement. Buffers stay below
65,536 records. The warm final snapshot contains 138 culling roots. Logical input
state may lag the camera, but hit testing can now query its committed position.

The first camera-aware capture has five cold and two warm displayed intervals
above 9.375 ms in forty seconds. The warm maxima are 12.5118 and 12.4656 ms.
Native motion has only one stationary transition per block, rather than the
per-command freezes found in the previous experiment. The correct root-culling
path therefore still has frame spikes.

A second instrumented capture records snapshot and culling-update costs. It has
fourteen cold and six warm displayed misses. It adds timer reads and before/after
blocked-bit observations, so these counts are not a clean performance comparison
with the first run. Captures are single-run samples and system/run variation is
not excluded.

Warm snapshot medians are 0.1671 and 0.1622 ms; p95 0.2835 and 0.2638 ms; maxima
0.4866 and 0.7798 ms. Samples are deduplicated by synchronization ID for that
statistic. Applying culling has medians 0.00890 and 0.00855 ms, p95 0.1011 and
0.0993 ms, maxima 0.4087 and 0.3646 ms. Seven roots change in the nearby native
commit for each of the six warm misses. Their corresponding producer intervals
are 14.74-17.74 ms, far larger than the measured native culling loop. This is a
correlation that makes downstream renderer rebuild work a candidate; it is not
proof of exclusive cause. Whole renderer/driver/scheduling attribution remains.

## Prepared-margin prototype, not measured or promoted

The diagnostic has an optional conservative culling region covering only its
already-prepared roots, guarded by the owning viewport clip. This would keep row
boundary visibility bits stable while the viewport clip masks offscreen pixels.
Two-pass snapshotting computes that region before changing node bindings, avoiding
transient blocking during slot reassignment. Unassigned or incomplete rows detach
their old camera binding. The diagnostic schedules ownership-checked render-thread
cleanup after synchronization when motion finishes.

This option is not deployed or performance-validated. It does not enlarge the
materializer's delegate window. A timing comparison is deferred because of the
separate SVG correctness gap below. No speedup is claimed for it.

## Newly identified SVG gate and next required work

GalleryVectorSceneCache binds a cached SVG's displayEnabled to its host's GUI
inViewport value. GalleryVectorImage passes that through the native upload gate's
present property, gated by sourceStatus. Updating only a delegate root's culling
bit does not update that SVG presentation gate. Consequently an SVG can still
enter late while the camera is ahead of the GUI. The earlier striped/raster
fixtures do not test or fix this path.

The final camera integration must give eligible SVG gates a camera-aware
presentation decision while preserving CPU/native readiness, immutable source
generation checks, upload priorities, hidden preparation, and ordinary viewer
semantics. It must not enable arbitrary intrinsic child gates or render incomplete
SVGs. That requires an SVG-specific rendered regression before another complete
camera performance claim.

After that, replace the dummy NumberAnimation target and diagnostic hook with a
real native scrolling component. Natural settlement, interruption, middle-button
motion, keyboard jumps, camera/model handoff, source/path/mode replacement, hidden
tabs, geometry changes, arbitrary DPR, and teardown still need complete coverage.
Prepared coverage, resource bounds, actual delegate leaves and motion-frame image
comparison remain rollout gates. The original objective is unchanged.

## Build and evidence state

The current native core and host build. Embedded-only QML import isolation passes
three checks; the current host's Windows import audit finds only system DLLs.
The Go payload is not regenerated or deployed for this unfinished integration.
The existing running worktree launcher and extracted host remain responding.

Exact sources, before/after failures and passes, rendered images, native samples,
PresentMon exports and analysis are archived with SHA-256 checks at
profiler/experiments/viewport-motion-correctness-2026-10-19/manifest.json.
Executables and libraries are identified by hash but excluded from the archive.
Diagnostic map/PDB files were compressed only after round-trip hash verification;
their original contents remain recoverable. No unrelated files were removed.

AI assistance has been used to create this output.
