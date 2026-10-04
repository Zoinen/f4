# Auto-sized columns did not follow panel fonts

**Date:** 2026-09-28
**Severity:** medium
**Files:** semantic column schema, GalleryFileFieldPresentation, gallery header

## Problem

Changing panel font family or size left Details columns at their previous widths.

## Root Cause

The schema carried only relative widths. Qt had no distinction between default
and explicitly resized columns and distributed them independently of the font.

## Solution

Publish an optional autoWidth flag, measure automatic columns in the panel font,
and give the name column the remaining width. Share a reactive width array across
row delegates and headers. Preserve relative sizing for manual columns.

## Prevention

Test both family and size changes, linked interface fonts, and manual widths.
The regression logged the same 194.857 px before the fix when changing 12 to
24 px; after the fix the measured size column changes from 93.714 to 172 px.
Do not assume FontMetrics method calls establish a font binding dependency.

## Tags

`#qml` `#fonts` `#layout` `#semantic-protocol`
