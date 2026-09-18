# Keep environment profile identity controls in one row

**Date:** 2026-09-18
**Severity:** low
**Files:** qt/host/qml/EnvironmentProfilesBody.qml, qt/host/qml/GenericDialog.qml, qt/host/tests/F4OperationsQueueTests.cpp

## Problem

The responsive native layout stacked Name, its input and Enabled vertically.

## Root Cause

The new dedicated layout preserved expanding panes but lost the console's
horizontal grouping. Its resize regression did not assert that grouping.

## Solution

Allocate caption and checkbox widths from their text, with the input filling
the remainder. Reserve enough minimum dialog width for this row. Round text
extents upward in physical pixels: nearest rounding reduced a 35-logical-pixel
caption to 34.8571 at DPR 1.75 and caused elision.

## Prevention

The resize regression now checks same-row placement, ordering, input width and
untruncated caption in browse/edit modes. Before correction physical row centers
were 273.5, 332 and 399. Verify all visible text/image leaf origins and unit-vector
transforms at DPR 1.75, and inspect captures; geometry alone missed the elision.

## Tags

`#qml` `#layout` `#fractional-dpr` `#envman`
