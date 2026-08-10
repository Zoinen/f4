#!/usr/bin/env bash
set -euo pipefail

qt_root="${1:?usage: build-qwindowkit.sh <qt-root>}"
build_type="${2:-RelWithDebInfo}"

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
qwk_source="${repo_root}/build/qwindowkit-src"
qwk_build="${repo_root}/build/qwindowkit-build"
qwk_install="${repo_root}/build/qwindowkit-install"
qt_version="$(basename "$(dirname "${qt_root}")")"
qwk_cxx_flags=""
qwk_platform_args=()

if [ "$(uname -s)" = "Darwin" ]; then
    qwk_platform_args+=("-DCMAKE_OSX_DEPLOYMENT_TARGET=13.0")
fi

for include_dir in \
    "${qt_root}/include/QtQml/${qt_version}" \
    "${qt_root}/include/QtQml/${qt_version}/QtQml"
do
    if [ -d "${include_dir}" ]; then
        qwk_cxx_flags="${qwk_cxx_flags} -isystem ${include_dir}"
    fi
done

rm -rf "${qwk_source}" "${qwk_build}" "${qwk_install}"
git clone --recursive --branch main https://github.com/stdware/qwindowkit.git "${qwk_source}"

cmake -S "${qwk_source}" -B "${qwk_build}" -G Ninja \
    -DCMAKE_BUILD_TYPE="${build_type}" \
    -DCMAKE_PREFIX_PATH="${qt_root}" \
    -DCMAKE_CXX_FLAGS="${qwk_cxx_flags}" \
    -DCMAKE_INSTALL_PREFIX="${qwk_install}" \
    -DQWINDOWKIT_BUILD_QUICK=TRUE \
    -DQWINDOWKIT_BUILD_WIDGETS=FALSE \
    -DQWINDOWKIT_BUILD_EXAMPLES=FALSE \
    -DQWINDOWKIT_BUILD_DOCUMENTATIONS=FALSE \
    "${qwk_platform_args[@]}"

cmake --build "${qwk_build}" --parallel
cmake --install "${qwk_build}"

qwk_cmake_dir=""
for candidate in \
    "${qwk_install}/lib/cmake/QWindowKit" \
    "${qwk_install}/lib64/cmake/QWindowKit"
do
    if [ -f "${candidate}/QWindowKitConfig.cmake" ]; then
        qwk_cmake_dir="${candidate}"
        break
    fi
done

test -n "${qwk_cmake_dir}"
grep -R "QWindowKit::Quick" "${qwk_cmake_dir}"
