# Preserve native icons after thumbnail decode errors

**Date:** 2026-10-18
**Files:** third_party/ZoinGallery/qml/GalleryEntryPreview.qml, qt/host/tests/F4GalleryBridgeTests.cpp, qt/host/CMakeLists.txt, qt/host/README.md
**Severity:** medium

## Problem

With Native icons selected, files such as AAC and BAT could settle on a plain
green file silhouette rather than their Windows shell icon.

## Root Cause

The source-color icon renderer was selected only when the thumbnail URL was
empty. A failed thumbnail kept its URL and renderer, exposing the generic
placeholder tinted with the extension's highlight color. No native icon image
was created after the decode error.

## Solution

Keep the failed thumbnail renderer and its stable Error status. A separate,
normally inactive Loader instantiates the existing full-color icon component
after failure. Hide the generic fallback once that icon becomes ready. This
avoids resetting the thumbnail status and repeatedly retrying the failed URL.
The existing source-color component retains its physical-pixel alignment.
Opt-in benchmark tracing records `[FIX] preview.thumbnail-icon-fallback`.

## Verification

Before the fix, the failed-thumbnail regression could not find the native icon
leaf after Image.Error. After the fix it passes in Masonry, Columns, Details,
Grid and Icons, including the registered 175% CTest case. It checks the actual
native image leaf's scene origin and unit vectors, captures the rendered frame,
checks untinted pixels, verifies stable failure, and recycles to a valid image.
Focused native color/alpha tests, embedded QML import isolation, generated Go
payload extraction and the Windows static DLL audit also pass. The running
worktree application's extracted host hash matches the rebuilt host.

The broader bridge suite still fails its old Lucide-default expectation and
the 20 ms catalog timing limit; both were reproduced with the original QML.
The broader icon suite also reports an unavailable original Lucide resource.
These checks were not weakened or changed for this fix.

## Prevention

Treat thumbnail URL presence and successful thumbnail decoding as distinct
states. Do not replace an error-producing renderer based only on its status if
the replacement status can restart the same failing request. Cover failure and
recovery together. Additional useful coverage is a delayed native icon provider
and vector-error fallback during delegate recycling.

## Tags

`#qml` `#native-icons` `#thumbnail` `#fallback` `#pixel-grid` `#regression`
