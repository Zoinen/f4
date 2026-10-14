# Preserve full Icons artwork size when the raster becomes Ready

## Root Cause

Qt Controls IconImage caps painted dimensions at intrinsic icon dimensions.
Trimming the transparent provider canvas reduces those intrinsic dimensions.
Retained old pixels can appear correctly enlarged during loading, but the new
Ready IconImage settles smaller. The actual rendered fixture measured 157
painted physical pixels inside a 168-pixel-high slot, despite correct leaf bounds.

## Solution

Use a plain PreserveAspectFit Image for Icons-mode provider rasters, which are
already tinted by F4IconProvider. Keep IconImage for standalone SVG tinting and
other modes. Share the source URL, raster extent and Loader lifetime, preserving
same-entry retention and replacement isolation. No oversampling workaround or
additional cache is introduced.

## Verification and Prevention

The rendered regression first fails at 157 vs 168 physical pixels, then passes
at slot sizes 128x96, 384x288 and 512x384 with DPR 1.75. It checks artwork pixels
after Ready, actual leaf physical origins/extents and unit transforms. Register
it in the existing fractional-DPR resize CTest gate. Existing asynchronous
retention and entry-reassignment checks pass. Validate painted artwork after
completion, not merely the retained Loading frame or wrapper geometry.

Tags: #qt #qml #icons #intrinsic-size #async #fractional-dpr
