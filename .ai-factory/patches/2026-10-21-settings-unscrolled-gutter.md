# Preserve balanced Settings gutters without a scrollbar

**Date:** 2026-10-21
**Severity:** low
**Files:** SettingsDialogBody.qml, SemanticGroupViewport.qml, F4OperationsQueueTests.cpp

## Problem

Short Settings sections filled the scrollbar's released space and came within
4 DIP of side help, compared with 24 DIP between the sidebar and content.

## Root Cause

The viewport reserved right-side spacing only while its scrollbar was visible.

## Solution

Supply a separate unscrolled right inset from the Settings layout. Together with
the help gap it equals the opposite visual gutter. Keep scrolling geometry
unchanged and leave other semantic viewports unaffected by default.

## Prevention

The resize regression switches a long page to a short page, verifies that the
scrollbar disappears, and compares actual visible left and right gutters. The
pre-fix check reported left=24, right=4 at 175% scale.

## Tags

`#qml` `#settings` `#scrollbar` `#layout`
