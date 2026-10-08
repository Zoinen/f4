# Workspace switching: retained native state — 2026-10-08

The updated portable Qt application is running from `D:/Code/f4-zoin/f4-zoin.exe --gui=qt` with normal logging and no diagnostic runner. Its window shows the zoin worktree identity.

## Live result

Same three restored workspaces and alternating typed workspace.activate actions as the baseline. Each run waits eight seconds after startup and performs 24 switches at one-second intervals. Four initial switches are excluded from warm statistics. The final Qt log ends during the last shell snapshot, so that incomplete sample is excluded: nine valid returns to tab 1 and ten switches to tab 2.

| Destination | Before median | After median | After p95 | Improvement |
|---|---:|---:|---:|---:|
| Tab 1 | 331.222 ms | 64.171 ms | 80.162 ms | 5.2× |
| Tab 2 | 238.250 ms | 71.394 ms | 74.955 ms | 3.3× |

Frame latency is measured from Qt's activation event to the first completed frame after application of the menu, both panel catalogs, and shell snapshots. This is frame completion rather than display/photon latency. The runner uses the real Qt → Go action path and real restored panels, bypassing physical mouse dispatch. Snapshot responses lack action trace IDs and are associated by their one-second activation intervals. p95 uses linear interpolation.

| Qt application phase, median | Tab 1 | Tab 2 |
|---|---:|---:|
| menus_snapshot | 4.747 ms | 9.517 ms |
| both panel_catalog_snapshot messages | 18.527 ms | 23.720 ms |
| shell_snapshot | 15.907 ms | 16.271 ms |

The Go cross-identity patch guards remain intact. Workspace changes still send authoritative snapshots; Qt now reuses retained models and views while validating genuine updates. The remaining 64–71 ms is measurable publication, binding and frame work; this change does not achieve a 30 ms switch.

## Changes

- PanelSessionRegistry retains catalog/session pairs by panel identity, with an eight-session limit per side and LRU eviction. Hidden catalogs retain data while releasing in-flight page/metadata leases.
- FilePanelView retains up to eight gallery renderer views per side. Returning to an existing panel selects its prior view, layout and delegates. Hidden views cannot own panel input or drop handling; the active view re-registers its drag endpoint.
- GalleryPanelHostAdapter resolves the displayed descriptor's exact session. A catalog arriving before the shell can no longer install the new session underneath the previous tab's descriptor. Renderer transactions are filtered by panel identity.
- ShellSceneStore captures authoritative descriptors before dropping compact overrides, avoiding a transient return to the old workspace.
- ApplicationMenuPopup creates delegates while visible. An otherwise equivalent menu-bar publication with only a changed Go object ID preserves its presentation. Actual command/checkmark changes remain observable.
- Cached session file-field callbacks cannot publish metadata for another active panel. Initial session registration preserves the snapshot needed to refresh icon themes.

## Verification

Focused native regressions: 9 passed, including retained identity/model reuse, newer catalogs, icon-mode refresh, and repeated directory/parent navigation. Registry tests: 5 passed, covering identity retention, catalog revisions, pending-lease cleanup and bounded eviction. Typed shell/menu store tests: 9 passed.

Qt UI checks at 175%: 7 passed, including returning to the same renderer object across masonry/details modes, atomic shell publication, column resizing, lazy application-menu commands, and embedded QML imports. The renderer check enumerates visible native Text/Image leaves, requires stable object names, and verifies physical-pixel origins plus identity scene transforms for each leaf. Rendered masonry, details and menu captures were inspected with Windows fonts; small size/shortcut text remains sharp.

Full gallery suite: 67 passed, 7 failed, 1 skipped. The seven failures were reproduced with the pre-change bridge/session and adapter behavior: repeatedOpenReplaysAgainstUsefulLocalPreview, panelCatalogPatchLeavesOtherSessionUntouched, inactiveGalleryDoesNotStealFocus, galleryRoutesOwnedAndCommanderKeys, viewerOwnsEscapeAndZoom, catalogPathChangeAppliesPresentationBeforeSessionSignals, and equalGalleryColumnSchemaDoesNotResetLayout. The baseline also fails the newly updated retention test, as expected because its old behavior replaces the session. Two icon-refresh failures identified during development were fixed and now pass.

Static Windows system-DLL import audit, embedded-only QML import test, compressed host packaging, and CGO_ENABLED=0 portable Go build passed. The generated long linker response files for native tests were split into separate lines to work around the existing Windows response-file truncation; no build-system source was changed for that workaround.

## Evidence

- [Final timings](D:/Code/f4-zoin/artifacts/workspace-cache-final-summary.json)
- [Final Qt timeline](D:/Code/f4-zoin/artifacts/workspace-cache-final-qt.jsonl)
- [Final Go timeline](D:/Code/f4-zoin/artifacts/workspace-cache-final-go.jsonl)
- [Focused native checks](D:/Code/f4-zoin/artifacts/workspace-cache-regressions-verified.txt)
- [175% UI checks](D:/Code/f4-zoin/artifacts/workspace-cache-ui-modes-verified.txt)
- [Full gallery checks](D:/Code/f4-zoin/artifacts/workspace-cache-gallery-verified-full.txt)
- [Baseline comparison](D:/Code/f4-zoin/artifacts/workspace-gallery-baseline-full.txt)
- [Masonry capture](D:/Code/f4-zoin/artifacts/workspace-retained-panel-175.png)
- [Details capture](D:/Code/f4-zoin/artifacts/workspace-retained-details-175.png)
- [Menu capture](D:/Code/f4-zoin/artifacts/workspace-retained-menu-175.png)

AI assistance has been used to create this output.
