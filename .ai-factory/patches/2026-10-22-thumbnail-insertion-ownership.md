# Transparent thumbnails after incremental catalog insertion

## Reproduction

A `rowsInserted` observer materializes the new image row and attaches an
already cached thumbnail synchronously. Reconciliation then overwrites the
live row with its pre-notification snapshot, losing the thumbnail ownership
while retaining the facade and its URL. Reattaching mistakes that URL for a
legacy publication and removes shared provider pixels. Subsequent QML loads
receive the provider's transparent placeholder rather than the image.

## Fix

Do not overwrite newly inserted rows after notifying observers. Remove obsolete
row-local provider cleanup; thumbnail and viewer caches alone own the pixels.
Original image files and persistent caches are unchanged.

## Coverage

- `ThumbnailMemoryCacheTest::thumbnailAttachedDuringRowInsertionRetainsCacheOwnership`
  fails before the fix because reattachment deletes provider pixels.
- `ViewerDecodeSourceTest::rgbPngThumbnailRemainsOpaque` checks opaque RGB PNGs.
- `MasonryLayoutModesTest::opaquePngThumbnailRendersPixels` checks real shader
  output with `ZG_NATIVE_RENDER=1`, including an opt-in `ZG_PNG_PROBE` source.
