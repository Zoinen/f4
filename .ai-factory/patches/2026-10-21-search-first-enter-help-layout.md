# Route Search-first Enter by focus and stack side help content

**Date:** 2026-10-21
**Files:** internal/panel/frame.go, commandline_visibility_test.go; SettingsHelpCard.qml, SettingsDialogBody.qml, SemanticWidgetDelegate.qml, F4OperationsQueueTests.cpp

## Problem and cause

Enter submitted any retained command text, even when a Search-first panel owned
focus. The earlier implementation explicitly gave drafts priority over panel
activation. Separately, the narrow right-side help card still placed its icon
beside the text, reducing usable text width.

## Solution

Require command focus in Search-first with visible panels before submitting a
draft. Keep Classic and terminal-only command dispatch unchanged. An inactive
draft survives panel activation. Modified Enter continues through existing
configurable actions.

Pass a frontend-local stacked presentation flag to side help cards: the icon
has its own top row, with heading, description and timing below. Bottom cards
retain their horizontal layout. No theme, timing or control values change.

## Verification and prevention

The inactive-draft regression failed before the fix and now verifies both draft
retention and directory activation. Fixtures explicitly reload a directory after
changing its VFS path before selecting an entry. Additional checks preserve
focused Search-first and Classic submission. Qt resize checks assert stacked
side geometry and horizontal bottom geometry, including Cocoa at 175%.

Route input by its actual owner, not by the presence of draft text; test both
the positive command path and the negative inactive-draft path.
