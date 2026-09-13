# Focused connection editors

**Date:** 2026-09-13
**Severity:** medium
**Files:** internal/settings/record_dialog.go, plugins/netfox/netfox.go, plugins/cloudfox/dialog.go, qt/host/qml/SettingsDialogBody.qml

## Problem
Add connection opened unrelated global settings; NetFox mouse activation did not open an editor.

## Root Cause
Plugins redirected creation to a category-level settings route. NetFox handled Enter but did not implement the semantic panel action used by the GUI.

## Solution
Expose a reusable record editor through an optional VFS host interface. Reuse the Settings Center controls, provider draft, validation and commit; hide navigation and record lists. Retain the complete provider draft to preserve sibling records, and refresh panels after successful save. Route NetFox semantic Add to this editor; use shared plus icon metadata for both plugins.

## Prevention
Test keyboard and semantic activation independently. Cover cancel, save, missing records and sibling preservation. Test record-only and full settings layouts at DPR 1.75, including native text/image leaves and effective transforms.

## Tags
`#semantic-ui` `#plugins` `#settings` `#draft-isolation`
