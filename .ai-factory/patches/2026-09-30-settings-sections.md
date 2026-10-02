# Settings category focus and independent native pages

## Problem
Escape stopped closing settings after a category-list click.
GUI preferences and color editing occupied the same native page.

## Root Cause
The category pointer acquired keyboard focus, but only ThemeEditorContent
handled Escape; it was not an ancestor of that focused list.

## Solution
Handle bubbling Escape on SettingsDialogBody. Move GUI controls into
GuiSettingsPage and register Theme creator separately. Draft restoration
and color reset/restore now respect page ownership.

## Prevention
The settings regression clicks the category list and sends Escape (failed
before the fix). Keep control tests on their owning pages and test both
category focus and child focus, including popup-first Escape behavior.

## Tags
`#qml` `#focus` `#settings`
