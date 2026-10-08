# Panel time and size sorting direction is preserved across restart

**Date:** 2026-10-08
**Files:** internal/panel/qt_semantic.go, internal/panel/workspace.go, internal/panel/semantic_model_test.go
**Severity:** medium

## Problem

After choosing newest-first in the GUI, reopening the app showed oldest-first.
The sort mode remained Time while the direction changed.

## Root Cause

The semantic GUI sort action changed the live panel but did not persist a
session. In addition, action-driven Time and Size sorting use the inverse
meaning for `SortReverse` from the legacy INI session comparator. Saving the
action value directly caused it to restore in the opposite direction.

## Solution

Persist the workspace after a semantic sort action. When capturing a session,
translate action-driven Time and Size directions into the legacy session
convention, preserving compatibility with existing session files and restores.

## Prevention

- Test both the sort action persistence hook and a serialized session round trip.
- Keep UI sort-direction semantics separate from the legacy session encoding.
- Verify the actual first entries after a restart, not only the sort indicator.

## Tags

`#panels` `#sorting` `#session` `#persistence`
