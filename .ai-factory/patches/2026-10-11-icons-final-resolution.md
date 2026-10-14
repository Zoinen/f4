# Render fitted icon vectors at final physical resolution

**Date:** 2026-10-11
**Files:** qt/host/src/F4IconProvider.cpp, qt/host/tests/F4IconProviderTests.cpp, qt/host/tests/F4GalleryPointerTests.cpp, third_party/ZoinGallery/qml/GalleryEntryPreview.qml
**Severity:** medium

## Problem

Large Icons-mode folder outlines had jagged edges after their raster arrived.

## Root Cause

The tight-artwork route cropped an already rendered square bitmap. QML then
enlarged that smaller cropped image to the rectangular slot with smooth sampling
disabled. Initial suspicion of missing DPR handling was disproved by tracing:
Qt already multiplies Image provider requests by the window DPR. Explicitly
multiplying sourceSize again would oversample and rescale unnecessarily.

## Solution

Request the entire rectangular logical slot, letting Qt apply DPR. Measure SVG
artwork bounds once per resource/stroke width, then render the vector directly
into the final physical slot with antialiasing, aspect preservation, horizontal
centering and bottom alignment. Do not crop the returned Lucide raster. Keep
other presentation modes' existing contracts intact. Bounds tracing is opt-in
with F4_ICON_BOUNDS_TRACE.

## Prevention

Verify actual provider requested sizes, not only QML sourceSize. Rendered tests
at DPR 1.75 check full-slot painted extents, physical pixel-grid alignment and
large zoom. Provider tests check exact physical output size and antialiased
edges; related async resize tests retain same-entry images without stale content.

## Tags

`#qml` `#svg` `#hidpi` `#rasterization` `#icons`
