# Gallery resize and swipe viewport continuity

**Date:** 2026-09-14
**Files:** third_party/ZoinGallery/qml/FlickableZoomable.qml, third_party/ZoinGallery/qml/GalleryViewerCatalogController.qml
**Severity:** medium

## Problem

Fullscreen moved the visual center at arbitrary zoom. Swipe navigation briefly
displayed the incoming image at the outgoing image's offset after animation.

## Root Cause

Only Fit mode handled viewport size changes; custom zoom retained top-left
coordinates. Swipe commit restored its prepared viewport with a 5 ms timer
after the source handoff, leaving a renderable intermediate state.

## Solution

Resize custom viewports by half the viewport size delta, preserving zoom and
the center image point subject to image bounds. Keep the existing Fit path.
Apply a pending committed viewport synchronously when the target sources are
installed instead of deferring it to another event-loop iteration.

## Prevention

Regression-first tests at DPR 1.75 measured a center change from (720,510) to
(900,650), and a swipe handoff at (-123,-85) instead of its endpoint (0,-85).
Tests now check immediate geometry rather than waiting for eventual correction,
and cover Fit resizing across portrait and landscape aspect ratios.

## Tags

`#qml` `#gallery` `#viewport` `#async-handoff`
