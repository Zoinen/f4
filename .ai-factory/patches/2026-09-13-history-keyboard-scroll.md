# Avoid replaying console viewport offsets on keyboard selection

## Root Cause

Each selection update positioned the native list at the console top index before
containing the selected row. Different native and console row capacities made Up
move the viewport even while the newly selected row was already visible.
The DPR 1.75 regression measured contentY changing from 2140.29 to 2113.14 on Up.

## Solution

Selection updates only contain the selected row in the current native viewport.
Opening, model changes and explicit scroll-only updates still apply the top hint.
Pointer acknowledgements keep their existing no-scroll behavior.

## Prevention

Test repeated Up and Down near the end, as well as explicit scroll updates followed
by keyboard reveal and hover acknowledgements. Keep diagnostics in regression output.
Further coverage can exercise Page Up/Down with variable-height section headers.
