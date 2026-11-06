# Preserve native icon detail before cropping

**Date:** 2026-10-18
**Files:** qt/host/src/F4IconProvider.cpp, qt/host/tests/F4IconProviderTests.cpp
**Severity:** medium

## Problem

Native file icons could look blurry even when the URL and texture dimensions
matched the delegate's physical pixel size.

## Root Cause

Bitmap-backed QIcon implementations downsample their padded canvas when
pixmap() receives the final display size. Cropping that smaller bitmap and
enlarging its visible artwork causes a second resample and loses fine detail.
The Windows shell engine often returns a full jumbo bitmap, but Qt's generic
file/folder icons also use bitmap-backed engines.

## Solution

For file routes, acquire the largest advertised source representation at DPR 1,
crop transparent margins at source resolution, and resize once to the physical
target. Preserve the existing logical-size/DPR path for engines without bitmap
sizes and for theme routes. Optional F4_NATIVE_ICON_DEBUG logging records
source, cropped, and target sizes.

## Prevention

The regression supplies padded 256px artwork with one-pixel detail and compares
the result to a single source crop/resize at 100% and 175%. Both cases failed
before the fix and passed afterward. Verify image content as well as output
dimensions: a large output image alone does not prove sufficient resolution.
The existing five-layout 175% scene-origin/transform and rendered fallback
checks also pass. Further useful coverage is a desktop fixture with a custom
multi-resolution folder icon across display-DPR changes.

## Tags

`#qt` `#native-icons` `#resolution` `#resampling` `#fractional-dpr`
