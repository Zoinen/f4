#!/usr/bin/env bash

set -euo pipefail

repo_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
build_type="${F4_QT_BUILD_TYPE:-RelWithDebInfo}"
host_build_dir="${F4_GALLERY_BUILD_DIR:-$repo_dir/qt/host/build-gallery-editable-final}"
qwindowkit_prefix="${QWINDOWKIT_PREFIX:-$repo_dir/build/qwindowkit-install}"
f4_bin="$repo_dir/f4"
qt_host_base="$host_build_dir/bin/$build_type/f4-qt-host"
qt_host="$qt_host_base"
if [[ "$(uname -s)" == "Darwin" ]]; then
    qt_host="$qt_host_base.app/Contents/MacOS/f4-qt-host"
fi

require_file() {
    local path="$1"
    local description="$2"
    if [[ ! -e "$path" ]]; then
        echo "error: $description was not found at $path" >&2
        exit 1
    fi
}

command -v cmake >/dev/null || {
    echo "error: cmake is required" >&2
    exit 1
}
command -v go >/dev/null || {
    echo "error: go is required" >&2
    exit 1
}

require_file "$repo_dir/third_party/ZoinGallery/CMakeLists.txt" \
    "initialized ZoinGallery Git submodule"
require_file "$host_build_dir/CMakeCache.txt" \
    "configured f4 Qt host build"
require_file "$host_build_dir/conan_toolchain.cmake" \
    "f4 Qt host Conan toolchain"

qwindowkit_config="$qwindowkit_prefix/lib/cmake/QWindowKit/QWindowKitConfig.cmake"
if [[ ! -f "$qwindowkit_config" && -z "${QWINDOWKIT_PREFIX:-}" ]]; then
    echo "Building QWindowKit with the f4 Qt host settings..."
    "$repo_dir/ci/build-qwindowkit.sh" "$host_build_dir" "$build_type"
fi
require_file "$qwindowkit_config" "QWindowKit package configuration"

echo "Configuring f4 Qt host with QWindowKit..."
cmake -S "$repo_dir/qt/host" -B "$host_build_dir" \
    -DCMAKE_TOOLCHAIN_FILE="$host_build_dir/conan_toolchain.cmake" \
    -DCMAKE_BUILD_TYPE="$build_type" \
    -DCMAKE_PREFIX_PATH="$qwindowkit_prefix" \
    -DQWindowKit_DIR="$qwindowkit_prefix/lib/cmake/QWindowKit" \
    -DUSE_QWK=ON \
    -U ZoinGallery_DIR

if ! grep -q '^USE_QWK:BOOL=ON$' "$host_build_dir/CMakeCache.txt"; then
    echo "error: Qt host configuration did not enable QWindowKit" >&2
    exit 1
fi

echo "Building f4 Qt host..."
cmake --build "$host_build_dir" --config "$build_type" --parallel

echo "Building f4 Go core..."
(cd "$repo_dir" && go build -o "$f4_bin" .)

require_file "$qt_host" "f4 Qt host executable"
if [[ ! -x "$qt_host" ]]; then
    echo "error: Qt host is not executable: $qt_host" >&2
    exit 1
fi

if [[ -f "$host_build_dir/conanrun.sh" ]]; then
    # Conan supplies the shared Qt/codec runtime used by the host and bundled
    # ZoinGallery submodule.
    source "$host_build_dir/conanrun.sh"
fi

export F4_EXT_UI_PATH="$qt_host"

echo "Launching f4 with ZoinGallery panel host: $qt_host"
exec "$f4_bin" --gui=qt --attached "$@"
