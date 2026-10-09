# Publish Find File results from dialog-only workspaces

**Date:** 2026-10-09
**Files:** internal/plughost/extui.go, internal/plughost/extui_test.go, internal/app/find_file_semantic_test.go, docs/UX_GUIDELINES.md
**Severity:** medium

## Problem

Alt+F7 completed the search internally while Qt continued showing Searching.
Switching to text presentation and back forced the missing results update.

## Root Cause

Find File creates a transparent workspace containing only dialogs. Native
surface ownership recognized shells, documents and operation queues, but not
dialog-only workspaces. The renderer consequently armed its fallback reveal
barrier. Later snapshots waited for a full console cell frame; a subsequent
partial cell frame could replace that full frame before it was flushed.

## Solution

Recognize nonempty dialog-only app scenes as native when their controls contain
no fallback nodes. Preserve explicit text presentation and unsupported-control
fallback. This keeps progress and results on the semantic stream without
requesting or waiting for a console frame.

## Prevention

The transport regression failed before the fix because Searching was held for
a fallback cell frame, and passed after it. Coverage also checks dialog-only
text/fallback scenes and asynchronous search publication without user input.
An isolated live Alt+F7 protocol probe showed Find File and Searching before
the fix, and Find File, Searching and Search Results after it.

## Tags

`#qt` `#async` `#semantic-scene` `#search` `#fallback`
