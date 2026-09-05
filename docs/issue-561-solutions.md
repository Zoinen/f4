# Issue #561: settings dialogs in a short GUI window

## Candidate 1: change every settings dialog

Give each settings dialog its own compact layout or scrolling container. This
would duplicate viewport handling across roughly thirty dialogs, risks
inconsistent keyboard navigation, and would still leave plugins affected.

## Candidate 2: impose a global minimum window height

Reject resizes below the tallest dialog. It avoids clipping, but it makes the
application impossible to use in a small window even when no dialog is open
and does not satisfy the request for accessible dialog content.

## Candidate 3: make modal dialogs viewport-aware

Have the shared `vtui` window implementation resize a modal dialog to the
available viewport and provide a scrollable content area. This covers f4 and
plugin dialogs consistently, preserves normal layouts at usable sizes, and
keeps every action reachable. It is the preferred design, but must be
implemented and released in `vtui`, then consumed by f4.
