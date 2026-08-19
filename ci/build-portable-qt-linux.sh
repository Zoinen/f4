#!/usr/bin/env bash
set -euo pipefail

if [[ "$(id -u)" != 0 ]]; then
    echo "error: run this baseline builder as root inside Ubuntu 18.04" >&2
    exit 2
fi
if ! grep -q 'Ubuntu 18.04' /etc/os-release; then
    echo "error: glibc 2.27 contract requires the Ubuntu 18.04 build container" >&2
    exit 2
fi

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y --no-install-recommends \
    autoconf automake bison build-essential ca-certificates curl flex git patchelf \
    gnupg gperf libtool m4 patch pkg-config software-properties-common xz-utils
add-apt-repository -y ppa:ubuntu-toolchain-r/test
apt-get update
apt-get install -y --no-install-recommends gcc-11 g++-11

export UV_INSTALL_DIR=/usr/local/bin
curl -LsSf https://astral.sh/uv/install.sh | sh
uv venv --python 3.12 /opt/f4-build-venv
uv pip install --python /opt/f4-build-venv/bin/python \
    'conan==2.29.1' 'cmake==3.31.6' 'ninja==1.13.0'
export PATH="/opt/f4-build-venv/bin:/opt/go/bin:${PATH}"
export CC=gcc-11
export CXX=g++-11
export CONAN_HOME="${CONAN_HOME:-$PWD/.conan2-portable-linux}"

git config --global --add safe.directory "$PWD"
conan profile detect --force

# www.freedesktop.org rejects GitHub-hosted runners with HTTP 418 for this
# release URL. MacPorts mirrors the byte-identical upstream archive (the
# Conan Center SHA-256 remains authoritative), so export the unchanged recipe
# with only its transport URL replaced.
conan download fontconfig/2.15.0 --only-recipe --remote=conancenter
fontconfig_recipe="$(conan cache path fontconfig/2.15.0)"
fontconfig_recipe_copy="$(mktemp -d /tmp/f4-fontconfig-recipe.XXXXXX)"
cp "${fontconfig_recipe}/conanfile.py" \
    "${fontconfig_recipe}/conandata.yml" \
    "${fontconfig_recipe_copy}/"
sed -i \
    's#https://www.freedesktop.org/software/fontconfig/release/#https://distfiles.macports.org/fontconfig/#' \
    "${fontconfig_recipe_copy}/conandata.yml"
grep -q 'https://distfiles.macports.org/fontconfig/fontconfig-2.15.0.tar.xz' \
    "${fontconfig_recipe_copy}/conandata.yml"
conan export "${fontconfig_recipe_copy}" --name=fontconfig --version=2.15.0

# Conan package IDs do not encode the glibc build baseline. On a cold cache,
# rebuild every target-side native package even if Conan Center offers a GCC
# 11 binary; build-only tools are allowed from the remote when they run on
# 2.27. m4 is the exception: its remote binary requires newer glibc, so rebuild
# it too. Once this container has completed successfully, persist a marker in
# the cached package graph and let Conan reuse those baseline-built packages.
target_packages=(
    brotli bzip2 double-conversion elfutils expat fontconfig freetype glib
    harfbuzz icu jasper lcms libde265 libffi libheif libiconv libjpeg-turbo
    libmount libpng libraw libselinux libtiff libwebp libxml2 md4c msgpack-cxx
    openssl pcre2 qt sqlite3 wayland xkbcommon xz_utils zlib zstd
)
baseline_marker="$CONAN_HOME/p/.f4-glibc-2.27-ready"
conan_build_args=(--build=missing)
if [[ ! -f "$baseline_marker" ]]; then
    conan_build_args+=(--build='m4/*')
    for package in "${target_packages[@]}"; do
        conan_build_args+=("--build=${package}/*")
    done
    echo "No glibc 2.27 package marker found; forcing baseline rebuild"
else
    echo "Reusing cached glibc 2.27 Conan package graph"
fi

for attempt in 1 2 3; do
    if conan install qt/host "${conan_build_args[@]}" \
        -s:h build_type=Release -s:h compiler.cppstd=gnu20 \
        -s:b build_type=Release -s:b compiler.cppstd=gnu20 \
        -o:h 'qt/*:shared=False' \
        -o:h 'qt/*:qtwayland=True' \
        -o:h 'qt/*:with_egl=True' \
        -o:h 'qt/*:with_libjpeg=libjpeg-turbo' \
        -o:h 'qt/*:disabled_features=quickcontrols2_fusion quickcontrols2_imagine quickcontrols2_material quickcontrols2_universal quickcontrols2_fluentwinui3 quickcontrols2_stylekit quickcontrols2_windows' \
        -o:h 'xkbcommon/*:with_wayland=True' \
        -o:h 'libraw/*:shared=False' \
        -c 'tools.build:compiler_executables={"c":"gcc-11","cpp":"g++-11"}' \
        -c tools.system.package_manager:mode=install \
        -c tools.system.package_manager:sudo=False \
        --output-folder=qt/host/build-portable-linux
    then
        break
    fi
    if [[ "${attempt}" == 3 ]]; then
        echo "error: Conan install failed after ${attempt} attempts" >&2
        exit 1
    fi
    retry_delay=$((attempt * 15))
    echo "warning: Conan install attempt ${attempt} failed; retrying in ${retry_delay}s" >&2
    sleep "${retry_delay}"
done

# The workflow may still fail later while linking or testing f4.  Record that
# Conan completed so Actions can preserve this expensive static Qt graph even
# when the cache post-step would otherwise be skipped after a job failure.
touch qt/host/build-portable-linux/.f4-conan-ready
touch "$baseline_marker"

bash ci/build-qwindowkit.sh "$PWD/qt/host/build-portable-linux" Release static
cmake -S qt/host -B qt/host/build-portable-linux -G Ninja \
    -DCMAKE_TOOLCHAIN_FILE="$PWD/qt/host/build-portable-linux/conan_toolchain.cmake" \
    -DCMAKE_BUILD_TYPE=Release \
    -DCMAKE_PREFIX_PATH="$PWD/build/qwindowkit-install" \
    -DQWindowKit_DIR="$PWD/build/qwindowkit-install/lib/cmake/QWindowKit" \
    -DBUILD_TESTING=ON -DUSE_QWK=ON -DF4_PORTABLE_STATIC=ON
# Keep the glibc-baseline runner deterministic.  Some hosted Linux images
# expose a very large virtual CPU count; letting Ninja use all of it can
# starve Qt's long-running AUTOMOC/moc --collect-json jobs and leave the job
# alive without progress.  Four workers still parallelize the native build
# without oversubscribing the runner.
cmake --build qt/host/build-portable-linux --config Release --parallel 4
export QML_IMPORT_PATH="$PWD/qt/host/build-portable-linux/ZoinGallery:$PWD/qt/host/build-portable-linux/qml"
export QML2_IMPORT_PATH="$PWD/qt/host/build-portable-linux/ZoinGallery:$PWD/qt/host/build-portable-linux/qml"
ctest --test-dir qt/host/build-portable-linux -C Release --output-on-failure \
    -R '^(F4|QtShellController|WindowGeometryPersistence)'

host="$PWD/qt/host/build-portable-linux/bin/Release/f4-qt-host"
# Smoke-test the linked host before ELF metadata cleanup. Ubuntu 18.04 ships
# patchelf 0.9, which removes DT_NEEDED but leaves version-needed metadata.
set +e
QT_QPA_PLATFORM=offscreen QSG_RHI_BACKEND=software "$host" \
    --f4-ext-connect=127.0.0.1:1 --f4-ext-nonce=ci-smoke \
    --f4-ext-cols=100 --f4-ext-rows=30 >/tmp/f4-qt-host-smoke.log 2>&1
smoke_status=$?
set -e
if [[ "${smoke_status}" != 2 ]]; then
    cat /tmp/f4-qt-host-smoke.log >&2
    echo "error: disconnected Qt host returned ${smoke_status}, expected 2" >&2
    exit 1
fi
if grep -Eq 'QQmlApplicationEngine failed to load component|Could not find the Qt platform plugin' /tmp/f4-qt-host-smoke.log; then
    cat /tmp/f4-qt-host-smoke.log >&2
    echo "error: static Qt host could not load an embedded QML or platform plugin" >&2
    exit 1
fi

# Conan's imported Qt interfaces can append a redundant dynamic libstdc++
# item after the compiler driver's static-runtime selection. Remove only that
# metadata entry after the working-host smoke test; the static archive remains
# part of the executable and the audit checks the final ELF dependency graph.
needed_runtime="$(readelf -d "$host" | sed -n 's/.*Shared library: \[\([^]]*\)\].*/\1/p')"
if printf '%s\n' "$needed_runtime" | grep -Fxq 'libstdc++.so.6'; then
    if nm -D "$host" | grep -Eq ' U (_Z|GLIBCXX|CXXABI)'; then
        echo "error: Qt host has unresolved C++ symbols; refusing to strip libstdc++.so.6" >&2
        exit 1
    fi
    patchelf --remove-needed libstdc++.so.6 "$host"
    if readelf -d "$host" | grep -Fq 'Shared library: [libstdc++.so.6]'; then
        echo "error: patchelf failed to remove libstdc++.so.6" >&2
        exit 1
    fi
fi
if readelf -d "$host" | grep -Eq '\((RPATH|RUNPATH)\)'; then
    patchelf --remove-rpath "$host"
    if readelf -d "$host" | grep -Eq '\((RPATH|RUNPATH)\)'; then
        echo "error: patchelf failed to remove Qt host RPATH/RUNPATH" >&2
        exit 1
    fi
fi
bash ci/audit-portable-qt-linux.sh "$host" 2.27

python ci/package-embedded-qt-host.py "$host"
go test -tags f4_embedded_qt_host \
    -run 'TestMaterializeEmbeddedQtHost|TestGeneratedEmbeddedQtHostPayload' .
mkdir -p dist/f4-linux-amd64
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
    -tags f4_embedded_qt_host -ldflags='-s -w' \
    -o dist/f4-linux-amd64/f4 .
bash ci/audit-static-go-linux.sh dist/f4-linux-amd64/f4

artifact_files="$(find dist/f4-linux-amd64 -maxdepth 1 -type f -printf '%f\n')"
if [[ "${artifact_files}" != "f4" ]]; then
    echo "error: portable Linux artifact must contain only the Go launcher" >&2
    printf '%s\n' "${artifact_files}" >&2
    exit 1
fi
