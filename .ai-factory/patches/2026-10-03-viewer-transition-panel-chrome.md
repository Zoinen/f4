# Panel chrome vanished at the start of the gallery transition

**Date:** 2026-10-03
**Files:** qt/host/qml/F4HostWindow.qml, qt/host/qml/FilePanelView.qml, qt/host/qml/GalleryViewerHost.qml
**Severity:** medium

## Problem

During the slowed panel-to-viewer transition, the selected tile's blue fill, the panel splitter, and both panel status cards disappeared immediately instead of fading with the panels.

## Root Cause

The selected tile's entire selection surface was forced to opacity zero while the transition border was visible. The border overlay reproduced only its stroke, not its fill. Separately, the splitter and status cards used `viewerVisible` to set `visible` to false, bypassing the existing progress-driven opacity on the panel layer.

## Solution

Leave the selection surface and panel chrome visible during the transition. The existing `normalSurfaceOpacity` follows the viewer's transition progress; it now fades the fill, splitter, and status cards together. Splitter interaction remains disabled while the viewer is active. Added QML regression assertions for the source selection, divider, and status cards at intermediate progress. Optional `--debug-viewer-transition` logging records progress and source opacity.

## Prevention

For animated surface handoffs, distinguish visual presence from input activation. Prefer a shared transition-progress opacity over immediate `visible` changes, and test intermediate frames, not only endpoints.

## Tags

`#qml` `#transition` `#opacity` `#viewer` `#regression`
