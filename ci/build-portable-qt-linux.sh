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
    ca-certificates curl git gnupg software-properties-common xz-utils
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
conan install qt/host --build=missing \
    -s:h build_type=Release -s:h compiler.cppstd=20 \
    -s:b build_type=Release -s:b compiler.cppstd=20 \
    -o:h 'qt/*:shared=False' \
    -o:h 'libraw/*:shared=False' \
    -c tools.system.package_manager:mode=install \
    -c tools.system.package_manager:sudo=False \
    --output-folder=qt/host/build-portable-linux

bash ci/build-qwindowkit.sh "$PWD/qt/host/build-portable-linux" Release static
cmake -S qt/host -B qt/host/build-portable-linux -G Ninja \
    -DCMAKE_TOOLCHAIN_FILE="$PWD/qt/host/build-portable-linux/conan_toolchain.cmake" \
    -DCMAKE_BUILD_TYPE=Release \
    -DCMAKE_PREFIX_PATH="$PWD/build/qwindowkit-install" \
    -DQWindowKit_DIR="$PWD/build/qwindowkit-install/lib/cmake/QWindowKit" \
    -DBUILD_TESTING=ON -DUSE_QWK=ON -DF4_PORTABLE_STATIC=ON
cmake --build qt/host/build-portable-linux --config Release --parallel "$(nproc)"
ctest --test-dir qt/host/build-portable-linux -C Release --output-on-failure

host="$PWD/qt/host/build-portable-linux/bin/Release/f4-qt-host"
bash ci/audit-portable-qt-linux.sh "$host" 2.27
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
