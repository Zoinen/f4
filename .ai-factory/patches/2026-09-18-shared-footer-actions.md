# Shared F-bar buttons for dialog footer actions

## Change

Environment Manager's footer was a passive legend. Extract the panel F-bar
button presentation so both surfaces use the same interaction and rendering
code, including separators, hover feedback and right-aligned shortcuts. Dialog
shortcuts keep their accent color. Buttons wrap when the dialog narrows.

## Ownership

Go publishes action IDs and availability with localized keyHints. Clicks route
through the existing keyboard handler, preserving confirmations, configuration
dialogs, persistence and focus behavior. Split the ambiguous Move Up/Down hint
into two explicit buttons. Reject stale actions from another editing mode.

## Regression

Go baseline lacked footer actions; tests now cover toggle, moving, edit,
save/cancel, empty-list availability and adding a profile. Typed protocol tests
include action IDs and disabled flags. QML tests cover actual footer clicks,
shared panel behavior and physical-pixel alignment at DPR 1.75.
The footer edge regression recorded a physical coordinate of 367.750000 before
snapping the inherited F-bar margins in the shared component and its containers.
