# Preserve antialiasing when fitting colored icon rasters

**Date:** 2026-10-12
**Severity:** low
**Files:** third_party/ZoinGallery/qml/GalleryEntryPreview.qml, qt/host/tests/F4GalleryPointerTests.cpp

## Problem
Rectangular Codex folder artwork developed visibly stepped contours.

## Root Cause
The colored icon Image used smooth:false. The loaded SVG raster and its final
aspect-fitted rectangle do not always map 1:1 onto physical pixels. Nearest-neighbor
sampling discarded interpolated antialiasing at those scales. Prior aspect-only
tests could pass without testing the sampling policy.

## Solution
Enable filtered sampling for colored icons only. Provider-rendered Lucide paths
retain their existing pixel-aligned sampling. Keep native aspect and high-resolution
SVG source requests; no runtime alpha cropping or changes to artwork.

## Verification
Regression initially failed because smooth was false. Log requested raster size,
DPR and physical painted extent using [FIX:codex-resolution] across all six modes
and two sizes. Rerun this check plus existing Lucide slot-fit coverage.

## Prevention
Aspect, texture resolution, sampling and physical placement are independent checks.
Additional coverage should compare framebuffer contours against direct SVG rendering
at fractional DPR and irregular slot sizes.

## Tags
`#qml` `#icons` `#antialiasing` `#resampling`
