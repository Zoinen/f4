# Show Android immediately during initial discovery

Go already clears the old rows when SwitchToVFS installs the Android manager.
The Qt Gallery bridge deferred provisional replacements across paths and kept
the previous local catalog visible until Android discovery completed.

Limit cross-path provisional retention to local-to-local transitions. VFS
destinations now replace the catalog immediately, including an empty provisional
list. Same-folder refresh retention remains unchanged.

Regression: androidDriveSwitchClearsPreviousCatalogImmediately starts with four
local rows and switches to an empty loading Android catalog. Before the fix the
Gallery model still had four rows; after the fix it has zero and subsequently
accepts the completed device catalog. Tested at 175% scale; no layout arithmetic
or transforms were changed.

sameFolderRefreshKeepsGalleryObjects passes. The additional sparseRefreshReachesGalleryAndCrossesPagingThreshold
test fails at the sparse-to-dense reset assertion both before and after this fix.
