# Visible file sizes stopped at the cursor fallback window

**Date:** 2026-11-06
**Files:** qt/host/src/PanelCatalogModel.h, PanelCatalogModel.cpp, F4GalleryBridgeProtocol.cpp; qt/host/tests/F4GalleryBridgeTests.cpp; docs/FILE_FIELDS.md
**Severity:** medium

## Problem

Documents/f4 showed sizes for the first eight files, while later visible files
remained blank indefinitely. Native UI inspection reproduced the missing values
on both panels, including after scrolling.

## Root Cause

The metadata planner stored the actual viewport and a synthetic cursor window in
the same fields. An asynchronous filesystem metadata revision reset those fields
to the 32-row cursor fallback. Cursor prioritization could overwrite them again.
The layout did not change, so QML had no new visibleIndexesChanged signal to
restore the actual painted range. Twenty-four directory rows plus eight file
rows exhausted the fallback, leaving remaining visible file sizes unresolved.

## Solution

Retain the last reported viewport independently, qualified by path and catalog
revision. Restore it on metadata refresh and keep cursor requests from replacing
it. Cursor urgency remains independent of viewport coverage. Record reported
ranges even after metadata completion, for future metadata-only revisions.
Opt-in navigation tracing logs retained ranges with [FIX:metadata-viewport].
No filesystem work or synchronous IPC is added to directory entry. The existing
first-frame gate, eight-row requests, frame pacing, input idle pause and sparse
offscreen policy remain in force.

## Verification

The regression failed first: only four eight-row chunks were requested instead
of six for a 48-row viewport. It now passes for state patches, complete scenes
and cursor prioritization. Related metadata scheduling/size tests pass, as do
ExtUiProtocolTest and PanelCatalogModelTest. Live rebuilt Qt displays previously
missing file sizes in Documents/f4, including after scrolling to the last rows.
Forty native benchmark transitions completed: enter mean 97.689 ms, leave mean
93.956 ms (95.823 ms overall), within the previously observed ~95 ms range.
Raw data: .diagnostics/size-navigation/metadata-viewport-1-measured/.
The broader architecture check reports pre-existing line-limit violations in
DocumentSurface.qml, GenericDialog.qml, SemanticMenuPopup.qml and
SemanticWidgetDelegate.qml; none was edited by this fix.

## Prevention

Do not reuse authoritative viewport coverage for synthetic cursor prefetch.
Exercise metadata refreshes without layout changes and viewports larger than the
fallback. Preserve bounded, input-aware asynchronous scheduling. A further useful
case is switching between cached workspaces with distinct viewport positions.

## Tags

`#qt` `#metadata` `#viewport` `#async` `#directory-navigation`
