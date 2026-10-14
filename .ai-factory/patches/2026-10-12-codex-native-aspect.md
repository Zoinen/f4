# Tight native-aspect Codex assets

**Date:** 2026-10-12
**Severity:** low
**Files:** qt/host/icons/codex/*.svg, qt/host/tools/PrepareCodexIcons.cpp, third_party/ZoinGallery/qml/GalleryEntryPreview.qml, qt/host/tests/F4IconProviderTests.cpp, qt/host/tests/F4GalleryPointerTests.cpp

## Problem
Square Codex canvases still left transparent margins in folder previews.

## Root Cause
Square normalization intentionally retained short-axis margins. Removing those
requires both rectangular viewBox and matching intrinsic SVG dimensions. Requesting
both raster dimensions in QML can distort direct SVGs before aspect-fit placement.

## Solution
Prepare rectangular alpha bounds offline, retaining all nonzero-alpha pixels.
Set matching SVG width/height. Request direct SVG raster width only, deriving
height from native aspect, then PreserveAspectFit in every panel presentation.
Existing provider-rendered Lucide/native aspect handling is unchanged.
The offline tool logs each prepared asset with [FIX:codex-padding].

## Verification
The tightened resource test failed before cropping (Astro 404x512 inside512x512).
After preparation, provider suite: 35 passed, 1 fractional-DPR skip.
Panel tests exercise all six presentation modes and two sizes with the rectangular
folder artwork, plus existing Lucide large-raster tests: 9 passed.

## Prevention
Keep viewBox and intrinsic aspect consistent, and test the loaded QML texture's
painted aspect, not only the source SVG. Additional coverage could include more
Codex silhouettes and fractional-DPR combinations.

## Tags
`#svg` `#icons` `#aspect-ratio` `#offline-assets`
