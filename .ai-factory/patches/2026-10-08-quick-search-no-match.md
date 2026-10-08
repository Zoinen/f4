# Quick search does not indicate missing matches in Qt

**Date:** 2026-10-08
**Severity:** low
**Files:** internal/panel/qt_semantic.go, sdk/extui/model.go,
internal/nativeui/scene.go, internal/nativeui/scene_incremental.go,
qt/host/src/ExtUiScenePatchReducer.cpp, qt/host/qml/FilePanelView.qml

## Problem and root cause

The search query always used the normal text color. The panel already knew
whether a search matched, but did not expose that state to Qt. The bounded
highlight map alone cannot reliably represent a whole-catalog no-match result.

## Solution

Expose fastFindNoMatch in every full, provisional and paged panel state,
serialize false explicitly for recovery, and carry it through typed projection
and validated incremental patches. Reuse the existing matcher result rather
than scanning files in QML or maintaining another cache. Tint the query and
cursor slightly red only for an active nonempty query with no match.

## Verification

Regression first: Go reported a missing field and QML retained its normal color.
Go covers no match, restored match, empty and closed queries, including paged
headers. Qt checks the actual query/cursor colors and recovery; existing search
leaf pixel-alignment checks run at DPR 1.75. The test diagnostic uses
[FIX:quick-search-no-match]; no per-keystroke production logging is added.

## Prevention

When adding transient panel state, cover every catalog presentation variant,
explicitly serialize its reset, and validate incremental transport acceptance.
Additional useful coverage is a match outside the retained highlight window.

## Tags

`#qml` `#quick-search` `#semantic-protocol` `#regression`
