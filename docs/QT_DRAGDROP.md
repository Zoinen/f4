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

Temporary-panel roots collect references, matching F5/F6: dropping there does
not copy file contents or delete originals, including with Shift. Dropping onto
a referenced directory transfers into that directory's underlying VFS. Copies
from a temporary panel use the original basename and detect self-copies against
the underlying source before opening the destination. Local temporary-panel
references can supply real desktop file URLs; archive/remote references cannot.

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

## Follow-up verification, Windows 175%, 2026-09-08

The worktree is `zoin_branch/qt-drag-drop`. These results distinguish semantic
file-operation checks from actual desktop gestures; the full matrix is still
in progress.

| Scenario | Evidence so far |
| --- | --- |
| Real panel to real panel | Live copy, content/hash comparison; repeated conflict cancellation; switching to Queue and back followed by another copy |
| Cancel conflict keeps originating workspace | Live at 175%, plus three consecutive queued-operation cancellations in `TestQueuedDropCancelKeepsOriginatingWorkspace` |
| Real to TempPanel root | Live reference insertion at 175%; original remains visible |
| Real/TempPanel to real/TempPanel, copy and move | Sixteen semantic/VFS combinations in `TestSemanticDropRealTemporaryMatrix`, including nested folders and source retention/removal |
| TempPanel to real directory | Live at 175%: two conflict cancellations followed by overwrite; file SHA-256 matches. Nested folder also copied, cancelled and retried with overwrite |
| TempPanel to another TempPanel | Live reference insertion from slot 0 into empty slot 1 at 175% |
| Drop after dialog/full scene rebuild | Regression reproduced `dropAllowed` changing from true to false; `TestSemanticDropCapabilitySurvivesFullSceneRebuild` now preserves both writable and read-only capabilities |
| Nested TempPanel paths | Matrix reproduced failure on the second directory level; resolver now follows each `/c/<component>` segment |
| TempPanel reference onto its original | Regression reproduced truncation before fix; `TestTemporaryPanelCannotOverwriteItsReferencedSource` now verifies unchanged original |
| TempPanel local desktop URLs | `TestTemporaryPanelDragExportsRealLocalPaths`; actual Explorer export unverified |
| Cancel multi-directory external payload | Regression reproduced copying later groups after cancellation; `TestExternalDropCancelStopsRemainingGroups` now passes |
| External file and folder payloads | `TestSemanticDropExternalFilesAndDirectories`: local directory, TempPanel root and referenced directory; original contents preserved |
| Drag after catalog publication | `TestSemanticDragPreparationAfterPublishedCatalog`, paged and non-paged catalogs, three successive preparations each |
| Panel identity mismatch | Old live stderr ended with this error. Protocol and controller regressions verify snapshot recovery without closing or advancing rejected revision; original producer-side ordering cause remains to be isolated |
| Explorer to/from real and virtual panels | Test folder prepared; native automation rejects drag endpoints outside the source window. Real Explorer round trips remain unverified |
| Real/ZIP to real/ZIP | Six semantic/VFS file copy/move combinations in `TestSemanticDropArchiveMatrix`; no live archive gesture or archive conflict coverage yet |
| Remote providers and remaining conflict matrix | Not fully verified; external remote/archive materialization and external moves are not implemented |

Queued transfer dialogs now anchor to their originating panels. Unpublishing a
metadata snapshot no longer unregisters the newly published live panel. On a
panel-identity mismatch Qt requests one fresh snapshot, ignores further deltas
for that stream until it arrives, and leaves unrelated streams running.

Full scene promotion also preserves `dropAllowed`; losing that capability after
closing a dialog was the reproduced cause of subsequent drops being ignored.
The Windows drag bridge records startup release/cancellation separately from
native target entry so an asynchronous preparation can finish its internal
gesture without treating Escape as a drop.

Latest validation: Go drag/drop, transfer, TempPanel and scene rebuild regressions
pass; all 19 pointer tests pass at both 100% and 175%, with the rendered outline
capture inspected. The current static Windows host import audit passes.
