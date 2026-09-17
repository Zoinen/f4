# Resume all visible folder-preview cells after mode switches

## Problem
Icon to Grid retained only the previously decoded images. A multi-folder
regression reproduced four ready cells where the enlarged grid required nine.

## Root cause
Presentation changes temporarily hide the viewport and suspend preview reads.
The retained child layout can resize and request newly visible images while
suspended. If refreshed directory contents are unchanged, incremental catalog
application emits no delta to replay that lost demand.

## Solution
After installing the refreshed child catalog and its resource lease, resume
reads and publish image-readiness invalidation. The retained layout replans its
current visible cells without replacing cached images or decoding hidden cells.

## Validation and prevention
Count every ready cell across repeated Icon/Grid transitions for multiple
folders, including one-to-nine and four-to-nine expansion at DPR 1.75. A check
for just one usable preview cannot verify recovery of a larger grid.
