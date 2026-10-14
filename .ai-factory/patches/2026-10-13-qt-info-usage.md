# Native Ctrl+L capacity meters

**Date:** 2026-10-13
**Severity:** medium

## Problem
Console Ctrl+L rendered capacity bars; Qt rendered only Total and Free rows.

## Root Cause
The independent semantic projection retained the old scalar layout. Its typed
ExtUI row and legacy-to-scene adapter could not carry usage-meter data, and
InfoPanelView supported only ordinary, blank and section rows.

## Solution
Add a usage row containing localized Total/Used/Free text and used percentage.
Reuse the console percentage calculation for disk, RAM and swap. Preserve these
fields through the typed protocol and native adapter, rendering a native
ProgressBar with the panel progress palette. QML diagnostics use [FIX:info-usage].

## Regression
TestInfoPanelSemanticDiskUsage failed before implementation: disk usage meter
missing from semantic panel. The same test passes after implementation.
Scene projection coverage verifies every usage field. The rendered QML check
exercises 0%, 50% and 100% usage with nonzero visible geometry.

## Prevention
Presentation additions require typed-model, adapter and rendered-QML tests.
Further coverage should exercise live changing memory and narrow panel widths.

## Tags
`#qt` `#qml` `#extui` `#capacity` `#semantic-parity`
