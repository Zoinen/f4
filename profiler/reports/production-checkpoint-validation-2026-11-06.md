# Production checkpoint validation — 2026-11-06

This checkpoint preserves the current accumulated implementation and research.
It is **not a clean release gate**. Failures below are retained explicitly;
their baseline status has not been established by rebuilding every earlier commit.

## Passing non-visual checks

- CI Python tests: 93 run, one skip; passed with UTF-8 mode and `GOWORK=off`.
- Profiler analysis tests: 71 passed.
- Go dirwatch, nativeui and plughost package tests passed.
- `TestFullBuildExcludesIOSDependencies` and architecture module-boundary check passed.
- Windows panel tests compiled after adding the existing iOS test-only WinTUN
  dependency/checksums. One test still fails:
  `TestWindowsLocationAccessErrorDialogShowsReasonAndResolvesThemeAtRenderTime`
  reports a normal-button palette attribute mismatch.

## Native Qt checks

The initial broad invocation used the Windows default Controls style, which is
not embedded in these Basic-style static fixtures, and multiplied the current
175% display by a stale 0.875 scale override. Its missing Windows-style imports
and DPR failures are not product-regression evidence. The GUI suites were rerun
with `QT_QPA_PLATFORM=windows`, `QT_QUICK_CONTROLS_STYLE=Basic`,
`QT_SCALE_FACTOR=1`, `QSG_RHI_BACKEND=d3d11`, `ZOIN_TEST_NATIVE_RENDERER=1`,
`ZOIN_EXPECT_TEST_DPR=1.75`, and `ZOIN_EXPECT_TEST_API=d3d11`.
Core/cache-only rows below come from the initial invocation where applicable.

| Suite | Result |
| --- | --- |
| `F4QtBatchRootOrdersTests` | 46 passed, 0 failed, 0 skipped, 0 blacklisted, 11310ms |
| `F4QtCurvePreprocessTests` | 6 passed, 0 failed, 0 skipped, 0 blacklisted, 956ms |
| `F4QtQmlSignalMaskTests` | 9 passed, 0 failed, 0 skipped, 0 blacklisted, 260ms |
| `ShellStateStoreTests` | 10 passed, 0 failed, 0 skipped, 0 blacklisted, 30ms |
| `ZoinGalleryLayoutEngineTests` | 33 passed, 0 failed, 0 skipped, 0 blacklisted, 25ms |
| `ZoinGalleryDecodeSizePolicyTests` | 8 passed, 0 failed, 0 skipped, 0 blacklisted, 3ms |
| `ZoinGalleryThumbnailMemoryCacheTests` | 17 passed, 0 failed, 0 skipped, 0 blacklisted, 610ms |
| `ZoinGalleryPersistentDerivedImageCacheTests` | 39 passed, 1 failed, 1 skipped, 0 blacklisted, 3776ms |
| `ZoinGalleryViewerCacheTests` | 33 passed, 0 failed, 0 skipped, 0 blacklisted, 91ms |
| `ZoinGalleryComponentHostStagingTests` | 5 passed, 0 failed, 0 skipped, 0 blacklisted, 676ms |
| `ZoinGallerySceneItemMotionTests` | 11 passed, 0 failed, 0 skipped, 0 blacklisted, 1189ms |
| `F4GalleryBridgeTests` | 74 passed, 8 failed, 0 skipped, 0 blacklisted, 41692ms |
| `ZoinGallerySessionTests` | 187 passed, 2 failed, 2 skipped, 0 blacklisted, 77627ms |
| `ZoinGalleryMasonryLayoutModesTests` | 180-second timeout; incomplete suite |
| `ZoinGalleryQmlInteractionTests` | 45 passed, 19 failed, 1 skipped, 0 blacklisted, 87115ms |

## Unresolved failures

This is a mixed-DPI two-monitor desktop. Some interaction fixtures landed at
actual DPR 2.0 while asserting 1.75, despite other native fixtures running at
1.75. Those geometry failures are environmental/unresolved, not verified
product pixel-grid regressions. A rerun must bind each actual window to the
intended screen and verify its DPR; an application-level or primary-screen
DPR check alone is insufficient.

The integration suites contain functional/fixture expectations as well as
timing assertions. These runs were validation, not isolated cadence captures;
timing failures must not be treated as independent 160 Hz performance measurements.

### ZoinGalleryPersistentDerivedImageCacheTests

- `PersistentDerivedImageCacheTest::jpegWithoutOrientationRetainsMetadata() 'testImage(QSize(120, 80)).save(path, "JPEG")' returned FALSE. ()`

### F4GalleryBridgeTests

- `F4GalleryBridgeTests::panelIconsKeepNativeRasterAtMixedDpi() 'checked >= 6' returned FALSE. ()`
- `F4GalleryBridgeTests::nativeFileIconsDoNotKeepFallbackUnderlay() 'native' returned FALSE. ()`
- `F4GalleryBridgeTests::galleryIconsFollowSharedIconSet() Compared values are not the same`
- `F4GalleryBridgeTests::hostRuntimeDefersBoundedDecodeWorkers() Compared values are not the same`
- `F4GalleryBridgeTests::deferredCatalogApplyStaysWithinKeyboardFrame() 'elapsedNs < 20'000'000' returned FALSE. (447-row deferred catalog took 22.830 ms)`
- `F4GalleryBridgeTests::galleryKeepsAuthoritativeCursorVisible() 'layout->property("contentY").toDouble() > 0.0' returned FALSE. ()`
- `F4GalleryBridgeTests::sparseFinalWaitsForProvisionalPreviewFrame() Compared values are not the same`
- `F4GalleryBridgeTests::navigateParentReentryKeepsCursor() Compared doubles are not the same (fuzzy compare)`

### ZoinGallerySessionTests

- `GallerySessionTest::scrollingPromotesAnAlreadyPendingBackgroundProbe() Compared values are not the same`
- `GallerySessionTest::recycledPreviewNeverLoadsVectorAsBitmap(masonry) Compared values are not the same`

### ZoinGalleryMasonryLayoutModesTests

- `MasonryLayoutModesTest::pixelGridScopeDefersDirtyLeavesDuringMotionAt175Percent(explicit-scope) '!output.isEmpty()' returned FALSE. ()`
- `MasonryLayoutModesTest::pixelGridScopeDefersDirtyLeavesDuringMotionAt175Percent(attached-scope) '!output.isEmpty()' returned FALSE. ()`
- `MasonryLayoutModesTest::groupedOverflowAndDisabling(grouped-overflow) 'count' returned FALSE. ()`
- `MasonryLayoutModesTest::gridPageNavigationPreservesFractionalRowPhase() 'qAbs(alignedDownGeometry.y() - scrollAnimation->property("to").toReal()) < 1e-6' returned FALSE. ()`
- `MasonryLayoutModesTest::masonryThumbnailsConvergeAfterCatalogReorder() 'missingThumbnails().isEmpty()' returned FALSE. (row=3 model=image://zoingallery-thumbnails/zg-thumb-convergence-right-6cafece73e54026f-v1793969283292000000-s1453-236x177-11 visual= size=512x384`

### ZoinGalleryQmlInteractionTests

- `GalleryQmlInteractionTest::videoControlsAutoHide(hdr-shared-surface) Compared doubles are not the same (fuzzy compare)`
- `GalleryQmlInteractionTest::hdrControlsAreManualAndAligned(hdr-video-hdr-display) Compared doubles are not the same (fuzzy compare)`
- `GalleryQmlInteractionTest::hdrControlsAreManualAndAligned(hdr-video-sdr-display) Compared doubles are not the same (fuzzy compare)`
- `GalleryQmlInteractionTest::hdrControlsAreManualAndAligned(sdr-video-hdr-display) Compared doubles are not the same (fuzzy compare)`
- `GalleryQmlInteractionTest::detailsZoom(zoom-combined) Compared doubles are not the same (fuzzy compare)`
- `GalleryQmlInteractionTest::detailsZoom(zoom-separated) Compared doubles are not the same (fuzzy compare)`
- `GalleryQmlInteractionTest::detailsZoom(pixel-grid-combined) Compared doubles are not the same (fuzzy compare)`
- `GalleryQmlInteractionTest::detailsZoom(pixel-grid-separated) Compared doubles are not the same (fuzzy compare)`
- `GalleryQmlInteractionTest::detailsZoom(padded-pixel-grid-combined) Compared doubles are not the same (fuzzy compare)`
- `GalleryQmlInteractionTest::detailsZoom(padded-pixel-grid-separated) Compared doubles are not the same (fuzzy compare)`
- `GalleryQmlInteractionTest::viewerFlightKeepsInnerViewportStable(landscape) 'qAbs(origin.x() * dpr - qRound(origin.x() * dpr)) < 0.001' returned FALSE. (galleryViewerImageShader physical origin=(10.285714,1.142857), DPR=2)`
- `GalleryQmlInteractionTest::viewerFlightKeepsInnerViewportStable(portrait-wide-tile) 'qAbs(origin.y() * dpr - qRound(origin.y() * dpr)) < 0.001' returned FALSE. (galleryViewerImageShader physical origin=(360.000000,1.142857), DPR=2)`
- `GalleryQmlInteractionTest::viewerFlightKeepsInnerViewportStable(quarter-turn) 'qAbs(origin.y() * dpr - qRound(origin.y() * dpr)) < 0.001' returned FALSE. (galleryViewerImageShader physical origin=(920.000000,1.142857), DPR=2)`
- `GalleryQmlInteractionTest::viewerFlightKeepsInnerViewportStable(odd-source) 'qAbs(origin.x() * dpr - qRound(origin.x() * dpr)) < 0.001' returned FALSE. (galleryViewerImageShader physical origin=(9.142857,0.000000), DPR=2)`
- `GalleryQmlInteractionTest::panelPointerSelectionRevealsPartiallyVisibleTile() 'qAbs(layout->property("contentY").toReal() - expectedBelow) < 1.0' returned FALSE. ()`
- `GalleryQmlInteractionTest::videoThumbnailShowsCenteredPlayBadgeAt175Percent(landscape) Compared doubles are not the same (fuzzy compare)`
- `GalleryQmlInteractionTest::videoThumbnailShowsCenteredPlayBadgeAt175Percent(portrait) Compared doubles are not the same (fuzzy compare)`
- `GalleryQmlInteractionTest::videoThumbnailShowsCenteredPlayBadgeAt175Percent(square) Compared doubles are not the same (fuzzy compare)`
- `GalleryQmlInteractionTest::panelExposesTransitionTileAndSuppressesOnlyItsImage() Compared values are not the same`

The persistent-cache JPEG metadata test could not create its JPEG fixture with
the current static Qt image writer; that does not validate the metadata behavior.
The timed-out Masonry run is incomplete and must not be described as a pass.

## Build and packaging scope

The cache test initially failed to link the video-enabled Core graph because
its QML video-frame implementation was absent from the test link. Its CMake
link now matches the other video-enabled Core tests, and the rebuilt cache suite passes.

The main application was already rebuilt and exercised during the editor work.
This pass did not replace it or claim a new end-to-end performance capture.
Portable import/subsystem/DLL/extraction checks from that work are recorded in
[the editor report](editor-drag-selection-2026-11-06.md). No executable, build tree,
raw trace, generated screenshot or embedded payload belongs to this checkpoint.

See [the maintenance guide](../../docs/PANEL_RENDERING_PERFORMANCE.md) for the
performance conclusions and limits of historical evidence.

AI assistance has been used to create this output.
