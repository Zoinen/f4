# AI virtual paths and breadcrumb presentation

## Problem and root cause

AIVFS decorated only PanelTitle; GetPath and path operations still emitted bare
tree paths. Stripping ai:// before checking absoluteness also resolved qualified
paths relative to the current directory. The mount supplied no semantic icon.

## Solution

Keep internal cwd/tree paths unchanged while qualifying public GetPath, Abs,
Join, Dir and panel titles. Resolve an explicit ai:// prefix from the tree root.
Publish the existing sparkles icon and label the first QML breadcrumb AI without
changing its canonical navigation/edit value.

## Verification and prevention

Before the fix, the regression returned / for the root and failed ai://out
navigation from /ctx. Cover qualified read/create/rename/delete, root return,
legacy tree paths, icon publication and complete breadcrumb leaf transforms at
DPR 1.75. Diagnostics remain in regression output. Further coverage can test
restoring AI URIs from a different provider through bookmarks.

## Tags

`#vfs` `#ai` `#breadcrumbs` `#regression-first`
