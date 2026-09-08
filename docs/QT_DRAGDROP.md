# Qt drag and drop

The Qt frontend accepts local file URLs from the desktop as copies. Files
dropped on a directory go into that directory; files, `..`, and empty panel
space target the current directory. Hidden panels, document surfaces, blocking
overlays, and known read-only destinations refuse drops.

Drag a marked entry to carry the marked set. With no marks, drag one entry.
Pressing an unmarked entry while other entries are marked retains cursor
navigation. Ctrl/Shift selection presses retain their existing meaning; change
modifiers after starting the drag. Qt's system drag-distance threshold applies.

Between panels in the same Qt host, the default is copy and Shift selects move
(Ctrl takes precedence). Both use Go's VFS file-operation engine, including its
queue, overwrite/error dialogs, cancellation and progress. The native desktop
transaction remains Copy: only the Go operation owns an internal move and source
deletion. The system copy badge therefore also appears during an internal move.
Dragging local files to another application offers Copy only. Remote/archive
entries can be dragged between panels without materializing temporary files;
exporting them to other applications is not implemented in this first stage.

## Implementation

- `F4GalleryBridgeDragDrop.cpp` observes the registered panel windows, owns the
  native `QDrag`, decodes `QUrl` file URLs (including UNC paths), and rejects
  arbitrary non-file URLs. An opaque per-gesture MIME token identifies drags
  from this host; no VFS handle or credentials cross the desktop MIME boundary.
- `GalleryPanelHost.qml` uses the gallery layout's viewport hit testing for all
  five presentation modes, paints a scene-snapped outline and scrolls at edges.
- A complete local selection starts directly from the current catalog. For
  entries outside Qt's sparse cache, `panel.prepareDrag` requests the full list
  from Go; `drag_prepared` replies by request ID. Release, cancellation, changed
  panel identity/path/revision and late replies cannot start a new gesture.
- `panel.dropFiles` carries target panel identity, path and catalog revision,
  with either local paths or a source identity and stable entry IDs. Go validates
  both ends again and captures the source directory before scheduling
  `ExecuteFileOpAt`. Qt never copies, removes or renames files itself.
- `dropAllowed` is a typed panel capability; the ExtUI state-patch validator and
  Qt catalog state preserve it. File-operation errors remain authoritative when
  a provider cannot determine writability in advance.
- Windows guards the OLE startup/release race: a gesture that has already
  released cannot leave a native drag waiting for another click. A stalled
  transaction is cancelled after two released-button polls; an available
  internal destination is then resolved again. An unavailable destination
  cancels the gesture.

External Move/Link and remote export need a separate completion and temporary
file-lifetime contract. A native drop acknowledgement is not proof that a queued
transfer completed.

## Checks

Go regression: `TestSemanticDropPlan`, including stale identities, target
resolution, complete drag preparation, captured source directory and rejection
of external moves/relative paths. Existing drag/drop tests remain applicable.

Qt regressions: `F4GalleryPointerTests::nativeDropUsesIdentityAndSnappedOutline`,
`nativeDragListsSurviveWireEncoding`, and
`ExtUiSceneReducerTests::dropCapabilitySurvivesStatePatch`. Run pointer tests
at scale factors 1 and 1.75. The outline test includes a fractional ancestor,
all presentation modes, read-only/modal gating, internal-token validation and a
rendered capture. Static test executables need the Basic controls style and a
font directory when running with the offscreen platform.

The portable static host retains Qt Quick Effects shader resources, verified
by the pointer regression, so they remain available after cache extraction.
The transport regression verifies that native `QStringList` paths and entry
IDs are MessagePack arrays rather than scalar strings.

Windows validation included launching the embedded static host from its cache,
copying a file between the two live panels and comparing SHA-256 hashes.
Explorer round trips and native macOS/Linux desktop gestures still require
platform-specific manual verification.
