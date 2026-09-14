# Native traffic lights can relayout after the first Qt frame

## Problem and root cause

The one-shot frameSwapped hook was present in the binary, but it only corrected
the first presentation. Native AppKit button geometry can change later without
any QML area change or Qt window resize. QWK watches resize and the area's own
geometry; a one-shot refresh cannot enforce alignment afterward.

The native regression moved all three buttons after the first frame without
resizing the window. Both windowed and maximized cases failed: center y=26,
expected y=18. The earlier startup-only cases still passed.

## Solution

Observe public AppKit frame notifications for buttons and their titlebar
ancestors, plus activation, backing changes, and fullscreen exit. Coalesce work
onto the GUI event queue, compare native and desired centers, and invoke QWK's
public area setter only when they differ. Observe QML ancestor geometry too.
Leave fullscreen controls to AppKit. Retry initialization on frame notifications
until the native area exists, then disconnect that startup listener. Notification
tokens and pending work are scoped to the Qt window lifetime; no delay timer or
continuous frame polling is used.

## Verification

Native startup and late-layout cases pass for windowed/maximized windows with
QT_SCALE_FACTOR=1.75. All three native button frames are checked against their
actual backing pixel grid, without resizing the window to repair the layout.
The canonical live app logged native center (39,16), desired (50,21), then
confirmed the applied center (50,21), without a window resize.

## Prevention

Test geometry changes after the initial frame, not just first exposure. Add
coverage for fullscreen exit and late agent creation when changing startup again.

## Tags

`#macos` `#qwk` `#native-layout` `#lifecycle`
