# Apply macOS precision scroll packets without animation restarts

**Date:** 2026-10-17
**Severity:** medium
**Files:** third_party/ZoinGallery/qml/GalleryViewportController.qml,
third_party/ZoinGallery/tests/GalleryPanelGestureTest.cpp

## Problem and Root Cause
In Low Power Mode, measured panel position updates were 50–67 ms apart despite
frame-swap intervals around 16.7 ms. AppKit delivery also slowed and batched, so
the trace does not establish one exclusive cause. Each macOS wheel delta
restarted a 15 ms NumberAnimation, needlessly postponing movement until an
animation tick and compounding sparse delivery. The wheel path also ignored
angle-only mouse events on macOS.

## Solution
Apply nonzero macOS pixel deltas synchronously through the existing clamped
offset/persistence path. Accumulate against the visible offset, cancelling an
unfinished discrete-wheel animation without jumping to its destination. Native
momentum arrives as further pixel deltas, so do not manufacture another easing
curve. Angle-only wheel input retains 150 ms easing; horizontal preference in
Columns and keyboard navigation are unchanged. Optional benchmark diagnostics
use [FIX:precise-trackpad-scroll].

## Regression and Prevention
Before the fix, precision-packet tests failed in all five presentation modes:
contentY stayed 100 instead of moving to 117.25. The same tests pass afterward,
covering fractional packet accumulation without event-loop ticks, offset
persistence, both clamps, reversal and takeover from an unfinished mouse-wheel
animation. The complete GalleryPanelGestureTests suite passes (14 tests).
The app is rebuilt and restarted with a separate post-fix scroll trace.

Keep input delivery and presentation timing separate. Never add/restart a
sub-frame animation on every high-resolution trackpad event. Additional useful
coverage is a physical-trackpad Low Power Mode comparison and discrete-wheel
testing on Windows/Linux; synthetic tests cannot establish perceived smoothness.

Tags: #macos #qt #qml #scroll #trackpad #low-power #regression
