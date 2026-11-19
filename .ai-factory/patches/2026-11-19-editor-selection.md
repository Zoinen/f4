# Commit editor highlights in the same Qt frame

**Files** Qt document viewport, row delegate, scene store, and document surface tests
**Severity** medium

## Problem

Extending a word selection in the Qt editor could flash an occurrence color at the beginning of the row. Clearing a selection could briefly reveal the old occurrence color.

## Root cause

The window presenter updates the row model after Qt's normal polish. The delegate called `Row.forceLayout()` before the repeater finished changing its run widths and children. The first selection frame placed the occurrence at physical x=0 relative to the text inset instead of x=61.25 at 175% scale. The scene store also cleared the current compact caret update before the replacement styled rows became visible.

## Solution

Place each affected visible styled row once after the row transaction finishes. Tie compact caret updates to the native row publication so the visible rows keep their caret and selection until the next publication commits.

## Prevention

Check both the first render-boundary geometry and the rendered selection colors at a fractional scale. Retain the row pool and the compact cursor-only path during scroll and input regressions.

## Tags

`#qt` `#editor` `#selection` `#rendering`
