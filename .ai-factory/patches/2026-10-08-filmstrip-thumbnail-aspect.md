# Filmstrip thumbnails stretched during decoding

**Date:** 2026-10-08
**Files:** third_party/ZoinGallery/src/embed/GallerySession.cpp, Decoders/QtDecoder.cpp, ThumbnailLoader.cpp, qt/host/tests/F4GalleryPointerTests.cpp

## Root cause

OSD requested the rectangular cell dimensions instead of an aspect-fitted
decode target. QtDecoder passed the target directly to QImageReader, which
stretched PNG pixels. QML PreserveAspectFit could not undo that distortion.
Thumbnail derivation also used IgnoreAspectRatio. Existing panel thumbnails
could mask the defect, explaining why only some entries were distorted.

## Fix

Fit OSD requests using the original image dimensions. Preserve source aspect
in QtDecoder when metadata is not yet available and when deriving smaller
thumbnails. Keep the existing shared catalog/cache pipeline.

## Verification

The rendered OSD regression failed before the change: a 700x900 portrait
decoded as 84x64. Afterward portrait and both landscape ratios pass at DPR
1.75, along with viewer gesture/navigation checks. Rebuilt and restarted the
macOS app and visually checked portrait filmstrip thumbnails in wizard_last_page.

## Prevention

Test decoded bitmap aspect, not merely QML fillMode or successful loading.
Include EXIF-rotated and metadata-late sources in future broader coverage.
