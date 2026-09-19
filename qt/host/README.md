# f4 Qt Host

Before changing or running build, packaging, deployment, release, or CI logic,
read [`docs/PORTABLE_BUILD_POLICY.md`](../../docs/PORTABLE_BUILD_POLICY.md).
It defines the required single-file Linux/Windows and signed-bundle macOS
contracts and their verification gates.

This directory contains the optional Qt/QML sidecar renderer for `f4 --gui=qt`.
The Go core does not link Qt; it starts `f4-qt-host` only when the Qt backend is requested.

Build after initializing the repository's pinned submodules:

```sh
git submodule update --init --recursive
cd qt/host
conan install . --build=missing -s build_type=RelWithDebInfo -s compiler.cppstd=20 --output-folder=build
cd ../..
bash ci/build-qwindowkit.sh "$PWD/qt/host/build" RelWithDebInfo
cd qt/host
cmake -S . -B build -DCMAKE_TOOLCHAIN_FILE=build/conan_toolchain.cmake \
  -DCMAKE_BUILD_TYPE=RelWithDebInfo \
  -DUSE_QWK=ON \
  -DQWindowKit_DIR=../../build/qwindowkit-install/lib/cmake/QWindowKit \
  -DCMAKE_PREFIX_PATH=../../build/qwindowkit-install
cmake --build build --config RelWithDebInfo
```

On macOS, add `-s:h os.version=13.0` to the Conan command. The recipe enforces
that target so Qt and every native dependency match the host's deployment
minimum.

QWindowKit is enabled by default, matching the standalone ZoinGallery build.
The helper clones QWindowKit with submodules from its `main` branch and uses
the same Quick-only settings: widgets, examples, and documentation are off.
The documented commands also pass `-DUSE_QWK=ON` explicitly so an existing
CMake build directory cannot silently retain an old `OFF` cache value.
Pass `-DUSE_QWK=OFF` to CMake only when deliberately building the dummy
window-agent fallback.

## Icon sets

The QML frontend offers **Lucide** (the default) and **System** in the
Appearance dialog. The choice is stored as `QmlIconSet = lucide|system` in the
`[Appearance]` section and is applied live; it does not affect the terminal or
other external UI renderers.

System file icons come from Qt Gui's platform file-icon provider. Qt delegates
that lookup to Finder/NSWorkspace on macOS, the Shell image lists on Windows,
and the current MIME/XDG icon theme on Linux. The host requests each `QIcon`
at the delegate's logical size and device-pixel ratio so native multi-resolution
or vector artwork is selected before it becomes a Qt Quick texture. Missing
theme or remote-file icons fall back to the matching bundled Lucide glyph.

### Bundled icon assets

All other Qt menu and chrome icons use the vendored Lucide set in
`icons/lucide`. The Android, iOS, and Windows locations entries in the disk
menu use the Android Logo, Apple Logo, and Microsoft Windows Logo 2 SVGs in
`icons/streamline`, respectively. These icons were sourced from [Streamline's
OS systems icon collection](https://www.streamlinehq.com/icons/logos-line/os-systems?icon=ico_UvfZHgqO6RKUzd9d).

## ZoinGallery panel

ZoinGallery is the Qt frontend's single in-process panel renderer. The console
UI keeps its Brief, Medium, Detailed, and Wide implementations, but the QML
frontend has no parallel legacy list delegate tree.

ZoinGallery is pinned as `third_party/ZoinGallery`. Initialize it once after
cloning or changing branches:

```sh
git submodule update --init --recursive

cd /path/to/f4/qt/host
conan install . --build=missing \
  -s build_type=RelWithDebInfo -s compiler.cppstd=20 \
  --output-folder=build-gallery
cmake -S . -B build-gallery \
  -DCMAKE_TOOLCHAIN_FILE=build-gallery/conan_toolchain.cmake \
  -DCMAKE_BUILD_TYPE=RelWithDebInfo \
  -DUSE_QWK=ON \
  -DQWindowKit_DIR=../../build/qwindowkit-install/lib/cmake/QWindowKit \
  -DCMAKE_PREFIX_PATH=../../build/qwindowkit-install
cmake --build build-gallery --config RelWithDebInfo
```

`run-f4-gallery.sh` reconfigures the host with `USE_QWK=ON` and builds the
submodule in the same CMake graph. Set `QWINDOWKIT_PREFIX` only when its install
prefix is not `/path/to/f4/build/qwindowkit-install`. Conan supplies Qt and the
codec libraries for both projects; it does not package or locate ZoinGallery.
Every Conan install on macOS must use `-s:h os.version=13.0` so the host,
submodule, and Qt runtime share one deployment ABI.

### CI source contract

The native Qt-host CI checkout initializes submodules recursively. The f4
commit therefore pins the exact ZoinGallery revision without separate
repository variables, SHA inputs, or a private Conan package. CI builds/tests
the combined graph, installs and smoke-tests the relocatable sidecar tree, and
uploads an `f4-qt-<platform>-<arch>` artifact.

`F4GalleryBridge` owns one external-catalog session for each panel. It applies
catalog snapshots only when `catalogRevision` changes and applies cursor and
selection state separately. Local and virtual panels share the same component;
local and VFS images use Go's media broker over an independent connection,
with shared Gallery decode workers and versioned thumbnail caches.

Masonry, Grid and Icon modes automatically preview visible directories when both peers negotiate
`directoryPreviewsV1` together with `panelCatalogRowsV1`. Folder rows carry
`directorySource` enumeration authority, separate from image byte authority.
`enumerateDirectoryPreview` retains the first 200 non-directory files in VFS
delivery order, including unsupported and hidden files. Gallery filters supported
images, naturally sorts them, and evenly samples at most 16 names.
`resolveDirectoryPreview` validates those names and their captured metadata before
granting ordinary image descriptors under an independent preview lease. Neither
operation recurses. Providers may internally buffer more than 200 entries.

Directory work has a separate lane (two calls globally, one per non-local VFS
session), a 30-second deadline, and keeps its worker and VFS lease until the
underlying call returns. Listing and preview leases use the media connection's
existing acknowledgement, release and reconnect protocol. Main-catalog commits
do not release preview-owned image references.

The native Settings category **Gallery & cache** controls the image and folder
preview caches (Off / On / Cache only), layout resize animation and display-color
conversion. It shows the detected target color space and enumerates the compiled
decoder registry in priority order with each library and its supported formats.
The page uses the shared settings fields, drop-downs and checkboxes. Disk usage
is refreshed asynchronously while the page is open, without disabling actions
or changing their labels. Explicit maintenance queues behind an outstanding
usage poll on the cache worker. Clearing removes
cached images and retained folder collages; currently visible images can refill
the cache.

`Cache/diskLimitMiB` defaults to 512 and accepts 64–65536 MiB, including 4096 MiB
(4 GiB). It bounds the combined durable and session caches of derived images and
their metadata. `Cache/location` is an optional absolute directory; changes take
effect on restart and do not move or delete the previous location. An empty value
uses the existing namespaced cache root shown on the page. Preferences are stored
in the Qt host's QSettings, alongside its existing Gallery cache preferences.

### Image Quick View

Ctrl+Q uses the same `GalleryViewerHost` / `ZG.GalleryViewer` as Enter for images.
`ViewerCoordinator` keeps its source panel separate from its Quick View destination
and tracks closed, docked, expanding, full, and collapsing presentation states.
The Loader, session, decode requests and caches survive dock/full transitions.
Only full-view progress fades shell controls and substitutes the workspace title.
Docked geometry is clipped to the complete alternate-panel slot, with no opening
animation. Fit responds to the viewport; custom zoom retains its absolute scale
and center image point subject to viewport bounds and physical-pixel rounding.

The **Quick View** group in **Gallery & Cache** stores F4-owned Qt preferences
`QuickView/useBuiltinF4Viewer` (false) and `QuickView/previewOnHover` (true).
Apply takes effect immediately. The built-in choice restores Go image decoding,
bounded PNG serialization and the QML Image. Native image presentation bypasses
that work and uses the source catalog's authenticated media descriptors. Errors
stay in the selected renderer. Text, hex, directories and provider previews
continue to use the existing F4 surfaces.

`QuickViewController` configures each shell with `quickView.configure` before
new alternate panels load. `quickView.preview` carries Quick View and source
panel identities, catalog revision, stable entry ID and increasing generation.
Hover changes the presented entry without cursor, selection or focus changes.
Leaving the list and keyboard input clear the override; pointer movement can
establish it again. Dragging, blocking overlays and full presentation suppress
hover. Non-image hover work is cancellable and obsolete results are discarded.

Click or Tab focuses the docked viewer. Its navigation commits the source cursor
without activating the source panel. Enter expands, Escape focuses the source
list, and Ctrl+Q closes Quick View. Enter on the source list always opens its real
cursor item, even when another image is hovered. Full-view close gestures return
to Quick View while its gallery destination exists; changing the renderer or
removing that destination restores the ordinary full-view lifecycle.

GalleryRuntime shares an 8 MiB RAM cache of compact folder snapshots across
panels. Each snapshot stores Unknown / Empty / HasImages, up to 16 selected
children, dimensions and thumbnail-cache keys; it contains no read authority or
leases. Incoming folder states are restored in one RAM pass before catalog
publication. The lightweight original folder frame and filename paint from the
row snapshot in the first frame, independently of deferred ImageFile creation.
The image grid is created separately after geometry is committed.
The small frame is retained with a recycled viewport delegate; its grid is
destroyed while the delegate displays a file or a mode without folder previews.
Photo-to-folder reassignment commits the new identity and square geometry before
activating the frame. Known previews also suppress hidden fallback icon/text work.

Each panel retains at most 32 inactive child models, including previous paths.
Eviction releases those models without discarding compact snapshots or shared
pixels. Recreated models restore geometry and use ready RAM pixels while reads
are suspended; fresh directory authority enables background revalidation.
Leaving the viewport cancels work and releases preview leases. Directory
admission resumes on completion of a worker, without polling. Disk image and
metadata formats remain unchanged; folder snapshots last for the application
session and are cleared by the existing cache maintenance action.

Image dimensions, orientation and EXIF also have a shared 16 MiB memory cache.
It uses the same source revision, byte size and weak-authority keys as disk
metadata. Warm catalog rows receive these values before model publication,
including deferred and sparse rows. A supplied image-source byte size is usable
even while display metadata is deferred. Later path/timestamp enrichment of
the same resource preserves its geometry and pixels. Cold disk lookups run on
the bounded Gallery cache worker; they do not block the Qt event loop. Missing
or invalid EXIF orientation is normalized to unrotated before caching.

Folder appearance does not wait for new QML image-ready notifications. Clearing image
caches also clears the memory metadata tier. The opt-in media trace reports
`qt.gallery.metadata.restored`, `qt.gallery.metadata_cache_batch` (memory), and
`qt.gallery.metadata_cache_disk` alongside thumbnail hit/admission events.

Repeated metadata notifications with identical cell geometry preserve the
Masonry row index and its revision. They must not cancel a PageUp/PageDown
animation or discard its reversible page history. Actual geometry changes
still invalidate that history. `F4_NAV_BENCHMARK_TRACE=1` also enables passive
QML navigation diagnostics without configuring an automatic benchmark target:
`navigation.page.planned`, `navigation.page.geometry-invalidated`, and
`navigation.scroll.running-changed` include the cursor, viewport and destination.

The outer folder tile keeps its mode's geometry (square in Masonry). Its decorative, aspect-fit grid uses 1, 4,
9 or 16 cells at widths below 80, 150, 300 or at least 300 logical pixels.
Only displayed cells request thumbnail pixels. Folder selection, activation,
dragging and context menus remain owned by the outer panel. Empty results clear
previews; failed refreshes keep the last successful display. Refresh, re-entry,
navigation and existing file-operation refreshes drive freshness. Decoded image
caches remain shared across panels.
The existing opt-in media timing trace includes `directory.*` lease/admission
events and `qt.directory.*` completion events.
`qt.directory.snapshots.restored` records the batch RAM time and each folder's
state. Requests, enumerate/resolve replies and state publications carry folder
identity and navigation/catalog correlation. QML `directory.appearance.changed`
events can be matched to the next `qt.frame.sync.begin` / `qt.frame.end` pair.
Use separate `F4_MEDIA_TIMING_QT_OUTPUT` and `F4_NAV_BENCHMARK_QT_OUTPUT` paths to
avoid mixing independently buffered writers. The automatic navigation runner
accepts `F4_NAV_BENCHMARK_LAYOUT=masonry` and `F4_NAV_BENCHMARK_SETTLE_MS=1500`
alongside its existing target, cycles and warmup options.

The renderer button selects ZoinGallery strategies: Masonry, two- or
three-column column-major layout, Details, uniform Grid, and large Icons.
Layout choice, column count and each strategy's density are saved independently
per panel. Switching strategies preserves the authoritative f4 cursor and
selection without reapplying the catalog.

On Windows and Linux, installation uses Qt's QML/runtime deployment helper.
On macOS, the build additionally creates
`bin/<config>/f4-qt-host.app/Contents/MacOS/f4-qt-host`. The app bundle is what
the Go launcher prefers: Xcode's `actool` compiles the layered Icon Composer
document into adaptive `Assets.car` data plus an `AppIcon.icns` fallback for
older macOS releases. Do not override it with `QGuiApplication::setWindowIcon`;
that flattens the Dock icon and disables system-controlled appearances.

The Conan generate step still stages the relocatable `lib`, `qml`, and
`plugins` tree on macOS. This includes ZoinGallery, Qt QML and platform
plugins, module shaders/assets, codec libraries, and the shared Qt runtime.

Run the host-side bridge test with:

```sh
ctest --test-dir build --output-on-failure
```

Runtime lookup order from Go:

1. `F4_EXT_UI_PATH`
2. on macOS, `f4-qt-host.app/Contents/MacOS/f4-qt-host` next to `f4`
3. a bare host executable next to the `f4` binary as a compatibility fallback
4. the equivalent app-bundle or bare paths below
   `qt/host/build/bin/<config>` for local development

The protocol is a 4-byte big-endian length prefix followed by a MessagePack map.
The host accepts the upstream ExtUI `--f4-ext-*` startup arguments and keeps the older `--f4-qt-*` names as a compatibility fallback. It renders vtui cells in a custom `VtuiGridItem` while using semantic `sdk/extui` scenes for QML-native panels, menus, dialogs, document surfaces, and future sibling modules such as a possible editable-package integration with `ZoinGallery`.

Protocol 3 installs one app-schema scene at semantic version 4 and then advances
its monotonic `revision` with atomic `scene_patch` messages. Root and shell map
patches are bounded; panel `state_update` operations never contain catalog rows,
`selection_delta` carries only changed stable IDs and source indexes, and the
rare `selection_replace` is reserved for journal overflow or recovery.
`catalog_replace` is the only operation allowed to carry every row, and is sent
only when that panel's `catalogRevision` changes.

The Qt host validates the complete patch and every base revision before
committing any part of it. It then emits targeted menu, command-line, panel-state,
or catalog signals. In particular, opening or closing a menu does not emit
`sceneChanged` and does not copy either panel catalog. A complete scene is the
bootstrap and conservative recovery path for unsupported structural changes;
ordinary redraws, cursor movement, selection, menus, and bounded shell state use
the incremental path.

Menu items may carry an optional semantic `icon` name. Empty icons are omitted
from MessagePack; opening a drive menu therefore adds only its bounded menu rows
and icon names to the `menus` root patch, never either file-panel catalog.
