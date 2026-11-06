# Editable working set instead of provenance classes

**Date:** 2026-10-19
**Severity:** medium
**Files:** internal/filegroups/store.go, document.go; internal/settings/file_groups*.go;
internal/theme/file_groups.go, styles/modern.ini; qt/host/qml/SemanticWidgetDelegate.qml

## Problem

Source labels and imported-versus-standard sections made an evolving standard
rule set appear to contain different classes of records. Deleting a standard
group merely disabled it, and persistence re-layered missing records over defaults.

## Root Cause

The original layered-override design exposed storage ancestry as editing policy.
An absent default record became a tombstone, and the preview reconstructed the
same inherited list. The old colour-style fallback could also resurrect all
highlight rules after the working set had intentionally been cleared.

## Solution

Use a complete Version-3 Mode=snapshot working set on Apply, retaining legacy
sparse readers and backups. All rows can be deleted; standard data remains only
the initial/reset source. Remove the Source column and ancestry dividers and add
confirmed Restore standard set, transactional with Apply/Cancel. Incorporate
the development-profile masks into canonical standard groups and its colours into
Modern, preserving other schemes. Keep explicit old palette identities when
renumbering shipped Highlight sections. Once file groups own colours, do not
fall back to deleted legacy palette rules. Debug logs report snapshot commits.

## Verification

Regression checks failed before implementation for deleted rows returning,
missing snapshot metadata, legacy colour fallback and an unlabeled Qt restore
button. The same checks pass after implementation. Tests cover independent
saved fields/order, an empty set, all six command slots, failed saves, exact
backups, external edits, draft-only reset and theme-descriptor preservation.
Live Apply saved 46 canonical groups without active legacy duplicates; the exact
previous profile was preserved separately before reset.

## Prevention

Keep persistence semantics aligned with the editor's promises: deletion is not
disable, reset is explicit, and legacy loaders must not repopulate an owned set.
Test Apply/reload against changed defaults, not just an unchanged factory list.
Useful additional coverage is opening a snapshot made by a newer shipped set
with an older binary; downgrade compatibility is not guaranteed by Version 3.

## Tags

`#file-groups` `#settings` `#defaults` `#snapshot` `#migration` `#regression`
