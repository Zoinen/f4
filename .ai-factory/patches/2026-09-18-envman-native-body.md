# Fit Environment Profiles to its native dialog viewport

**Date:** 2026-09-18
**Severity:** medium
**Files:** plugins/envman/manager_semantic.go, plugins/envman/manager_ui.go, qt/host/qml/GenericDialog.qml, qt/host/qml/EnvironmentProfilesBody.qml

## Problem

The Qt Environment Manager kept an outer vertical scrollbar after resizing and its two panes ended at different heights.

## Root Cause

The generic dialog layout converted terminal row spans into fixed native pixel heights. Native headers, control metrics and the action footer consumed extra space while the two large controls retained independently calculated heights.

## Solution

Expose an owner-declared environmentProfiles layout with stable roles for the existing controls. Reuse their existing Qt delegates in a viewport-driven two-pane body, with a shared bottom edge and internal scrolling only. Preserve Go action identities and console geometry.

## Prevention

Test both browse and edit states, live shrink/enlarge, narrow width, and semantic geometry acknowledgments. Require no outer scroll range, matching pane bottoms and stable delegates. Verify the real text/raster leaves at DPR1.75 and inspect captures.

The baseline failed because `dialogBodyScrollBar` remained visible at 850×700
logical pixels (DPR1.75), before any narrow-window constraint was involved.
The narrow-width check also caught a missing snapped border inset in the
minimum-height calculation (1.142857 logical pixels at DPR1.75). The minimum
now uses the same inset as the body allocation.

## Tags

`#qml` `#responsive-layout` `#envman` `#resize`
