# Grid folder captions must share file-caption bounds

## Problem
Folder preview captions were narrower than file captions in grid presentation.

## Root Cause
The folder content shell already applies a horizontal margin. Its title applied
another margin inside that shell, unintentionally doubling the text inset.

## Solution
In grid mode only, compensate the shell origin and use the same four-pixel
tile-relative inset and snapped tile width minus eight as ordinary file labels.
Keep scene-space pixel alignment and other modes unchanged.

## Prevention
Compare caption width and tile-relative origins, not only whether text fits
inside its immediate parent. The DPR 1.75 folder fixture failed at 193.143 vs
201.143 px before the fix and passes afterward.

Tags: #qml #grid #folder-preview #padding
