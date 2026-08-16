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
remote/VFS catalogs simply disable local preview decoding.

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
