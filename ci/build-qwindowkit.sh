#!/usr/bin/env bash
set -euo pipefail

qt_root="${1:?usage: build-qwindowkit.sh <qt-root> [build-type] [shared|static]}"
build_type="${2:-RelWithDebInfo}"
linkage="${3:-shared}"

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
qwk_source="${repo_root}/build/qwindowkit-src"
qwk_build="${repo_root}/build/qwindowkit-build"
qwk_install="${repo_root}/build/qwindowkit-install"
qwk_qmsetup_host_build="${repo_root}/build/qwindowkit-qmsetup-host-build"
qwk_qmsetup_host_install="${repo_root}/build/qwindowkit-qmsetup-host-install"
qwk_qmsetup_host_config="${qwk_qmsetup_host_install}/lib/cmake/qmsetup/qmsetupConfig.cmake"
qwk_patch="${repo_root}/ci/patches/qwindowkit-default-maximize-hint.patch"
qwk_patch_hash="$(cmake -E sha256sum "${qwk_patch}" | awk '{print substr($1, 1, 16)}')"
qwk_marker="${qwk_install}/.f4-qwindowkit-ready-${linkage}-${build_type}-${qwk_patch_hash}"
qt_version="$(basename "$(dirname "${qt_root}")")"
qwk_cxx_flags=""
qwk_platform_args=()

case "${linkage}" in
    shared)
        qwk_platform_args+=("-DQWINDOWKIT_BUILD_STATIC=OFF")
        ;;
    static)
        qwk_platform_args+=("-DQWINDOWKIT_BUILD_STATIC=ON")
        ;;
    *)
        echo "error: QWindowKit linkage must be 'shared' or 'static'" >&2
        exit 2
        ;;
esac

if [ -f "${qwk_marker}" ] && grep -R "QWindowKit::Quick" "${qwk_install}/lib/cmake/QWindowKit" "${qwk_install}/lib64/cmake/QWindowKit" 2>/dev/null; then
    echo "Reusing cached QWindowKit ${linkage} ${build_type} install"
    exit 0
fi

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

if git -C "${qwk_source}" apply --reverse --check "${qwk_patch}" 2>/dev/null; then
    echo "QWindowKit default maximize-hint fix is already upstream"
else
    git -C "${qwk_source}" apply --check "${qwk_patch}"
    git -C "${qwk_source}" apply "${qwk_patch}"
fi

# QWindowKit falls back to building its qmsetup submodule during the outer
# configure when no host package is discoverable.  That nested build hides its
# compiler log and is especially fragile in a target Qt graph.  Build the tiny
# host-only helper explicitly, like the Windows ARM path does, and point the
# outer project at the resulting package.  The helper is native to the runner;
# QWindowKit itself still uses the target Qt package selected above.
if [ ! -f "${qwk_qmsetup_host_config}" ]; then
    rm -rf "${qwk_qmsetup_host_build}" "${qwk_qmsetup_host_install}"
    qwk_qmsetup_host_flags=()
    if [ "$(uname -s)" = "Linux" ]; then
        # Ubuntu 18.04's glibc still exposes pthread_sigmask from libpthread;
        # newer glibc folds it into libc, which hid this missing link flag on
        # the native ARM VM and on the regular Linux runner.
        qwk_qmsetup_host_flags+=(
            "-DCMAKE_CXX_FLAGS=-pthread"
            "-DCMAKE_EXE_LINKER_FLAGS=-pthread"
        )
    fi
    cmake -S "${qwk_source}/qmsetup" -B "${qwk_qmsetup_host_build}" -G Ninja \
        -DCMAKE_BUILD_TYPE=Release \
        -DCMAKE_INSTALL_PREFIX="${qwk_qmsetup_host_install}" \
        -DCMAKE_INSTALL_LIBDIR=lib \
        -DQMSETUP_STATIC_RUNTIME=ON \
        "${qwk_qmsetup_host_flags[@]}"
    cmake --build "${qwk_qmsetup_host_build}" --target install --parallel
fi
test -f "${qwk_qmsetup_host_config}"
qwk_platform_args+=("-Dqmsetup_DIR=${qwk_qmsetup_host_install}/lib/cmake/qmsetup")

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
touch "${qwk_marker}"
