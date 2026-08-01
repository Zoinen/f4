#!/usr/bin/env bash

set -euo pipefail

repo_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
f4_bin="$repo_dir/f4"
qt_dir="$repo_dir/qt/host"
build_dir="$qt_dir/build"
build_type="${F4_QT_BUILD_TYPE:-RelWithDebInfo}"
qt_host="$build_dir/bin/$build_type/f4-qt-host"

cd "$repo_dir"
go build -o "$f4_bin" .

if [[ ! -f "$build_dir/CMakeCache.txt" ]]; then
    command -v conan >/dev/null || {
        echo "error: conan is required to configure the QML host" >&2
        exit 1
    }
    command -v cmake >/dev/null || {
        echo "error: cmake is required to configure the QML host" >&2
        exit 1
    }

    (
        cd "$qt_dir"
        conan install . \
            --build=missing \
            -s "build_type=$build_type" \
            -s compiler.cppstd=20 \
            --output-folder="$build_dir"
        cmake -S . -B "$build_dir" \
            -DCMAKE_TOOLCHAIN_FILE="$build_dir/conan_toolchain.cmake" \
            -DCMAKE_BUILD_TYPE="$build_type"
    )
fi

cmake --build "$build_dir" --config "$build_type"

if [[ ! -x "$qt_host" ]]; then
    echo "error: QML host was not produced at $qt_host" >&2
    exit 1
fi

if [[ -f "$build_dir/conanrun.sh" ]]; then
    # Qt from Conan needs its plugin and dynamic-library paths at runtime.
    source "$build_dir/conanrun.sh"
fi

export F4_EXT_UI_PATH="$qt_host"
exec "$f4_bin" --gui=qt "$@"
