# Resolve ComboBox rows through their text role

**Date:** 2026-09-27
**Files:** qt/host/qml/F4ComboBox.qml, qt/host/tests/F4QuickViewSurfaceTests.cpp
**Severity:** medium

## Problem
Expanded font family and size lists displayed QQmlDMListAccessorData instead
of their values, although the closed controls showed correct captions.

## Root Cause
The shared delegate stringified its model wrapper when no name/text role
existed. Primitive string lists expose data through Qt's model adapter;
stringifying that adapter prints the internal object type and address.

## Solution
Use ComboBox.textAt(index) for explicit textRole and primitive lists. Preserve
legacy name/text role handling. The regression opens all six font controls,
compares rendered labels with their public text values, and checks text leaf
origins and transforms at DPR 1.75. Test diagnostics use [FIX:combo-text].

## Prevention
Test expanded dropdown contents, not just displayText and selection changes.
Include both JS string arrays and QStringList models; retain object-role tests.

## Tags
`#qml` `#combobox` `#model-roles` `#regression`
