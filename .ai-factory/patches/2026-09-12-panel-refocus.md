# Existing panel cursor must not imply keyboard ownership

## Problem and cause
Clicking a file row with command-line focus did not restore panel navigation.
Gallery empty-space clicks emitted activateRequested, but row presses only
changed the cursor. The host's activation handler therefore never ran for the
existing cursor row. Stub panel tests did not exercise this real gallery path.

## Solution
Emit activateRequested for valid row presses. The host already filters ordinary
active-panel clicks and forwards activation when command input owns navigation.

## Prevention
Exercise the real gallery component with commandLineOwnsNavigation enabled and
click its existing cursor row. The regression fails without the emission and
passes with it at 175% scaling; packaged-app testing also confirms grey-to-blue
cursor restoration without moving the row. Match the application's resource
import path in the test harness.
