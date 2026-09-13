# Refresh QWK traffic lights after native startup layout

## Root Cause

The system-button area is registered while the window is hidden. AppKit may
relayout standard buttons during initial presentation. QWK's area setter ignores
the same pointer, and its regular geometry notifications occur on later resizing.
The host tried to provoke that notification by setting zero window geometry and
restoring it over queued turns, racing native initial presentation.

## Solution

Replace the geometry workaround with a one-shot queued first-frame callback that
detaches and rebinds the existing area through QWK's public API. This reruns the
native layout hook on the GUI thread, after presentation. Guard both objects with
QPointer and scope the callback to the window lifetime. No timers, synthetic
resize events, native private APIs or per-frame polling are used.

## Verification and prevention

The native Cocoa regression observed three width changes before the fix and none
after. The resulting middle-button center is (48,18), matching the area. Check all
three AppKit button positions, visibility and physical geometry without a resize.
Further coverage can exercise full-screen startup and hide/show restoration.
