# Make console Help and Environment Profiles shrink from their opening size

**Date:** 2026-09-18
**Severity:** medium
**Files:** third_party/vtui/help_view.go, plugins/envman/manager_ui.go

## Problem

Held resize movements appeared ignored in Help and Environment Profiles while Settings resized normally. The earlier Environment Profiles regression enlarged the dialog first, masking the opening-size limit.

## Root Cause

BaseWindow defaults its minimum size to its initial geometry. These two callers never replaced that minimum with a usable compact size. Help also assigned console geometry without updating BaseWindow's cached size and processed links before an active window gesture.

## Solution

Declare usable content minima separately from opening dimensions. Keep Help's resize bookkeeping synchronized through SetPosition and route captured border gestures before link hit testing. Adapt Environment Profiles' right-side controls when shrinking.

## Prevention

Exercise shrinking immediately after opening through FrameManager dispatch and redraw, not only direct ProcessMouse after enlargement. Cover clamping, reversal while held, release, and child control bounds. Baselines: Help stayed77/22 instead of64/21; Environment Profiles stayed101/50 instead of100/48.

## Tags

`#console` `#resize` `#minimum-size` `#help` `#envman`
