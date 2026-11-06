# Refresh contextual key captions after native panel clicks

**Date:** 2026-10-22
**Files:** internal/panel/qt_semantic.go, internal/app/keybar_folder_labels_test.go
**Severity:** medium

## Problem

Clicking a file in the inactive panel retained F-bar captions from the folder
in the previously active panel. Qt's image-specific View caption could then
appear beside the stale Attr caption.

## Root Cause

The direct panel activation patch updates focus and command prompt without a
key-bar payload and suppresses the following semantic export. Cursor changes
occur after activation, so the patch cannot prove that context-dependent
captions and action enablement remain valid.

## Solution

Snapshot the bounded key set before native activation/cursor/pointer actions
and compare it after the entire action. Changed labels, icons or enabled
states invalidate scene-export suppression. Unchanged key sets keep the
existing compact path; no directory reload or catalog mutation is needed.
Diagnostics use the existing VTUI_DEBUG path with [FIX:panel-keybar].

## Prevention

Regression coverage exercises folder-to-file activation, unchanged file-to-file
activation, a target panel whose old cursor is on its parent row, and mouse-down
events. Tests assert the clicked file receives focus and changed chrome requests
an authoritative export. Do not prove a focus projection before accounting for
all cursor-dependent chrome changed by the same input action.

## Tags

`#qt` `#keybar` `#focus` `#semantic-fast-path`
