# Viewer transition selection fill and splitter stacking

**Date:** 2026-10-03
**Files:** qt/host/qml/GalleryViewerHost.qml, qt/host/qml/ShellSurfaceHost.qml, qt/host/tests/F4GalleryPointerTests.cpp, qt/host/tests/F4QuickViewSurfaceTests.cpp
**Severity:** medium

## Problem

The panel splitter painted across the expanding image. The selected tile's blue fill disappeared at transition start instead of traveling and fading with the image.

## Root Cause

The splitter had a higher scene `z` than the full viewer. The gallery panel deliberately exposes the cursor underlay during a viewer transition, making its selection surface transparent; fading that original surface cannot animate its fill. The transition host only carried a border and caption, not the fill.

## Solution

For the full viewer, place the splitter below the viewer while keeping its higher docked Quick View stacking. Add a transition fill behind the viewer image, sourced from the entry's cursor color and driven by the same interpolated rectangle, radius, and progress as the border. Added pre/post-fix assertions for stacking and for the moving fill at the start and midpoint of expansion. Optional `[FIX:viewer-transition]` logging includes the fill color.

## Prevention

Test intermediate animation frames and actual scene `z` ordering. When the source component intentionally suppresses its own chrome during a shared-element transition, carry every desired visual element in the transition layer rather than relying on source opacity.

## Tags

`#qml` `#transition` `#stacking` `#gallery` `#regression`
