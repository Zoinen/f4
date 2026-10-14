# Keep Qt native image color spaces alive

**Date:** 2026-10-12
**Files:** ci/patch-qt-dependencies.py, ci/patch-qt-recipe.sh, ci/test_patch_qt_dependencies.py, qt/host/tests/MacApplicationIconTests.mm, qt/host/CMakeLists.txt, qt/host/README.md
**Severity:** high

## Problem

Opening Settings crashed the macOS Qt host. The supplied report shows
QCocoaCursor creating an NSImage through QImage::toCGImage, followed by a trap
in CoreGraphics' color-space validation and CFGetTypeID.

## Root Cause

Qt 6.11.1's qt_mac_cgImageFormatForImage returns a raw CGColorSpaceRef in a
vImage_CGImageFormat, but the local QCFType releases that reference before
returning. CoreGraphics caches color spaces, which makes ordinary conversion
tests pass until the cache no longer keeps the object alive. This is a
dependency ownership bug, not a Settings QML layout bug.

## Solution

The Conan source patch transfers one retained color-space reference through
the existing format structure. All three Qt callers (native-image conversion,
QMacCGContext, and CoreText) adopt it with QCFType and release it after creating
their native image/context. The private header documents the ownership.
The macOS recipe reuse guard rejects older binary recipes without the fix;
non-macOS cache reuse remains unchanged. Recipe diagnostics use
[FIX:mac-color-space].

For live verification, rebuilt QtGui from the existing Qt 6.11.1 source tree,
using the current macOS SDK. Copied the original dependency package into
.diagnostics/qt-color-space/package, replaced only its QtGui dylib, and pointed
the local build's generated Conan CMake data at this copy. The original cached
binary package was not modified. Re-running conan install regenerates that
local data: use the patched recipe and rebuilt package, not the old binaries.

## Regression and Verification

- Ordinary QImage conversion passed before the fix: the ICC cache hid the bug.
- The regression retains the returned format while generating 10,000 distinct
  ICC spaces to evict its cached color space. With GuardMalloc and old QtGui it
  crashed with SIGSEGV in CoreFoundation (exit 139).
- The exact same check passed with the rebuilt QtGui and GuardMalloc.
- CTest MacApplicationIconTest and MacImageColorSpaceTest: 2/2 passed.
- Python Qt dependency and portable Linux script tests: 15 passed.
- Shell syntax and changed-file whitespace checks passed.
- Restarted the actual Go-parented GUI; lsof confirmed the patched QtGui loaded.
  Opened and closed Settings twice and exercised input fields without a crash
  or saving settings changes. Left the application running on its panels.

## Prevention

Never return a borrowed native pointer whose RAII owner is local to a helper.
Keep allocator-assisted lifetime tests: system caches can mask use-after-free
in apparently successful functional checks. A useful additional integration
check is repeated Cocoa cursor changes while opening and resizing dialogs.

## Tags

`#macos` `#qt` `#use-after-free` `#coregraphics` `#conan` `#regression`
