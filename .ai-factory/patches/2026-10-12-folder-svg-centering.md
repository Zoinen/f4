# Center external test folder artwork

## Problem
The zipper-free folder SVG appeared shifted left.

## Root Cause
The original archive canvas spans x=0..16, but its folder artwork spans x=0..14. Removing the zipper did not normalize the canvas.

## Solution
Keep both folder variants on a square viewBox, `-1 0 16 16`, centered on the artwork's (7,8) center. Tight rectangular bounds distorted the test rendering; retaining the square canvas preserves the original proportions. No runtime cropping or layout adjustment is needed.

## Verification
The geometry check verifies both a square canvas and alignment with the artwork center (7,8). Both variants pass. Restart the test host to invalidate cached external SVGs.

## Prevention
Check visible artwork bounds against the SVG viewport when deriving new icons; also cover both ordinary and parent-folder variants.

## Tags
`#svg` `#icons` `#geometry`
