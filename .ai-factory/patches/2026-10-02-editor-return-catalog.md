# Editor return must restore the panel catalog

- Reproduced in the Qt app with a disposable text file in `wizard_last_page`: after F4, saving, and Escape, the panel displayed “Folder is empty” while its status still counted the files.
- Cause: the editor-to-panels incremental scene transition installed a row-free shell. After the save advanced the panel catalog revision, Qt requested a snapshot, but the host's retained scene contained only the header and returned zero rows.
- Fix: reject the row-free incremental transition when returning to panels, allowing the existing full semantic export to supply the catalog. Entering the editor remains incremental.
- Regression: `TestAppScenePatchReturnsFromEditorWithCatalogSnapshot` rejects the unsafe patch; it failed before the fix and passes after it.
- Live Qt verification: saving and exiting the editor twice left the gallery populated and the selected test file visible.
