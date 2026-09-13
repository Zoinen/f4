# Command line shifts when empty

**Date:** 2026-09-14
**Files:** qt/host/qml/CommandLineView.qml, qt/host/tests/F4QuickViewSurfaceTests.cpp
**Severity:** low

## Problem

Clearing the input moved its baseline by one physical pixel at DPR 1.75.
The regression measured scene Y 587.285714 empty versus 586.714286 populated.

## Root Cause

Qt reports a different document height for empty and populated TextEdit lines
(17 versus 18 logical pixels in the Monaco fixture). Centering from content
height changed padding; multiline mode also changed the enclosing bar height.

## Solution

Measure a populated line with the same TextEdit font and text engine, use its
height for stable padding and as the minimum document height. Keep real command
text and cursor offsets unchanged. FontMetrics alone does not reproduce the
document engine's rounding. Regression diagnostics log baseline comparisons.

## Prevention

Test populated-to-empty transitions in plain and rich text, including scene
origins, unit transforms and captures at DPR 1.75. Keep floating-point tolerance
when comparing a baseline difference to exactly one physical pixel.

## Tags

`#qml` `#text-metrics` `#pixel-grid` `#empty-state`
