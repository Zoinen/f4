# Consistent native group padding and Settings gutters

**Date:** 2026-10-21
**Severity:** low
**Files:** SemanticWidgetDelegate.qml, SemanticDialogLayout.qml, SemanticGroupViewport.qml, SettingsDialogBody.qml, GenericDialog.qml, F4OperationsQueueTests.cpp

## Problem

Group surfaces had unequal vertical padding. The Settings scrollbar crowded
content, while side help and footer had unnecessarily large outer margins.

## Root Cause

Surface height inherited terminal row spans, but native edits and checkboxes
occupy different heights. The regression fixture measured 16 DIP above the
Language controls versus 8.29 DIP below at 175% scale.

## Solution

Measure visible native child bounds and place them inside 12 DIP vertical
padding. Keep the heading separate with a 4 DIP gap. Reserve 16 DIP to the left
of the content scrollbar and 4 DIP to its right before help. Set Settings right
and bottom window insets to 12 DIP, matching its top help inset.

## Prevention

Test mixed native controls, sparse semantic groups, resizing, checkable headings
and physical-pixel alignment. Assert actual child-to-surface distances rather
than only the semantic group's rectangle. Include geometry values in assertion
diagnostics; do not add per-frame layout logging.

## Tags

`#qml` `#settings` `#layout` `#padding` `#fractional-dpr`
