# Column names and thumbnail spacing at high zoom

**Date:** 2026-09-22
**Files:** `third_party/ZoinGallery/qml/ColumnsEntryDelegate.qml`, `third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml`, `third_party/ZoinGallery/src/embed/ExternalCatalogModel.cpp`, `third_party/ZoinGallery/tests/GalleryQmlInteractionTest.cpp`
**Severity:** medium

## Problem

Two- and three-column views kept names on one line despite taller rows. The visible gap between portrait thumbnails and names grew with zoom in Columns and Details.

## Root Cause

Columns always used single-line middle elision. Compact previews occupied a square slot even for portrait images; aspect-fit centered the narrow thumbnail in that slot, leaving zoom-proportional blank space before the text.

## Solution

Columns now uses the same height-dependent line count, wrapping, and multiline right elision as Details, while preserving compact middle elision. The preview slot width uses catalog image dimensions for portrait images, so geometry is stable before asynchronous decoding. A regression test covers two and three columns, separate extensions, compact and maximum zoom, visible image-to-name spacing, and DPR 1.75. The test failed before the fix and passes afterward. The rebuilt Qt host was launched in f4 and visually checked with a real image folder.

## Prevention

When enlarging a gallery row, test both text layout and the *painted* image boundary, not just delegate geometry. Use metadata for aspect-ratio-driven layout so decoding cannot shift text. Additional useful coverage: automate live zoom gestures through the Go/Qt round trip and test rotated EXIF portrait dimensions.

## Tags

`#gallery` `#columns` `#details` `#zoom` `#qml` `#regression`
