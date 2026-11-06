# Preserve embedded Hotkey Configurator in Qt Settings

**Date:** 2026-10-20
**Severity:** medium
**Files:** internal/settings/center_semantic.go, center_semantic_test.go;
internal/nativeui/scene.go, settings_semantic_test.go; sdk/extui/model.go;
qt/host/qml/SettingsDialogBody.qml, SemanticWidgetDelegate.qml;
qt/host/tests/F4OperationsQueueTests.cpp; docs/SETTINGS_CENTER.md

## Problem

The Hotkey Configurator category was empty in Qt while the console displayed
its five-column table and assignment actions.

## Root Cause

Settings viewport projection rebuilt scalar controls from its form boxes even
when the viewport hosted an embedded full page. Such pages have no form boxes,
so their exported children were replaced with an empty list. After preserving
the children, ordinary semantic coordinates also placed the action row outside
the available Qt page. Optional metadata must additionally survive typed ExtUI
projection, not just direct JSON fixtures.

## Solution

Preserve full-page controls and the viewport identity, with optional `fullPage`
metadata in the typed protocol. Qt gives these table pages the full content area,
docks the original buttons below the table, and omits the irrelevant help pane.
Control IDs, handlers, terminal layout and transaction ownership are unchanged.
VTUI_DEBUG diagnostics use `[FIX:settings-full-page]`.

## Prevention

Regression checks first reproduced missing columns, lost typed metadata and
buttons below the page. Test rows and action dispatch in Go, metadata without
Legacy fallback, and table/button bounds plus native clicks in Qt. Also cover
actual configurable/native/plugin shortcut rows and assignment-dialog keyboard
capture when extending this editor.

## Tags

`#settings` `#hotkeys` `#semantic` `#extui` `#qml` `#regression`
