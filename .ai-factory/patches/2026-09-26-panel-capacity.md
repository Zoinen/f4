# Preserve panel capacity during navigation refresh

## Problem
The Qt panel status overlay briefly lost its capacity row on each directory change.

## Root Cause
The semantic exporter required an exact directory match before exporting the existing capacity snapshot, although the replacement query is asynchronous.

## Solution
Retain capacity for the same VFS source until the new query completes. Keep symlink targets directory-specific and reject capacity from replacement sources. Debug logging records completed capacity refreshes.

## Prevention
Test the intermediate pending-query state during repeated parent/child navigation, not just final settled snapshots. Test source replacement independently.

## Verification
TestNativePanelStatusKeepsCapacityDuringNavigation failed before the fix and passed afterward, along with all TestNativePanelStatus tests.
