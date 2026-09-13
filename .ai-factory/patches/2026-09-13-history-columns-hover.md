# History hover and native column highlights

**Date:** 2026-09-13
**Severity:** medium
**Files:** internal/app/history_dialog.go, qt/host/qml/SemanticMenuPopup.qml, qt/host/qml/SemanticMenuItemDelegate.qml

## Problem
Hover acknowledgements repositioned history lists. Command history flattened date, directory and command into one label; search matches were only drawn by the console renderer.

## Root Cause
Native selection containment could not distinguish pointer acknowledgements from keyboard changes. Console custom paint data did not reach semantic menus.

## Solution
Keep contentY unchanged for pointer selection and its acknowledgement. Reuse semantic menu Details for history fields and rune-indexed match masks. Render command, directory and right-aligned date columns; dim metadata without dimming matches. Escape command text before styled rendering and retain raw execution identity.

## Prevention
Cover hover acknowledgements, keyboard containment, Unicode match masks, markup escaping and actual column leaves at DPR 1.75.

## Tags
`#history` `#hover` `#qml` `#unicode` `#semantic-ui`
