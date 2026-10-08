# Action-driven sort direction lost during panel refresh

**Date:** 2026-10-08
**Files:** `internal/panel/list.go`, `internal/panel/qt_panel_refresh.go`, `internal/panel/qt_panel_refresh_test.go`
**Severity:** medium

## Problem

After selecting newest-first time sorting, newly added files appeared at the end
of a live-refreshed panel, including the Qt masonry view.

## Root Cause

The completed same-directory refresh sorted a detached catalog through
`compareEntryOrder`. That comparator honored `SortReverse` but ignored
`sortDirectionSetByAction`, even though an explicit Time sort defaults to
newest-first when `SortReverse` is false. The action-driven state therefore
looked correct until refresh, then the detached catalog reverted to
oldest-first. Size sorting had the same mismatch.

## Solution

Make detached time/size sorting use the same action-driven primary direction
as the live sort while keeping name tie-breaks and reverse toggles consistent.
Add gated `[FIX:panel-sort-refresh]` diagnostics when an action-driven time or
size refresh changes the catalog order.

## Regression Test

`TestSameDirectoryRefreshKeepsActionDrivenDescendingSort` fails before the fix
for a newly inserted newest file and verifies both newest-first time and
largest-first size order after reconciliation.

## Verification

- Targeted panel sorting and refresh tests pass.
- `CGO_ENABLED=0 go build` succeeds.
- The full `internal/panel` suite still hits the existing macOS
  `TestEnterOnUnlistableDirectoryKeepsPanelInPlace` permission failure.

## Tags

`#panel` `#sorting` `#masonry` `#refresh` `#regression`
