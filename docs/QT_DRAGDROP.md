# Qt drag and drop

The Qt frontend accepts local file URLs from the desktop as copies. Files
dropped on a directory go into that directory; `..` targets the parent directory.
Files and empty panel space target the current directory. Hidden panels, document surfaces, blocking
overlays, and known read-only destinations refuse drops.

Drag a marked entry to carry the marked set. Drag an unmarked entry to carry
only that entry, even when other entries are marked. Hold Alt at mouse press
(Command on macOS) to drag only the pressed entry regardless of marks.
The dragged set is captured at mouse press; Ctrl/Shift may be held from the
start or changed during the drag to choose copy/move. Qt's system drag-distance
threshold applies.

Between panels in the same Qt host, the default is copy and Shift selects move
(Ctrl takes precedence). Both use Go's VFS file-operation engine, including its
queue, overwrite/error dialogs, cancellation and progress. The native desktop
transaction remains Copy: only the Go operation owns an internal move and source
deletion. The drag artwork follows the internal Copy/Move action.
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
Explorer round trips were subsequently exercised on Windows at 175% (below).
Native macOS/Linux desktop gestures still require platform-specific verification.

## Follow-up verification, Windows 175%, 2026-09-08

The worktree is `zoin_branch/qt-drag-drop`. These results distinguish semantic
file-operation checks from actual desktop gestures. Virtual-panel coverage uses
TempPanel and ZIP; remote-service coverage is not implied.

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
| TempPanel local desktop URLs | `TestTemporaryPanelDragExportsRealLocalPaths`, plus live file/folder export to Explorer and hash comparisons |
| Cancel multi-directory external payload | Regression reproduced copying later groups after cancellation; `TestExternalDropCancelStopsRemainingGroups` now passes |
| External file and folder payloads | `TestSemanticDropExternalFilesAndDirectories`: local directory, TempPanel root and referenced directory; original contents preserved |
| Drag after catalog publication | `TestSemanticDragPreparationAfterPublishedCatalog`, paged and non-paged catalogs, three successive preparations each |
| Panel identity mismatch | Old live stderr ended with this error. Protocol and controller regressions verify snapshot recovery without closing or advancing rejected revision; original producer-side ordering cause remains to be isolated |
| Explorer to/from real and virtual panels | Live file/folder copies in both directions at 175%, including TempPanel references; Cancel, retry, overwrite, file-in-use failures and Escape verified (details below) |
| Real/ZIP to real/ZIP | Six semantic/VFS file copy/move combinations in `TestSemanticDropArchiveMatrix`; live file copies in both real/ZIP directions at 175%, plus ZIP-to-real Cancel followed by a new drag and Overwrite |
| File/folder conflicts | 72 combinations in `TestSemanticDropConflictMatrix`: local/TempPanel/ZIP source, local/ZIP target, copy/move, Cancel/Skip/Overwrite; contents, source retention and workspace checked |
| Destination write failure | 18 combinations in `TestSemanticDropWriteFailurePreservesSource`: local/TempPanel/ZIP source, copy/move, Abort/Skip/Retry then Abort; source contents preserved and error dialog remains on originating workspace |
| ZIP directory move cleanup | `TestRemoveDirectoryPreservesOtherArchiveMembers` covers explicit and implicit directory records, nested entries and preservation of similarly prefixed siblings |
| Remote providers | Not verified against a live remote service; external remote/archive materialization and external moves are not implemented |

Queued transfer dialogs now anchor to their originating panels. Unpublishing a
metadata snapshot no longer unregisters the newly published live panel. On a
panel-identity mismatch Qt requests one fresh snapshot, ignores further deltas
for that stream until it arrives, and leaves unrelated streams running.

Full scene promotion also preserves `dropAllowed`; losing that capability after
closing a dialog was the reproduced cause of subsequent drops being ignored.
The Windows drag bridge records startup release/cancellation separately from
native target entry so an asynchronous preparation can finish its internal
gesture without treating Escape as a drop.

Archive bulk copying is used only when the selected destination names are
absent. Otherwise the regular recursive engine owns overwrite decisions,
including nested conflicts. Archive directory removal enumerates the selected
subtree and removes exact archive records, handling implicit directory markers.

Latest validation: Go drag/drop, transfer, TempPanel and scene rebuild regressions
pass; all 19 pointer tests pass at both 100% and 175%, with the rendered outline
capture inspected. The current static Windows host import audit passes.

## Native Explorer verification, Windows 175%, 2026-09-08

Using direct Windows mouse-input emulation explicitly authorized by the user,
place Explorer and the worktree Qt host side by side on the 175% display. Both
windows reported DPI 168. Generate only disposable data under `.diagnostics`.
The gestures cross actual window boundaries and invoke native OLE drag/drop;
no filesystem copy command substitutes for a gesture.

| Native case | Observed result |
| --- | --- |
| Explorer -> real panel, file and nested folder | Copy completes; original and destination hashes match |
| Real panel -> Explorer, file and nested folder | Copy completes; original and destination hashes match |
| Explorer -> TempPanel root, file and nested folder | References appear; originals remain |
| TempPanel -> Explorer, file and nested folder | Original paths resolve correctly; exported contents match |
| Explorer -> referenced directory in TempPanel | File is copied inside the underlying directory, not added at the TempPanel root |
| Explorer -> real panel, existing file | Cancel leaves the originating panels visible; a new drag opens the conflict again and Overwrite succeeds |
| TempPanel -> Explorer, existing file | Explorer's Replace or Skip Files dialog appears; Escape cancels it; a new drag and Replace succeeds |
| Locked source, Explorer -> real panel | Cannot open source file dialog; Retry repeats the error; Abort exits without creating the destination or switching to Queue |
| Locked source, Explorer -> TempPanel root | Reference insertion succeeds without reading file contents |
| Locked source, TempPanel/real panel -> Explorer | Explorer reports File In Use; cancelling leaves F4 usable and source contents intact |
| Retry after removing the lock, TempPanel -> Explorer | A new drag copies the file successfully; contents match |
| Locked destination, Explorer -> real panel | Overwrite leads to Cannot create destination file; Skip leaves the existing file unchanged and retains the workspace |
| Escape during real-panel export | No destination file is created; the next ordinary drag succeeds |
| Incoming Shift, Explorer -> real panel / TempPanel root | F4 accepts Copy / reference insertion; source remains |
| Outgoing Shift, real panel -> Explorer | Explorer refuses Move because only Copy is offered; no folder is created. Repeating without Shift copies the folder |
| Internal Shift, real panel -> TempPanel root | Adds a reference and preserves the original |
| Internal Shift, TempPanel -> real directory | Moves the file, preserving its contents and removing the original |
| Internal Shift, real panel -> referenced directory | Moves a nested folder into the underlying directory and removes the source subtree |

Final assertions compared ten SHA-256 source/destination pairs and the contents
and source removal of both internal moves. The same F4 process remained alive
through the error/cancellation/retry series. The pre-existing Go conflict and
failure matrices provide broader combinations than these native gestures.

External Move/Link, exporting archive/remote contents to the desktop, live
remote services and native macOS/Linux gestures are outside the verified
Windows Copy contract. Incoming Shift and outgoing Shift have different
observed behavior as recorded above; do not describe external moves as working.

## Panel workspace tabs

Dragging a supported file payload over a visible panel tab activates it immediately,
without a hover timer. The drag can then continue into either visible panel in that
workspace. Dropping on the tab itself targets the current directory of its active
panel. Editor, terminal and operations-queue tabs are not file drop targets; modal
and blocking overlays retain their existing input guards.

Internal drags retain the source panel identity, path, catalog revision and entry
IDs across tab changes. Go resolves that source only among still-owned workspace
panels and rejects closed, removed or changed sources. Copy and Shift-move use the
existing transfer pipeline; completion refreshes views of the affected directories. External
file drops retain Copy-only semantics.

Verified on Windows at 175% with native mouse gestures: copy through an inactive
tab into its other directory, Shift-move directly onto the tab, source-panel refresh,
Escape cancellation followed by a successful repeat, and Explorer file drop onto an
inactive tab. Internal and Explorer copies matched SHA-256 hashes. Qt pointer and
actual WorkspaceTabs hit tests passed at 100% and 175%; Go regressions cover stable
tab/source identities, immediate activation and the active-panel destination.

## Standalone Gallery drag visuals

The Windows action-cursor artwork is ported from standalone ZoinGallery master
`fc1d150254f192da61f1848fc8a2e2a9932a1102` (`FileListModel.cpp`). It uses the native
arrow/forbidden cursor and the original Copy/Move pill, including equal cursor
canvas dimensions across actions. Modifier changes update the Copy-action cursor
bitmap for an internal Go-owned Shift-move; the external OLE contract stays Copy-only.
Qt Windows GiveFeedback observes cursor pixmap cache-key changes even while the
pointer is stationary.

Single-item previews capture the panel's actual rendered preview and preserve the
pointer hotspot, as standalone `BrickDelegate.qml` does. Multiple selections use
its 46px thumbnail/icon tiles, 6px spacing, 58px strip and a +N overflow tile after
five items. Cached Gallery thumbnails supply the images; icons cover uncached or
non-image entries. The compact strip is painted natively using the standalone
style colors, so it survives unloading a panel when hovering another workspace.

Verified at 100% and 175%: preview image/count rendering and identical action cursor
canvas sizes. Native Windows testing also verified the selected image preview,
eight-file strip (five matching thumbnails +3), and stationary Shift changing
Copy to Move. These visuals do not alter drop operation ownership.

## Drop destination outlines

Whole-panel outlines span the owning FilePanelView's full width, outside the
Gallery content gutters. Both edges are snapped in scene space at the current
DPR. Folder outlines retain their item bounds. Native hover rejects the source
catalog (including another local panel showing the same path) and a selected
source folder itself; a different child folder remains a valid destination.

Hovering a panel workspace tab outlines that workspace's active, writable panel
as soon as activation is reflected in the scene, even without another mouse move.
Tab hover does not auto-scroll the panel. Leaving the tab preserves the next
panel/folder hover indicator; leaving or finishing the drag clears it.

Regression tests cover these target rules, tab hover, and full-width physical
edges at 100%/175%. Windows native drags confirmed full-edge outlines while
hovering a tab and then its panel, and no outline when dropping back into the
source panel's current directory.

The drop hit area now shares the outline's snapped scene-space rectangle, including
both content gutters. Gutter drops target the panel directory; item hit testing is
only used inside the gallery viewport. Tests exercise the first/last interior
physical pixels and rejection beyond all four edges at 100% and 175%. Native
Windows drops into both side gutters were accepted by the destination panel.

## Same-directory refresh

Refresh keeps the last successful listing visible until the latest read completes.
Failed and superseded reads cannot replace it. A no-op observation retains entry
objects and the catalog revision. Insertions, removals and ordering changes retain
surviving identities, selection and cursor; deleting the cursor row falls back to
the next row, or the previous row at the end. Calculated directory sizes survive
when the directory metadata is unchanged. Access-time changes do not invalidate
previews. Unknown content versions are revalidated on each observation.

Clients advertising `panelCatalogDeltaV1` receive an optional `catalogDelta` in
the existing catalog transaction. Its `baseCatalogRevision`, `oldTotalCount` and
`ranges` (`oldIndex`, `index`, `count`) map unchanged rows to their new positions.
The delta never enters `state_update`. Catalogs with at most 512 rows retain the
existing complete payload; larger catalogs keep bounded page requests and send
retained ranges instead of a second full list of file objects. Missing/stale base
revisions use the existing authoritative catalog replacement path.

Gallery uses insert/remove/move signals for complete catalogs and persistent-index
remapping for sparse catalogs, retaining materialized rows and decoded media.
Crossing the paging threshold also preserves surviving objects. Changed content
invalidates its versioned source; unchanged rows retain their preview work. The
existing incremental MasonryLayout path restores viewport anchors and delegates.

Operation completion refreshes matching live directory views and their parent
views (folder metadata), including other workspaces. Copy does not refresh an
unrelated source. Move deduplicates source/destination views. Background MTime
polls from an older load generation cannot cause another read after completion.
On Windows, cached preview readers use read-only handles with delete sharing so
they do not block the application's own move/delete/replace operations.

Regression coverage includes no-op/error/stale reads, cursor fallback, affected
view deduplication, catalog-only delta transport, Qt model signal consistency,
100,000-row sparse updates, paging-threshold transitions, and preview handle
sharing with replacement-content validation. Gallery session tests and F4 pointer
tests pass; pointer tests run at both 100% and 175%. Native Windows gestures at
175% verified copy with overwrite confirmation and a successful Shift-move into
`test`, with the unrelated panel unchanged.
