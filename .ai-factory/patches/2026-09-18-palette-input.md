# Shared edit input and palette scrolling

## Cause

The native read-only QML TextInput could consume modified navigation keys and
kept pointer selections local. Go's Edit lacked semantic selection and deleted
only a character for Ctrl+Backspace/Delete. Table had no relative scroll action.

## Fix

Forward native edit keys to the existing Go input sink and send pointer selection
in rune offsets. Implement selection and word deletion in the shared Go Edit.
Send accumulated wheel row deltas to Go Table, keeping query focus and preserving
bursts before the next scene acknowledgement. No text mutation is added to QML.

## Regression evidence

Before fixes, Qt tests failed for missing selection/scroll messages and Go tests
failed for single-character word deletion and unhandled selection/scroll actions.
After fixes, the focused palette, panel, Edit/Table/Scroll suites and Qt tests at
DPR 1.75 pass, including double-click selection, modified keys and wheel bursts.

## Lesson

Test both sides of semantic input: native focus and selection can intercept an
event before Go, and absolute state derived from an old scene loses rapid input.
