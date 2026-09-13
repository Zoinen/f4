# Match transient menu chrome to the covered path row

## Problem and root cause

The F9 overlay reused the title bar base color but omitted the path row's
translucent foreground and bottom separator. The DPR 1.75 rendered regression
measured #19202b for the menu background versus #222a37 for the path row.

## Solution

Render the same two theme-color layers and use the shared separator
color and physical-pixel width. Preserve overlay geometry and text placement.
Qt.tint differs from renderer composition by one channel value; preserve the
original layering instead of weakening the exact pixel comparison.
The regression logs [FIX:f9-path-chrome] sampled pixels and compares the actual
rendered path/menu backgrounds and bottom strokes, plus separator geometry.

## Prevention

Compare rendered chrome, not just dimensions, when replacing one surface with
another. Retain leaf alignment and transform checks at DPR 1.75. Further coverage
can exercise custom translucent themes and theme changes while F9 is open.

## Tags

`#qml` `#menu` `#theme` `#pixel-grid`
