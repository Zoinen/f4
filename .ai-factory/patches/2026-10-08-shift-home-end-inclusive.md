# Inclusive Shift+Home/End selection

## Problem

Shift+End from the middle did not select the final entry; Shift+Home had the
equivalent first-entry omission. The earlier boundary regression only started
with the cursor already at the endpoint and therefore missed the moving case.

## Root Cause

The shared Shift gesture paints the entries being left and excludes its target.
Stationary clamping was handled, but Home/End must include their destination on
the first press even when the cursor moves.

## Solution

Pass an explicit inclusive-selection-target flag for Home/End through the panel
to the selection controller. Other moving navigation retains its previous range
semantics. Keep benchmark-gated selection.edge logging for diagnosis.

## Prevention

Test edge commands both from the middle and while already clamped. Verify every
entry in the range and entries outside it, for add/remove intent in each layout.
The moving masonry Shift+End regression failed before the fix. The expanded
matrix covers masonry, grid, details, and columns at DPR 1.75.

## Tags

`#qml` `#selection` `#keyboard` `#regression-coverage`
