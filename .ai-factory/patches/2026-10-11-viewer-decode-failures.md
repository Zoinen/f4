# Skip undecodable viewer candidates and refill the fit window

**Date:** 2026-10-11
**Files:** third_party/ZoinGallery/src/embed/ExternalCatalogViewer.cpp, ExternalCatalogDecode.cpp, ExternalCatalogPlanner.cpp, ExternalCatalogApply.cpp, GallerySession.cpp, qml/GalleryViewerCatalogController.qml
**Severity:** medium

## Problem

Viewer navigation stopped on recognized image extensions whose pixels could not
be decoded, including RAW files without a usable embedded preview. Such files
also consumed slots in the bounded fit prefetch window.

## Root Cause

Eligibility used the image extension rather than the outcome of metadata and
pixel decoding. Decode failures changed request state but neither excluded that
source revision from candidate scans nor widened the prefetch window.

## Solution

Track permanent failures by source identity and content revision. Refill the
window after failures, with one queued replan per batch. Navigation skips known
failures and continues in the same direction when a new candidate fails. If no
usable candidate remains, restore the last usable navigation origin. Direct open
retains error presentation. Preserve transport retries and usable fit/native
frames, and prune failure state when catalog identity or content changes.
Optional media timing diagnostics record `[FIX:viewer-decode]` exclusions.

## Prevention

Test both invalid metadata and valid dimensions followed by corrupt pixels.
Verify 16 usable fit frames, real session adjacency, changed-version recovery,
transient read retries, forward/back failure continuation, and boundary restore.
Keep navigation failure policy separate from filename-based panel eligibility.

## Tags

`#viewer` `#raw` `#prefetch` `#decode-failure` `#regression`
