# Wheel zoom lost its accumulated destination

**Date:** 2026-09-17
**Files:** third_party/ZoinGallery/qml/FlickableZoomable.qml; third_party/ZoinGallery/tests/ReusableViewerPrimitivesTest.cpp
**Severity:** medium

## Problem

Rapid wheel zoom appeared to consume only one step. Both embedded and standalone viewers forwarded zoom events through navigation completion first.

## Root Cause

With no active navigation, completion called finishWheelPan even when no wheel pan existed. It replaced zoomAnimation.to with the currently interpolated zoomScale. Repeated input therefore lost the previous destination. Duplicate pan-end timer notifications could do the same.

## Solution

Make idle pan completion a no-op. Active wheel pans still finish normally. Add optional debugWheelZoom diagnostics for input, current scale and destination.

## Prevention

Test forwarding as well as the zoom math, and advance animation frames between events. A same-frame-only test masked the failure. The regression failed before the fix (third positive step targeted about 1.84 instead of 2.30), then passed with direction reversal, duplicate completion and Alt discrete steps. Existing pan/inertia, smooth minification and fractional-DPR geometry checks also passed at 175%. Resource-only QML import and Windows portable dependency audit passed. No visual testing.

## Tags

`#qml` `#wheel` `#animation` `#idempotency` `#regression`
