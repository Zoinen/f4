# High gallery zoom jumped back after a gesture

**Date:** 2026-09-22
**Files:** `internal/panel/list.go`, `internal/panel/file_panel_test.go`
**Severity:** medium

## Problem

In Details and two/three-column layouts, increasing zoom above 72 could jump
back to a much smaller row size after the host committed the new value.

## Root Cause

The previous range extension updated the Qt host and ZoinGallery to accept
values through 216, but the Go panel's `GalleryDensityLimits` still capped
Columns and Details at 72. The Qt gesture preview therefore showed the larger
size until the Go semantic state returned 72 and replaced it.

## Solution

Raised the Go maximum for both modes to 216. Added debug logging when a
requested density is clamped, so any future cross-layer range mismatch has
an observable value and mode. A regression test first reproduced 200 becoming
72; after the change it verifies 200 is retained and 300 clamps to 216 in
both modes. A rebuilt app was live-tested at 210 and 216; 210 survived a
menu reopen and semantic round-trip.

## Prevention

When changing a cross-process range, search every validator and the persisted
state owner. Verify a value above the old limit across the complete request,
semantic response, and UI reapply path, not only the visual slider maximum.
Further useful coverage is an automated end-to-end gesture test that drives
the Qt host and real Go panel together.

## Tags

`#gallery` `#zoom` `#cross-process` `#semantic-state` `#regression`
