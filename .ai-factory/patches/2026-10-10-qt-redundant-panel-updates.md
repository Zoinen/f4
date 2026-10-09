# Preserve unchanged native panel presentation

**Date:** 2026-10-10
**Files:** qt/host/qml/PanelsSurface.qml, qt/host/src/F4GalleryBridgePanelSync.cpp,
qt/host/tests/F4QuickViewSurfaceTests.cpp, qt/host/tests/F4GalleryPointerTests.cpp
**Severity:** medium

## Problem

Cached Android navigation spends substantially more time in Qt presentation
than in Go's cached directory restore. Shell/terminal updates republish both
panels, and catalog revisions with unchanged rows start layout transactions.

## Root cause

QML panel bindings depend on the whole shell publication and receive newly
allocated descriptor objects even when their values are unchanged. Clearing
compact overrides on a shell update repeats accepted catalog descriptors.
The bridge uses catalog/metadata revision changes as a layout transaction
trigger despite the native model already retaining equivalent rows.

## Solution

Keep independently observable left/right panel descriptors until their bounded,
row-free values change. Continue forwarding catalog revisions, cursor state,
readiness and metadata acknowledgements to the session, but start a presentation
transaction only when rows, source identity, paging, grouping or renderer
configuration differ. Preserve atomic transactions for real row/density changes.
Keyboard-repeat queue behavior is unchanged.

## Verification

Both new regressions fail before the fix and pass afterward. Related cache,
immediate Android switching, workspace retention, activation focus, embedded QML
imports and 175% text/icon geometry checks pass. Static DLL audit and generated
embedded-host extraction/reuse/concurrency tests pass. The running `f4 [zoin]`
host matches the newly built host's SHA256.

The original `/sdcard` loop encountered a separate backend non-directory result
on refresh, so the Qt comparison used the regular `sdcard/Android` directory.
For the first 20 successful opens of baseline trace 12 and fixed trace 13:
descriptor callbacks decrease from 70 to 30, scene-patch application time from
143.958 ms to 65.717 ms, and median sent-open/destination acknowledgement from
28.060 ms to 23.360 ms. Peaks remain around 98 ms; this change does not establish
an under-30-ms end-to-end guarantee or eliminate all remaining navigation delay.
Temporary profiling instrumentation was removed before packaging the running app.

## Prevention

Test observable panel update counts for terminal-only shell changes and repeated
compact descriptors. Test revision-only refreshes for preserved rows and zero
layout transactions, while verifying real catalog and renderer changes still
start transactions. Compare complete Qt/Go traces rather than path-assignment
time alone, and exclude reproductions interrupted by unrelated backend errors.

## Tags

`#qt` `#qml` `#performance` `#android` `#catalog` `#redundant-updates`
