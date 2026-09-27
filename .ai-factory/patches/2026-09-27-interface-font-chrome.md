# Interface font propagation to shell chrome

**Date:** 2026-09-27
**Severity:** medium
**Files:** Qt host shell chrome, ZoinGallery PathControl, F4QuickViewSurfaceTests.cpp

## Problem

Changing the interface font left breadcrumbs, tabs, dialog titles, sort buttons
and function-key labels in the previous font.

## Root Cause

Plain QML Text items do not inherit live font changes from ApplicationWindow.
Updating the application default is insufficient for these existing leaves.

## Solution

Bind the interface font family at the text leaves. Expose a breadcrumb family
property so every PathControl delegate shares the host's choice.
Tab labels also use the interface pixel size.

## Prevention

Verify the actual rendered Text font after changing the setting, not just the
ApplicationWindow font. The original sort-label regression failed with Sans Serif
instead of Courier New. Expanded coverage checks the chrome leaves and physical
pixel alignment at DPR 1.75. Test diagnostics use [FIX:interface-font].

## Tags

`#qml` `#fonts` `#bindings` `#regression`
