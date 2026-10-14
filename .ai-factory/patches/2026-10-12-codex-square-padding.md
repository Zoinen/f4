# Tight square viewports for colored file artwork

**Date:** 2026-10-12
**Severity:** low
**Files:** qt/host/icons/codex/*.svg, qt/host/tools/PrepareCodexIcons.cpp, qt/host/tests/F4IconProviderTests.cpp

## Problem
Codex glyphs retained transparent outer padding and appeared too small.

## Root Cause
The imported square canvases were larger than their visible paths. Simply
keeping a square canvas did not normalize its size or center.

## Solution
Prepare viewports offline from all nonzero-alpha artwork at high resolution.
Use the maximum bound dimension as the square side and center on both axes.
Change only viewBox; preserve path geometry, intrinsic square size, colors,
translucency and cutouts. No runtime scans or modifications to Lucide.

## Verification
The new rendered-resource regression failed before normalization: Astro occupied
352x448 pixels inside 512x512. Rerun it for every bundled Codex SVG after preparation.
Also test ordinary and parent-folder cutouts and existing icon-theme switching.

## Prevention
Validate visible alpha bounds as well as viewport squareness and centering.
Square does not imply a glyph should be stretched to fill both dimensions.
Future coverage can include additional zoom/DPR combinations.

## Tags
`#svg` `#icons` `#offline-assets` `#aspect-ratio`
