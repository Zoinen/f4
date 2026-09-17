# Gallery cache and settings

## Scope
Retain folder preview snapshots across navigation, preserving known image geometry
and pixels while authoritative descriptors are refreshed. Add a native Gallery
settings category with cache modes, location/usage/limit/clear, color management,
layout animation, and live decoder registry information. Default disk budget
remains 512 MiB; set this user's preference to 4096 MiB.

## Tasks
- [x] Preserve bounded folder snapshots across navigation and add re-entry regressions.
- [x] Expose runtime cache controls, configurable disk budget/location, color and animation preferences, and decoder metadata.
- [x] Add native settings page with physical-pixel leaf tests and rendered inspection at DPR 1.75.
- [x] Verify relevant Gallery and host tests, document, pin Gallery, package and launch the worktree app with 4 GiB user preference.

Docs: update existing Qt host documentation.

## Verification and delivery
- Navigation regression first failed because its child model was destroyed.
  The completed check retains two decoded images, their published URLs and their
  full sizes through departure, re-entry and background refresh, without another
  image-ready completion or a size-change signal.
- GallerySessionTest: 53 passed, 2 skipped; CTest passed. Persistent derived cache,
  thumbnail memory cache, Masonry layouts and Masonry DPR 1.75 CTests passed.
- Native settings leaf origins and unit-vector transforms at DPR 1.75 passed,
  along with qmlImportsWithoutInstalledQt and compiledHostLoadsItsQmlModule.
  Captures inspected: .diagnostics/gallery-cache-release175.png and
  .diagnostics/gallery-cache-release175.png-decoders.png.
- Go directory/media regressions and embedded payload/extraction tests passed;
  Windows static import audit passed. Existing unrelated full-host-suite failures
  remain recorded in zoin-folder-previews.md.
- Gallery pin: 654243894524226d99ec101268ba57e58c6b5ac9.
- Embedded host SHA-256:
  FFA617E9AA6AE92577252185E575E95E6F992547E6386350FDA62EACAD030791.
- Rebuilt only D:/Code/f4-zoin/f4-zoin.exe, launched directly with --gui=qt,
  and verified that its child uses the freshly extracted matching host.
  QSettings Cache/diskLimitMiB is 4096; the live settings page shows 4.00 GiB.
  Live capture: .diagnostics/gallery-settings-live-4gb.png.

Settings controls follow-up: shared F4 inputs, cache-mode dropdowns, color and
animation checkboxes, and stable action labels during read-only usage polling.
Regression details: ../patches/2026-09-16-02.21.md. Updated DPR capture:
.diagnostics/gallery-controls-175.png.

Warm-navigation follow-up: normalize missing JPEG orientation, restore dimensions
from a shared bounded memory cache before catalog publication, keep disk metadata
reads on the cache worker, and restore folder frames from retained publication
state. Eight live entries restored all 355 dimensions in 73.5–95.3 ms, compared
with 1,249–1,457 ms before the fix. Two fully warm long scroll passes used only
memory thumbnail hits. Regression and profiling details:
../patches/2026-09-16-03.03.md. All six relevant CTests, focused host/static-QML,
Go and embedded-payload tests passed. Updated DPR captures:
.diagnostics/cache-nav-final-175-{masonry,grid,icons}.png.
