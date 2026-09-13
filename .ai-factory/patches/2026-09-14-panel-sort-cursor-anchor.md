# Sorting changes the cursor's viewport offset

**Date:** 2026-09-14
**Files:** third_party/ZoinGallery/MasonryLayout.cpp, third_party/ZoinGallery/MasonryLayoutModelReset.cpp, third_party/ZoinGallery/src/embed/ExternalCatalogResetTransaction.cpp, third_party/ZoinGallery/qml/GalleryPanelReconciler.qml
**Severity:** medium

## Problem

Sorting retained cursor identity but moved its viewport position. At DPR 1.75,
an 80-pixel offset became 292 in details, 158 in icons, 1616 in masonry, and
314.286 in columns in the initial regression.

## Root Cause

The preserved anchor used the leading visible item rather than the cursor.
Fixed layouts did not consume the reset anchor, and QML could restore a stale
saved offset afterward. Incremental sorting emitted individual row moves whose
intermediate positions hit scroll boundaries and irreversibly clamped offsets.

## Solution

Anchor to the visible cursor using the presentation's scroll axis, restore the
anchor in the replacement geometry, and publish the committed scroll position.
Apply same-identity permutations as one layout change with persistent-index
remapping, avoiding intermediate layout/clamping states and repeated rewraps.

## Prevention

Live follow-up exposed the host-specific reread path: sorting Photos published
a provisional 48-row preview for a 299-row catalog before the final order.
Replacing the complete model with that preview discarded image geometry and
cursor identity outside the page. The bridge now retains a completed catalog
during provisional same-folder refreshes and loading sparse previews of a
previously complete catalog. Live flags showed `loading=true`,
`catalogProvisional=false`, `catalogRowsDeferred=true`, 48/299 rows: checking
only the provisional flag was insufficient. `sameFolderRefreshKeepsGalleryObjects`
reproduced the loss (four rows became one) before the fix. The gallery reorder
test also covers sparse deltas inside the host's presentation transaction.

Live verification in `/Users/zoin/Photos`, cursor on
`IMG_20230924_134546-Enhanced-NR.jpg`: Name → Size → Name keeps the
tile's top at screenshot y=77; the broken path moved it to y=539 on Size.
Name and Size use different masonry columns, as expected from row regrouping.

Test immediate and settled offsets across details, icons, grid, masonry and
columns. Verify a sort emits one layout change, no row moves, and preserves
persistent indexes. Cursor anchoring remains subject to final scroll bounds;
sorting can still change the tile's position on the non-scrolling axis.

## Tags

`#sorting` `#viewport` `#stable-identity` `#model-transaction`
