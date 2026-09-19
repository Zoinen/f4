# Preserve captured pointer gestures through the profile preview

**Date:** 2026-09-18
**Severity:** medium
**Files:** plugins/envman/manager_frame.go, third_party/vtui/basewindow.go

## Problem

Shrinking Environment Profiles while holding the mouse repeatedly ignores moves. Releasing inside the preview can also leave the resize gesture active.

## Root Cause

The manager's read-only right-pane hit test ran before Window.ProcessMouse. A shrinking pointer moves inside the old window bounds, so the pane consumed the captured gesture. The guard also extended beyond the right border.

## Solution

Expose BaseWindow.IsMouseCaptured for window and child gestures. Skip preview interception and checkbox activation while capture is held, and constrain the preview guard to its actual horizontal interior. Existing window/child handlers continue to own movement and release. Debug logging uses VTUI_DEBUG.

## Prevention

Hit-test wrappers must preserve capture before considering current pointer position. The regression failed on the first held shrink move (121/62 remained instead of 119/61), then verifies five consecutive moves, release inside the preview, and unchanged profile state.

## Tags

`#mouse-capture` `#resize` `#envman` `#vtui`
