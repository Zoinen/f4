# f4 Qt Host

This directory contains the optional Qt/QML sidecar renderer for `f4 --gui=qt`.
The Go core does not link Qt; it starts `f4-qt-host` only when the Qt backend is requested.

Build:

```sh
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

The gallery panel is an optional, in-process QML module. A default build has no
ZoinGallery dependency and continues to render every panel as a file list.

For sibling development, register the ZoinGallery recipe as an editable Conan
package, then enable the host option:

```sh
cd /path/to/ZoinGallery
conan install . --build=missing \
  -s build_type=RelWithDebInfo -s compiler.cppstd=20 \
  -o '&:build_standalone=False'
cmake -S . -B build -G Ninja \
  -DCMAKE_TOOLCHAIN_FILE=build/conan_toolchain.cmake \
  -DCMAKE_BUILD_TYPE=RelWithDebInfo \
  -DZOIN_BUILD_STANDALONE=OFF -DUSE_QWK=OFF
cmake --build build --config RelWithDebInfo
conan editable add . --name=zoingallery --version=0.1.0

cd /path/to/f4/qt/host
conan install . --build=missing \
  -s build_type=RelWithDebInfo -s compiler.cppstd=20 \
  -o "&:with_zoingallery=True" --output-folder=build-gallery
cmake -S . -B build-gallery \
  -DCMAKE_TOOLCHAIN_FILE=build-gallery/conan_toolchain.cmake \
  -DCMAKE_BUILD_TYPE=RelWithDebInfo \
  -DF4_WITH_ZOINGALLERY=ON -DUSE_QWK=ON \
  -DQWindowKit_DIR=../../build/qwindowkit-install/lib/cmake/QWindowKit \
  -DCMAKE_PREFIX_PATH=../../build/qwindowkit-install
cmake --build build-gallery --config RelWithDebInfo
```

For the sibling checkout layout, `run-f4-gallery.sh` reconfigures the f4 host
with `USE_QWK=ON` before every build and fails if the real QWindowKit package
is unavailable. Set `QWINDOWKIT_PREFIX` only when its install prefix is not
`/path/to/f4/build/qwindowkit-install`. If that default installation is
missing, the launcher builds it with the same Quick-only settings first.

The ZoinGallery recipe deliberately maps editable artifacts to its local
`build` directory, so run its `conan install` without an explicit Conan output
folder. Remove the mapping with
`conan editable remove -r 'zoingallery/0.1.0'` when switching back to a pinned
package. Release/CI builds create that pinned package first with:

```sh
conan create /path/to/ZoinGallery --build=missing \
  -s build_type=RelWithDebInfo -s compiler.cppstd=20 \
  -o '&:build_standalone=False'
```

Every Conan `install`/`create` command in this workflow needs the same macOS
setting. The f4 host recipe rejects a missing or different target rather than
linking newer dependency binaries into a macOS 13 host.

### CI source contract

The native Qt-host CI job deliberately does not assume that a private Conan
remote exists. It creates the pinned `zoingallery/0.1.0` package from an exact
source revision. Configure these GitHub repository variables before enabling
the required check or creating a release:

- `ZOINGALLERY_SOURCE_REPOSITORY`: the `owner/repository` containing the
  ZoinGallery Conan recipe.
- `ZOINGALLERY_SOURCE_SHA`: the full 40-character commit SHA to package.

For a private source repository, also configure the optional
`ZOINGALLERY_SOURCE_TOKEN` secret with read access. Manual workflow dispatches
can override the repository and SHA inputs. The job verifies that checkout
`HEAD` exactly matches the supplied SHA, builds/tests both list-only and
Gallery-enabled host configurations, installs and smoke-tests the relocatable
sidecar tree, and uploads an `f4-qt-<platform>-<arch>` artifact. A missing or
non-immutable source setting fails with an explicit configuration error.

The Conan option writes `F4_WITH_ZOINGALLERY` into the generated toolchain. The
enabled build requires the exported `ZoinGallery::Core` and
`ZoinGallery::Qml` targets; a missing or ABI-incompatible package fails during
configuration instead of silently producing a partial gallery build.

`F4GalleryBridge` owns one external-catalog session for each panel. It applies
catalog snapshots only when `catalogRevision` changes and applies cursor and
selection state separately. The QML panel is selected only when the semantic
panel requests `presentation: gallery`, reports `sourceKind: local`, and is
`previewCapable`; archives and remote VFS panels keep the list while retaining
their requested presentation in the Go model.

The renderer button in each panel header keeps the existing Brief, Medium,
Detailed and Wide choices and adds the reusable ZoinGallery strategies:
Masonry, two- or three-column column-major layout, Details, uniform Grid and
large Icons. Layout choice, column count and each strategy's density are saved
independently per panel. Switching strategies preserves the authoritative f4
cursor/selection and does not reapply the catalog; Details sorting is routed
back through the same semantic panel actions as the native list header.

Gallery components are loaded by resource URL, so the list-only QML module does
not import ZoinGallery. On Windows and Linux, installation uses Qt's QML/runtime
deployment helper. Qt does not support that helper for a non-bundle executable
on macOS, while f4 requires the host to remain a sibling sidecar. The Conan
generate step therefore stages a relocatable `lib`, `qml`, and `plugins` tree
on macOS; `cmake --install` places it beside `bin/f4-qt-host` and installs a
matching `qt.conf`. This includes the ZoinGallery plugin, Qt QML and platform
plugins, shaders/assets contained in their modules, codec libraries, and the
shared Qt runtime.

Run the host-side bridge test with:

```sh
ctest --test-dir build --output-on-failure
```

Runtime lookup order from Go:

1. `F4_EXT_UI_PATH`
2. a host executable next to the `f4` binary
3. `qt/host/build/bin/<config>/f4-qt-host` for local development

The protocol is a 4-byte big-endian length prefix followed by a MessagePack map.
The host accepts the upstream ExtUI `--f4-ext-*` startup arguments and keeps the older `--f4-qt-*` names as a compatibility fallback. It renders vtui cells in a custom `VtuiGridItem` while using semantic `sdk/extui` scenes for QML-native panels, menus, dialogs, document surfaces, and future sibling modules such as a possible editable-package integration with `ZoinGallery`.
