# Clear menu hover on disabled rows

**Date:** 2026-10-20
**Severity:** low
**Files:** qt/host/qml/F4Menu.qml, TextEditContextMenu.qml,
ApplicationMenuPopup.qml, PanelRendererMenu.qml, PanelGroupByPopup.qml,
SemanticMenuPopup.qml, SemanticMenuItemDelegate.qml;
qt/host/cmake/F4HostQmlFiles.cmake; qt/host/tests/F4OperationsQueueTests.cpp

## Problem

Moving the pointer from an enabled menu entry to a disabled entry left the old
entry highlighted. Search-first navigation was also reported missing.

## Root Cause

Disabled Qt MenuItems and disabled MouseAreas receive no hover. The preceding
currentIndex or Go-owned selection therefore remained visible. Updating a menu
model can also acknowledge an older pointer selection after the pointer moves.

The original user profile explicitly stored NavigationMode=classic; it was not
an alternate profile. The runtime shortcut restored search mode and persisted it
without replacing the profile or copying unrelated settings.

## Solution

F4Menu observes disabled rows from guards outside the menu's
ListView and clears currentIndex. Keep guards above disabled rows only (the
follow-up enabled-hover regression ruled out a full-popup observer), non-blocking,
and track actual pointer movement so keyboard highlights are not cancelled by
synthetic hover updates. Parenting directly into the ListView registers an extra
menu row and breaks its content size. Parenting a PointerHandler directly during
popup construction hit a Qt setParentItem crash; use a normal Item as its owner.

Custom renderer/grouping menus also clear their local cursor on inactive rows.
Semantic menus suppress only the visual cursor without sending a disabled
selection to Go. Delayed pointer acknowledgements preserve suppression; new
keyboard selections restore it. Existing activation guards remain intact.

## Prevention

Regressions exercise enabled-to-disabled hover, keyboard recovery, text menu
activation, disabled semantic clicks and delayed Go acknowledgements at 175%.
Semantic pointer tests must move QCursor as well as inject the mouse event,
matching the existing stale-event guard. Further useful coverage includes
disabled nested submenus and menu model replacement during a captured drag.

## Tags

`#qml` `#menu` `#hover` `#disabled` `#pointer` `#settings`
