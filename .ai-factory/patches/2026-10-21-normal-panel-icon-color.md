# Preserve ordinary panel icon appearance under the cursor

**Date:** 2026-10-21
**Severity:** low
**Files:** third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml, GalleryEntryPreview.qml, tests/MasonryLayoutModesTest.cpp; qt/host/cmake/GroupIconResources.cmake

## Problem

Moving the cursor onto a file or folder replaced its icon's group colour with
the cursor foreground. Graphical panel icons also had a blanket opacity of 0.78.

## Root Cause

Icon tint reused the label's four-state foreground resolver, including cursor
colour. The fallback loader independently reduced opacity outside compact views.

## Solution

Use the group's normal foreground for ordinary icon tint and remove the loader
opacity reduction. Keep persistent marking and hidden-entry dimming unchanged.
Labels and selection surfaces retain their existing state-dependent styling.

## Verification

A five-mode regression covers folders and files, cursor movement and loader
opacity at 175% scale. All five cases failed before the fix because the cursor
colour replaced the normal colour. Related compact-view tests are updated to
assert the new normal-colour contract rather than the old cursor tint.

The test target initially could not build because Qt's generated alias resource
command belonged to a different CMake directory. A same-directory generation
target now supplies the cross-directory dependency, restoring normal Ninja builds.

## Prevention

Resolve icon tint separately from label cursor styling. Cover the effective
loader opacity as well as the leaf image and test all panel presentation modes.

## Tags

`#qml` `#icons` `#cursor` `#opacity` `#cmake`
