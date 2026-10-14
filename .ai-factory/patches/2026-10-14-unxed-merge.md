# Preserve fork behavior when merging upstream main

## Integration
Merge unxed/f4 main at 32abfce58 into zoin. Resolve conflicts semantically:
retain fork editor snapshots and GUI palette lookups while adopting upstream
mapping eligibility, macro descriptions/dialogs, drive chooser ordering and
confirmation prompts. Merge the pinned vtui dependency update into its local
copy with the previous required version as the three-way base.

## Regression
An upstream assumption that every unmapped editor uses an async buffer does not
hold for the fork's small-file materialization. Require absence of a mapping,
then verify BOM preservation and saved bytes. PTY geometry must retain per-child
acknowledgement tracking and hidden-workspace deferral, while sizing an active
grid before shell startup and erasing stale ConPTY layouts only on actual resize.
Record startup acknowledgement on the UI thread to avoid a redundant repaint.

## Verification
Build initially failed on missing WorkspaceTabsWhileCtrl in the vendored vtui.
After merging that dependency, its complete test suite passes. Targeted editor
BOM, hidden document geometry and command-surface inventory checks pass. Keep
both fork private-sync regressions and upstream fragmented zsh echo regressions.
The command inventory includes the fork's six additional audited surfaces.

## Prevention
Merge dependency code as well as go.mod pins when a replace points at a local
copy. Keep lifecycle, resource-cache and platform mapping tests during conflict
resolution. Further useful verification is a live Windows ConPTY startup/resize.

Tags: #merge #upstream #vtui #pty #editor #regression
