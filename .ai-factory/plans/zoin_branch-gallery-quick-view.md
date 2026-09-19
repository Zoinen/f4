# ZoinGallery Quick View

Baseline: F4 259139449e3cc3d496490e7fd45aa145374002b8; ZoinGallery a65d3ab0eaa6cf06f5abfbf9501661ce6e817f75.
Implement the user-approved plan in this isolated worktree; never import the source checkout's uncommitted changes.

## Requirements

Default native gallery image Quick View; optional built-in F4 image renderer (off), hover previews (on), both in Gallery & Cache and persisted by Qt. Non-images retain F4 previews. Hover never changes cursor/selection/focus and clears on leave or keyboard navigation. Docked Enter expands; Escape returns list focus; Ctrl+Q closes. The same viewer survives animated expansion/collapse. Docked presentation has no show/hide animation or shell hiding. Fit recalculates; custom zoom stays absolute with the same image point centered. Preserve full viewer features, native resource decoding, terminal compatibility, stale-request rejection, and 175% physical-pixel alignment.

## Tasks

- [x] Create clean worktree and initialize pinned submodule.
- [x] Implement revisioned Go preview overrides and native image ownership, with regression tests.
- [x] Implement Qt preferences and coordinator states.
- [x] Integrate persistent dock/full viewer, hover/focus routing, and viewport preservation.
- [x] Extend Qt/ZoinGallery regressions, including 175% geometry and rendered captures.
- [x] Update documentation; build portable worktree binary; verify and launch with --gui=qt.

Testing and documentation are required. Use existing opt-in diagnostics. Do not merge, overwrite, or stop another checkout's application.

## Verification results

- Go panel, native projection, ExtUI, architecture/composition tests passed.
  Embedded Qt-host lifecycle tests passed; CGO remained disabled and the system
  Go build cache was used.
- Coordinator and new bridge/pointer/surface regressions passed. ZoinGallery
  interaction suite: 18 passed; reusable primitives: 22 passed.
- GPU-rendered Windows tests at DPR 1.75 covered both dock sides, persistent
  viewer/session identity, custom zoom/center, rotation, resize, interrupted
  transitions, error rendering, settings leaves, and transition endpoints.
  The error label initially mapped to physical (1130.75, 581.5); the added
  regression failed before its scene-space alignment fix. Rendered failure
  checks also caught and fixed the previous image remaining behind an error.
- Static QML import isolation and the Windows system-DLL import audit passed.
- Five broader Qt failures reproduced on the exact pristine pinned baseline:
  panelCatalogPatchLeavesOtherSessionUntouched,
  inactiveGalleryDoesNotStealFocus, viewerOwnsEscapeAndZoom,
  sparseRefreshReachesGalleryAndCrossesPagingThreshold, and
  themeSelectionBordersAreLiveAndPersisted. They are not feature regressions.
- Fresh host was compressed and embedded in f4-gallery-quick-view.exe, then
  launched directly with --gui=qt. The responding extracted Qt child matches
  the built host SHA-256 and received the runtime branch argument; its title is
  f4 [zoin_branch/gallery-quick-view]. Centered header regressions passed.
  The previously running application remained alive.
- Worktree lives at C:/Users/xs/.codex/worktrees/f4-gallery-quick-view after a
  disk-capacity move. Logs and rendered captures are in .diagnostics; native
  build outputs are in .build. No Linux/macOS release validation was performed.

## Cursor latency follow-up

Native panel navigation defers the Go cursor commit until key release. The
Quick View presentation now uses that revision-validated pending image ID
immediately and requests it through the existing gallery session. Delayed
shell frames cannot restore the previous image. Non-image provider handling
is unchanged. The regression first failed with left:two still presented for
an immediate left:one cursor intent, then passed after the fix. Five targeted
bridge scenarios passed (7 including setup/cleanup), as did static QML imports
and the Windows import audit. Rebuilt and re-embedded the host before replacing
only this worktree's executable.
