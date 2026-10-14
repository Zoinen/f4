# Refresh Zoin icon catalog and GUI default

**Date:** 2026-10-14
**Severity:** low

## Problem
The revised artwork removed specialized icons and the imported manifest. File
classification consequently fell back to generic icons, and rebuilding failed
because CMake still required the deleted manifest.

## Root Cause
The resource catalog and GUI default were independent of the updated asset set.
The old catalog mixed licensing provenance with classification and was removed
alongside obsolete artwork. Legacy Go metadata could override a GUI default.

## Solution
Keep a small Zoin-owned classification manifest referencing exactly the remaining
30 SVGs. Match exact configuration filenames before extension masks; use retained
generic icons for unsupported types. Bundle the user-supplied ICON_LICENSES.txt
without modifying it or the SVGs. Make Zoin the GUI-owned empty-preference default,
retaining saved Lucide/Native choices and the old codex alias. VTUI_DEBUG logs the
resolved startup theme with [FIX:zoin-default].

## Verification
Pre-fix build failed on missing manifest.json. Default regression failed with
Lucide instead of Zoin. Tests validate every manifest asset, coverage of the full
bundled SVG set, licensing resource, classification and native artwork bounds.

## Prevention
Keep classification metadata separate from provenance. Check catalog/resource
consistency whenever adding or removing icons. Additional useful coverage is a
rendered GUI restart test with absent and explicit saved icon preferences.

## Tags
`#qt` `#icons` `#resources` `#gui-default` `#regression`
