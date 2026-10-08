# Restore masonry cursor filename highlighting

**Date:** 2026-10-08
**Files:** `third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml`, `qt/host/tests/F4GalleryPointerTests.cpp`
**Severity:** low

## Problem

The current masonry tile had a blue border but its bottom filename retained
the ordinary dark background instead of the gallery cursor highlight.

## Root Cause

The caption resolved its cursor background from per-file semantic console
colors, whereas the gallery card used the GUI cursor palette. Empty semantic
backgrounds fell back to the normal caption surface even for the current tile.

## Solution

For the current visible cursor in masonry only, use the existing GUI card
cursor color for the caption. Preserve semantic styling in other layouts and
the normal caption when the panel cursor is hidden. No new palette or cache.

## Prevention

Test the actual caption background leaf, not just the card border. The new
regression test failed before the fix and passes after it, including cursor
movement, panel focus changes and live palette updates at DPR 1.75. Related
quick-search, menu-focus and transition-caption tests pass too.

## Tags

`#qml` `#masonry` `#selection` `#theme` `#regression`
