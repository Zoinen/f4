# Preserve enabled MenuItem hover while clearing disabled rows

**Date:** 2026-10-20
**Severity:** medium
**Files:** qt/host/qml/F4Menu.qml; qt/host/tests/F4OperationsQueueTests.cpp

## Problem

After the disabled-hover fix, enabled entries no longer highlighted under the
pointer. Keyboard selection and clicking still worked.

## Root Cause

The passive HoverHandler occupied an Item covering the entire popup above its
MenuItems. Even blocking=false (and a disabled-only containmentMask) did not
preserve native MenuItem hover. The prior regression test initialized
currentIndex directly and therefore tested clearing but never actual hover
selection of an enabled entry.

## Solution

Create small guards only above disabled rows while the menu is visible. Enabled
rows have no guard over them and receive their normal Qt hover, including
submenu behaviour. Keep guards outside the ListView's content model; explicitly
track row geometry and scroll changes when mapping their rectangles. Keep the
existing [FIX:menu-hover] diagnostics for disabled-row clearing.

## Prevention

The strengthened test uses real pointer movement with QCursor synchronization:
hover Copy, hover disabled Undo, hover Paste, return to Undo, then use keyboard
navigation and activate Copy. It checks actual highlighted state in both
directions rather than injecting currentIndex. Allow Cocoa's asynchronous cursor
warp to settle before asserting stationary-pointer keyboard behaviour.

The test failed on Cocoa at the initial enabled hover before implementation.
Also useful: coverage of hover-triggered nested submenus and disabled rows in a
long scrolled menu, not just text context menus.

## Tags

`#qml` `#menu` `#hover` `#regression` `#test-quality`
