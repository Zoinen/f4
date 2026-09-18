# Final drag position lost on mouse release

**Date:** 2026-09-18
**Files:** internal/panel/list.go, internal/panel/file_panel_test.go, third_party/ZoinGallery/qml/GalleryPointerLayer.qml, third_party/ZoinGallery/tests/MasonryLayoutModesTest.cpp
**Severity:** medium

## Problem
After a downward selection swipe, the cursor could remain one row behind the release position.

## Root Cause
Both console and Gallery ended the drag without processing the coordinates carried by release. When no separate move event arrived for the last position, they committed the previous row. QTest mouseRelease synthesized movement and initially masked this: a directly delivered MouseButtonRelease reproduces the real sequence.

## Solution
Apply the final position before clearing the drag state. Gallery reuses its existing drag handler and sparse selection transaction; Go retains the original button and selection mode until the final row is processed. Canceled Qt gestures still only end the gesture. Go logs final cursor coordinates through DebugLog under [FIX:drag-release].

## Verification
Before: Go cursor 1 instead of 2; Qt cursor 2 instead of 3.
After: the same tests pass. Qt covers left/right buttons, upward/downward movement, live/deferred updates, existing drag and held Insert. Full Go panel tests pass. Static QML and portable Windows checks pass.

## Prevention
Always test release carrying a newer position than the last move, without a test helper synthesizing an intervening move. Preserve the distinction between release and cancellation. Additional useful coverage: outside-panel release and drag auto-scroll boundaries.

## Tags
`#mouse` `#selection` `#event-order` `#qt` `#console`
