# Restore embedded viewer Tab OSD

**Date:** 2026-10-08
**Files:** qt/host/qml/GalleryViewerHost.qml, qt/host/tests/F4GalleryPointerTests.cpp, third_party/ZoinGallery/qml/GalleryViewerOsd.qml, GalleryViewer.qml, GalleryViewerInput.qml, src/embed/GallerySession.cpp, include/ZoinGallery/GallerySession.h, CMakeLists.txt
**Severity:** medium

## Problem

Tab in the embedded image viewer did not display the metadata and filmstrip
available in standalone ZoinGallery.

## Root Cause

The shared viewer still toggled panelsVisible, but only standalone chrome
rendered that state. External sessions also returned null for imageModel.

## Solution

Add host-independent, lazily activated OSD with existing ImageFile::exifList
formatting and ImageModel filtering. Virtualized filmstrip delegates request
only their thumbnails via the existing catalog scheduler and shared caches.
Metadata arrivals do not reposition a manually scrolled filmstrip. Thumbnail
clicks use viewer navigation and restore viewer focus. Quick-view Tab keeps
its existing focus-source behavior. Optional --debug-viewer-osd logging records
visibility and current row.

## Prevention

Test actual rendered chrome, decoded thumbnail leaves, metadata, click navigation
and a second Tab, not just the viewer boolean. The regression failed before
implementation because the OSD was absent. Include fullscreen and virtual-device
catalogs in broader smoke coverage.

## Tags

`#qml` `#viewer` `#embedding` `#lazy-loading` `#regression`
