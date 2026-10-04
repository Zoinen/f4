# Grid and icons captions vanished during the viewer transition

**Date:** 2026-10-03
**Files:** qt/host/qml/GalleryPanelHost.qml, qt/host/qml/GalleryViewerHost.qml, qt/host/tests/F4GalleryPointerTests.cpp
**Severity:** medium

## Problem

The image filename disappeared immediately when opening the viewer from Grid or Icons mode, despite the previous flying-caption implementation.

## Root Cause

`currentItemCaption()` only searched for `galleryMasonryLabel-*`. The viewer's overlay therefore had no source label in Grid or Icons mode and was never visible. The existing test supplied a synthetic masonry-style label, so it missed the actual panel mode.

## Solution

Select the real caption leaf for the active panel mode. In the viewer, carry either the masonry label surface or the Grid/Icons text leaf at its own fixed size, preserving the source alignment, wrapping, and elision while fading. A regression test now queries the live panel host in all three modes. The Grid case failed before the fix and passes after it. The running macOS application was rebuilt and visually checked with a Grid image.

## Prevention

Transition tests must use the actual source panel in each supported renderer mode rather than only a synthetic caption fixture.

## Tags

`#qml` `#transition` `#caption` `#grid` `#regression`
