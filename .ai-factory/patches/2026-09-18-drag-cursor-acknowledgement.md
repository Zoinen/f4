# Keep optimistic drag cursor separate from Go acknowledgements

**Date:** 2026-09-18
**Files:** qt/host/src/F4GalleryBridgePanelState.cpp, qt/host/tests/F4GalleryBridgeTests.cpp
**Severity:** medium

## Problem
A fast swipe selected the correct final rows but left the cursor at an earlier row. Processing final release coordinates fixed a separate edge case and did not resolve this delayed-reply race.

## Root Cause
The sparse state-patch handler substituted the pending local cursor into its context, then committed that same value as Go's authoritative cursor. Reconciliation interpreted this as an acknowledgement and cleared PendingCursor. The next delayed selection/scalar patch could therefore move the cursor backward. If this happened before gesture release, the final selection transaction captured the stale cursor even though its selection range was correct. Full panel synchronization already kept the applied and authoritative cursors separate.

## Solution
Use the same separation for state_update, selection_delta and selection_replace: apply the optimistic cursor to the Gallery session while retaining only Go's actual cursor in SideState. Only a genuine matching Go observation acknowledges the pending cursor. The existing opt-in navigation trace records masked patch indices. No wire protocol, timers, full scene delivery, or directory scans are added.

## Verification
- Regression before fix: correct selection, but left:two cursor became left:one after the second delayed patch.
- Real Qt widget replay with old acknowledgement behavior: all four cases fail, downward cursor 2 instead of 12, upward 11 instead of 1, in Columns and Details.
- After fix: 20 fast swipe replays at DPR 1.75 pass, including final cursor intent and visual cursor. Rendered capture inspected.
- Genuine acknowledgement releases the guard, and later Go cursor movement works; repeated stale patches do not reset the catalog or generate extra actions.
- Focused bridge/controller/reducer/status tests, static-QML smoke, Windows import audit, and embedded payload tests pass.

## Prevention
Test at least two delayed replies, not only one stale full scene. Put replies between the final pointer move and release, and assert both the selected range and outgoing final cursor identity. Optimistic UI state must never become its own acknowledgement.

## Tags
`#qt` `#selection` `#cursor` `#acknowledgement` `#race` `#incremental`
