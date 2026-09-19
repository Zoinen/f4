# Preserve Go frame order across native menus and dialogs

**Date:** 2026-09-18
**Severity:** medium
**Files:** internal/nativeui/scene.go, internal/nativeui/scene_incremental.go, sdk/extui/model.go, qt/host/qml/ShellSceneStore.qml

## Problem

F4 in the F11 plugin menu opens its hotkey assignment dialog underneath the menu in Qt.

## Root Cause

Native scenes split the Go frame stack into separate menu and dialog streams. QML concatenated dialogs before menus, assuming menus always originate from dialogs. Menus can themselves open dialogs, so frame kind cannot determine stacking.

## Solution

Carry optional positive `stackOrder` through full and incremental typed projections and order QML overlays by this Go-owned position. Keep the previous fallback for peers without order metadata. No geometry or focus ownership changes are needed.

## Prevention

Test both menu-to-dialog and dialog-to-menu nesting, incremental publications, closure, pointer routing, and retained overlay identity. The Qt regression failed before the fix with `plugins-menu` as the top frame instead of `appearance-dialog`. Run the regression at DPR 1.75 and inspect the rendered capture.

## Tags

`#qml` `#semantic-protocol` `#stacking` `#dialogs` `#regression`
