# f4 Qt Host

This directory contains the optional Qt/QML sidecar renderer for `f4 --gui=qt`.
The Go core does not link Qt; it starts `f4-qt-host` only when the Qt backend is requested.

Build:

```sh
conan install . --build=missing -s build_type=RelWithDebInfo -s compiler.cppstd=20 --output-folder=build
cmake -S . -B build -DCMAKE_TOOLCHAIN_FILE=build/conan_toolchain.cmake -DCMAKE_BUILD_TYPE=RelWithDebInfo
cmake --build build --config RelWithDebInfo
```

Runtime lookup order from Go:

1. `F4_EXT_UI_PATH`
2. a host executable next to the `f4` binary
3. `qt/host/build/bin/<config>/f4-qt-host` for local development

The protocol is a 4-byte big-endian length prefix followed by a MessagePack map.
The host accepts the upstream ExtUI `--f4-ext-*` startup arguments and keeps the older `--f4-qt-*` names as a compatibility fallback. It renders vtui cells in a custom `VtuiGridItem` while using semantic `sdk/extui` scenes for QML-native panels, menus, dialogs, document surfaces, and future sibling modules such as a possible editable-package integration with `ZoinGallery`.
