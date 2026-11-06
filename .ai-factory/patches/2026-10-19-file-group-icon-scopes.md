# Scope imported bundled artwork to its original catalog

**Date:** 2026-10-19
**Severity:** medium
**Files:** internal/filegroups/migrate_icons.go, store.go, migrate_icons_test.go;
qt/host/icons/filegroups.default.ini, tests/F4IconProviderTests.cpp; docs/FILE_GROUPS.md

## Problem

File groups showed Lucide icons while the editor's selected theme was Zoin.

## Root Cause

ImportLegacy duplicated each legacy Icon into Icon.*. Older profiles contained
bundled Lucide resource URLs. Universal imported groups precede thematic rules,
so their generic icon choice correctly won against the selected theme. Isolated
preview resolver tests did not exercise these migrated profile rules.

## Solution

Use frontend-declared LegacyIconPrefix catalog ownership to repair only the
duplicated pair in imported legacy.highlight_ groups, including version-3 files.
Keep colours, commands, feature toggles, IDs and order unchanged. Preserve
custom resources, newly authored groups and explicit generic overrides. Startup
never writes; Apply performs the existing atomic/conflict-checked publication
with a dedicated backup for repaired version-3 files. Optional VTUI_DEBUG logs
use [FIX:file-group-icon-scope].

## Verification

The legacy/v3 regression failed before the fix because wildcard icon choices
still shadowed other catalogs, then passed unchanged after the repair. Coverage
includes unchanged original files, exact backups, idempotent reload, custom
overrides and command preservation. Qt coverage previews Zoin while the live
theme is Lucide without changing the live theme.

## Prevention

Test migrated real-shaped profiles through the complete rule-resolution path,
not only the frontend resolver with manually authored theme-specific choices.
Further useful coverage is an end-to-end migration with a user-installed catalog.

## Tags

`#file-groups` `#icons` `#migration` `#theme-scope` `#regression`
