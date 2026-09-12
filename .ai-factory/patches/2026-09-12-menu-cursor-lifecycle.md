# Preserve panel and menu cursor presentation during menu transitions

**Date:** 2026-09-12
**Files:** qt/host/qml/FilePanelView.qml, qt/host/qml/GalleryPanelHost.qml, Qt host menu components and regression tests
**Severity:** medium

## Problem

Opening a command menu hid the active file-panel cursor. Closing a menu could
briefly repaint its stale first-row selection before Go removed the popup.

## Root Cause

Panel input eligibility also controlled cursor painting. Blocking input for a
menu therefore removed the selected-file outline even though the panel's logical
cursor did not change. Menu pointer-selection state could be cleared while the
old popup and its stale semantic selection were still visible.

## Solution

Separate panel cursor visibility from input ownership, permitting menu overlays
to retain the active panel outline while dialogs continue to suppress it.
Exercise menu dismissal with delayed popup removal, so local pointer state and
backend acknowledgement cannot transiently repaint an unrelated row.

## Prevention

- Treat input ownership and persistent selection presentation separately.
- Test actual command-menu publication plus real-gallery focus transitions at
  DPR 1.75, with a nonzero source cursor and no generated panel cursor intents.
- Test dismissal through the caption, backdrop and item activation while the
  backend still reports the old selected row, including no-hover selection.
- Keep diagnostic assertions in tests rather than adding per-frame logging.

## Tags

`#qml` `#focus` `#selection` `#menu` `#async` `#regression`
