# Right-panel file icon raster diagnosis

Measured the current 6cc3300f1 F4 / 080367b gallery code at window DPR 1.75 using the production F4IconSet and F4IconProvider behind a recording wrapper. The subsequent fix removes the extra DPR factor from sourceSize; scene-coordinate alignment is unchanged.

## Results

The right Details panel icon scene origins are whole physical pixels at window widths 1400, 1401, 1537 and 2400: respectively (1253,196), (1254,196), (1373,196), (2128,196). The rendered extent is 32 by 32 physical pixels. Mapping local unit vectors through the full ancestor chain preserves identity. The C++ scrolling surface correctly compensates the fractional parent Y origin of 191.5 physical pixels by -0.5 physical pixels.

With the actual Lucide provider, the corresponding folder source URL has size=18, dpr=1.75, strokeWidth=2. Qt requests 56 by 56 pixels and the provider returns exactly 56 by 56 pixels. Thus the rendered icon must resample that raster to its 32 by 32 pixel quad. This is a raster-size problem even though the quad is aligned correctly.

## Cause

[GalleryEntryPreview.qml](../../third_party/ZoinGallery/qml/GalleryEntryPreview.qml) sets sourceSize to round(width * renderDpr). Qt subsequently applies the window DPR to sourceSize before calling the image provider: 18.285714 logical px -> sourceSize 32 -> provider request 56. The intended raster size is round(18 * 1.75) = 32, matching snapIconExtent().

The correction should be confined to the source-size contract: use the integer logical size expected by Qt, rather than multiplying it by DPR before Qt does so. Keep the existing panel-level scene-phase correction. No additional ancestor subscriptions, per-row rounding, timers, or scene traversals are needed.

## Coverage gap

The retained-view test inspects classes beginning QQuickImage or QQuickText; the icon is QQuickIconImage, so that test does not inspect it. Geometry-only coverage would still pass this bug. Tests must also compare the actual provider request/raster size against the displayed physical extent and inspect the rendered icon.

Diagnostic logs: artifacts/right-icon-semantic.txt. Diagnostic test: rightPanelFileIconsKeepNativeRasterAt175Percent in qt/host/tests/F4QuickViewSurfaceTests.cpp. Captures: /tmp/f4-right-icon-*-175.png. The recording wrapper and full ancestor logging are test-only; they add no runtime overhead to F4.

## Fix verification

The regression failed before the fix with actual raster 56x56 versus displayed 32x32. After changing sourceSize to rounded logical width/height, it passes at all four widths with matching 32x32 raster and whole physical-pixel scene origins. Local unit-vector transforms remain identity. The rendered window was inspected. Embedded-only QML imports, retained workspace checks, and the Windows system-DLL import audit pass. No new alignment bindings or ancestor subscriptions were introduced.

AI assistance has been used to create this output.
