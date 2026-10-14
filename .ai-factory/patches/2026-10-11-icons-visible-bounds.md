# Fit Icons artwork rather than transparent canvas

## Root Cause

Giving the image leaf its full preview rectangle only fits the square icon
canvas. Lucide folder artwork occupies 246x211 pixels in a 280x280 raster;
transparent margins make the visible icon substantially smaller than its slot.
Rectangular provider requests instead distort SVG proportions.

## Solution

Icons mode requests a square raster at the larger slot dimension and opts into
`tight=1`. The asynchronous F4 image provider trims fully transparent border
pixels, preserving the artwork's aspect ratio. PreserveAspectFit then fills the
slot as far as its aspect allows, without stretching or clipping. Other modes
and ordinary icon requests remain unchanged. Optional F4_ICON_BOUNDS_TRACE
reports original and visible raster bounds. No additional cache is introduced.

## Prevention

Validate visible alpha bounds as well as leaf geometry. The new provider test
first failed at QRect(17,29 246x211) vs QRect(0,0 280x280), then passed for folder,
folder-up and file. Existing provider and fractional-DPR gallery layout tests
remain part of verification. Extend coverage to unusual native icon silhouettes
when changing native theme rendering.

Tags: #qt #qml #icons #aspect-ratio #transparent-padding
