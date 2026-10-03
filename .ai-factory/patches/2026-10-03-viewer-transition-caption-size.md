# Gallery caption reflowed during the tile-to-viewer transition

**Date:** 2026-10-03
**Files:** qt/host/qml/GalleryViewerHost.qml, qt/host/tests/F4GalleryPointerTests.cpp
**Severity:** medium

## Problem

The text inside the source masonry tile appeared to disappear abruptly as its image expanded into the viewer.

## Root Cause

The transition already cloned and moved the source caption, but its container width interpolated from the tile width to the full image width. That changed the text's layout and elision during flight, breaking the visual continuity of the original label.

## Solution

Keep the cloned caption at the original width and height throughout opening and closing. Move its center toward the final image center while retaining the existing progress-based fade and vertical travel. Added intermediate-frame and reverse-transition size assertions. Optional `[FIX:viewer-transition]` logging now includes caption width.

## Prevention

For shared-element text transitions, animate position and opacity separately from text layout dimensions. Verify text bounds at an intermediate frame, not only at the endpoints.

## Tags

`#qml` `#transition` `#caption` `#text-layout` `#regression`
