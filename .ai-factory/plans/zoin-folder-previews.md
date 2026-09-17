# Restore folder image previews in F4-qt Masonry

Source: user-approved implementation plan in this task, 2026-09-15.
Branch: zoin. Testing: mandatory. Docs: update existing Qt host documentation.
Logging: existing opt-in F4 media timing trace.

## Required behavior

Automatically preview visible local and VFS folders in Masonry. Inspect the first
200 non-directory files in VFS delivery order, including hidden and unsupported
files; filter using Gallery's supported formats, naturally sort, evenly sample
at most 16. No recursion. Preserve square outer geometry, original folder-tab
frame and filename below, decorative children and existing folder interaction.
Nested aspect-fit grid columns: 1/2/3/4 at widths below 80/150/300/otherwise.

Go owns enumeration and directory authority; an optional directory descriptor
travels through the existing paged catalog. Negotiate directoryPreviewsV1.
Independent media operations enumerateDirectoryPreview and
resolveDirectoryPreview return bounded results with listing/child leases.
Separate preview references from main-catalog image references. Two directory
operations globally, one per non-local VFS session; 30-second deadlines and
private quota cancellation. Keep admission occupied until the provider returns.

Gallery runtime gets an optional DirectoryPreviewProvider. Visible demand owns
small external child catalogs with independent cancellation and shared image
caches. Retain at most 32 inactive models per panel; stale results are guarded by
catalog revision and source observation. Preserve last successful display on
refresh failure; empty success clears it. Existing refresh/re-entry/fileops
invalidate snapshots. Update both external folder roles and lazy ImageFile state.

Explicit contained layout must publish geometry before each image source.
Every affected text/image leaf needs a stable objectName, physical scene origin
and identity-transform checks at real DPR 1.75, plus rendered verification.

## Execution

- [x] Go broker, directory leases, media operations and regression tests.
- [x] Negotiated semantic directory descriptors and Qt media provider transport.
- [x] Gallery visible demand, child catalogs, cancellation and cache lifecycle.
- [x] Original folder presentation, contained grid and 175% regression tests.
- [x] Integration checks, documentation, submodule pin and portable worktree build/launch.

## Validation

Cover first-200 within oversized callbacks, directories/unsupported entries,
sampling, EOF/failure/quota/caller cancellation, stale leases and unauthorized
selection, reconnect, independent panel/viewer ownership, slow/uncancelable VFS,
all grid densities/aspects, stable outer geometry, sparse/recycled rows,
scroll/mode/navigation cleanup, interactions, pixel positions/transforms/renders.
Use existing Go and CTest suites and portable build gates. Build successfully
before replacing only this worktree's f4-zoin.exe; launch with --gui=qt.

Known limitation: providers may buffer a whole listing before callbacks; the
200-file budget bounds preview-owned data, not every provider's internal work.

## Execution verification (2026-09-15)

- Go suites passed: internal/plughost, internal/panel, sdk/extui, cmd/f4;
  generated embedded-payload/extraction and directory tests also passed with
  CGO_ENABLED=0 and the f4_embedded_qt_host tag.
- GallerySessionTest, MasonryLayoutModesTest, MasonryVisualDpr175Test and
  QtMediaClientTest passed. New F4 bridge directory-authority/open-target test
  passed. At DPR 1.75 the initial raster origin (22.75, 70) failed; final
  per-leaf origins, identity transforms, frame dimensions and 1-pixel borders
  passed. Inspected rendered filename and varied-aspect checker thumbnails.
- Static qmlImportsWithoutInstalledQt and compiledHostLoadsItsQmlModule passed;
  Windows DLL import audit passed. The launched extracted Qt host SHA256 is
  f8bdf3fda4c1bca6f940c0d62e2672cc9061efc6af2a5666ea8466625604a8e8,
  matching the rebuilt host. Worktree window is responsive, titled f4 [zoin].
- ZoinGallery is pinned to local commit 654243894524226d99ec101268ba57e58c6b5ac9
  (not pushed). Only f4-zoin.exe was replaced; the previous worktree binary is
  retained as f4-zoin.exe.previous-folder-previews-20260915-212910.
- A final regression reproduced hidden sample 16 receiving metadata in a
  four-cell preview. Contained layouts now suppress catalog-wide metadata
  demand, and hidden pooled image leaves cannot mark a preview ready.
  The regression, full MasonryLayoutModesTest, MasonryVisualDpr175Test,
  static QML checks, Windows import audit and regenerated payload tests passed
  again. Rebuilt and relaunched the corrected worktree executable; the
  intermediate build is retained as
  f4-zoin.exe.previous-folder-previews-final-20260915.
- Live follow-up in P:\[Year 2026] exposed missing MessagePack list encoding
  and missing child-source normalization, which local-path fixtures bypassed.
  Both were reproduced in the real adapter socket test and corrected. Small
  DNG bitmap previews were also mislabeled JPEG; a runtime-file regression
  reproduced this and now passes alongside synthetic 8/16-bit sample tests.
  Full GallerySessionTest and QtMediaClientTests passed again. The corrected
  real window shows folder collages (capture: .diagnostics/folder-real-final.png),
  with 122 successful decode completions and no failures in the traced run.
  The final worktree binary is running with verbose tracing disabled.
- Navigation follow-up (2026-09-16): reproduced the reported access violation
  under CDB. A queued Masonry rewrap retained a destroyed preview model.
  Layouts now detach synchronously on model destruction and discard obsolete
  reset slots. The ownership regression, 20 rendered navigation cycles,
  full Masonry modes/175% suites and packaging gates pass. The live application
  survived 30 entry/exit cycles; the rebuilt embedded host hash above was
  verified after launching the final worktree executable.

- Folder presentation follow-up: restored the original blue tab/frame, opaque
  interior, original spacing and centered two-line, right-elided filename.
  A regression first observed AlignLeft (1), expected AlignHCenter (4).
  The extended rendered test checks long captions, non-overlap, physical origins
  and identity transforms at 175%. Masonry modes, DPR suite, static QML gates,
  Windows imports and embedded payload tests passed. Capture:
  .diagnostics/folder-original-style-175.png.

- Grid and Icon extension: shared folder collage loading/presentation across
  Masonry, Grid and Icons. Parameterized regression first failed in both new
  modes, then passed with 175% leaf geometry, rendered captures and mode
  switching through Details. Full Masonry modes/DPR and portable packaging
  checks passed. Rebuilt and launched the worktree executable.

- Icon-to-Grid recovery: reproduced retained four-cell previews failing to
  populate nine-cell grids. Resume now invalidates child image readiness after
  installing the refreshed catalog and lease, replaying visible demand even
  when entries are unchanged. Multiple-folder one-to-nine and four-to-nine
  repeated transitions pass at 175%, as do full Gallery session, Masonry modes,
  DPR and portable packaging checks. Rebuilt and relaunched the worktree app.

The cache/settings follow-up is documented in zoin-gallery-cache-settings.md;
it preserves collages across navigation and adds configurable disk caching,
color/animation controls and decoder inventory to native Settings.

The complete older host suites are not green: F4GalleryBridgeTests reports
panelCatalogPatchLeavesOtherSessionUntouched, inactiveGalleryDoesNotStealFocus,
and viewerOwnsEscapeAndZoom; F4GalleryPointerTests reports
sparseRefreshReachesGalleryAndCrossesPagingThreshold; F4QuickViewSurfaceTests
at 175% reports themeSelectionBordersAreLiveAndPersisted. These failures are
outside the new directory-preview scenarios. The sparse-to-dense reset is
explicitly present in the pre-feature GallerySession implementation. Full host
suite totals: bridge 54 passed/3 failed, pointer 28 passed/1 failed, QuickView
at 175% 95 passed/1 failed. New feature and static packaging checks passed.
