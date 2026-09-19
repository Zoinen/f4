# Preserve selection operators in search-first navigation

**Date:** 2026-09-18
**Files:** internal/panel/frame.go, internal/panel/selection_shortcuts_test.go, qt/host/tests/F4GalleryBridgeTests.cpp
**Severity:** medium

## Problem

After panel zoom moved to Alt, plain plus, minus and asterisk reached Go but started quick search instead of selecting files.

## Root Cause

PanelsFrame explicitly excluded search-first navigation from its existing character-based selection dispatch. Qt forwarding was already correct; FileSystemPanel consequently received and consumed the operators as printable search input.

## Solution

Permit selection dispatch in every navigation mode while the panel owns input. Preserve active quick-search text and explicitly focused command-line input. Add opt-in debug tracing and cover top-row and keypad events plus text-input ownership.

## Prevention

Test backend action dispatch after proving frontend forwarding. Checking only that an event crossed the Qt boundary does not establish that the intended Go action handles it.

## Tags

`#input-routing` `#selection` `#quick-search` `#go` `#qt`
