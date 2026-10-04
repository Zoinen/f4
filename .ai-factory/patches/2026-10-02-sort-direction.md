# Qt startup sort indicator direction

**Date:** 2026-10-02
**Files:** `internal/panel/qt_semantic.go`, `sdk/extui/model.go`, `internal/nativeui/scene.go`, `internal/nativeui/scene_incremental.go`, `qt/host/qml/FilePanelView.qml`, `qt/host/src/ExtUiScenePatchReducer.cpp`
**Severity:** medium

## Problem

The Qt sort arrow could point upward on startup while restored time sorting placed newest files first. Changing sort twice corrected the arrow.

## Root Cause

Qt inferred the direction from `sortReverse`. Restored time and size modes retain a legacy comparator whose base direction differs from the comparator used after a sort action. The same boolean therefore has opposite visible meanings across those states.

## Solution

The panel now publishes its effective `sortAscending` direction in full and compact semantic updates. Qt uses that value for the icon and tooltip. A targeted diagnostic logs when the old inference would have disagreed.

## Prevention

For presentation state with mode-dependent semantics, publish the effective value instead of reimplementing domain logic in the frontend. Check both restored and action-driven states in regression tests.

## Tags

`#qt` `#sorting` `#semantic-model` `#startup`
