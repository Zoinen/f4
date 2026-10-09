# Retain command-line layout beneath transparent dialog workspaces

The Find File workspace clears its command-line stream while QML retains the
underlying panels. An empty frame lacks `visible: false`, so the host interpreted
it as visible and reserved an empty bottom strip.

The Qt regression reproduced the transition at 175% scale: command-line height
changed from zero to 38.285714 logical pixels (67 physical pixels). It covers both
hidden and visible command lines during progress and results, checks unchanged
panel heights, and verifies that a subsequent shell restores its own visibility.

ShellSceneStore now retains the last nonempty command-line presentation, matching
the existing retained shell. Its initial state is hidden. Empty stream updates
do not replace that presentation; explicit updates still apply immediately.

Validation: dialogOnlyWorkspaceRetainsCommandLineLayout,
commandLinePanelToggleIsImmediate, commandLineAutoHideRevealsUpward, and
qmlImportsWithoutInstalledQt pass at 175% scale.
