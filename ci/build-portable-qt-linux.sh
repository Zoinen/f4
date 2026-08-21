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
# GitHub-hosted runners occasionally leave the bionic mirror connection
# half-open.  Without explicit network/lock limits apt can sit silently until
# the Actions step is killed after a long idle period.  Keep bootstrap
# bounded and retryable so a transient mirror issue cannot consume the whole
# CI run.
apt_args=(
    -o Acquire::Retries=3
    -o Acquire::http::Timeout=30
    -o Acquire::https::Timeout=30
    -o Acquire::Languages=none
    -o DPkg::Lock::Timeout=60
    -o Dpkg::Use-Pty=0
)
apt_with_timeout() {
    # apt may leave an unresponsive helper behind after SIGTERM (notably while
    # parsing old bionic package indexes).  --kill-after makes the timeout a
    # hard upper bound instead of letting the Actions job consume its budget.
    timeout --foreground --kill-after=30s 10m apt-get "${apt_args[@]}" "$@"
}

echo "Installing Ubuntu 18.04 bootstrap packages"
apt_with_timeout update
apt_with_timeout install -y --no-install-recommends \
    autoconf automake bison build-essential ca-certificates curl flex git libssl-dev patchelf \
    gnupg gperf libtool m4 patch pkg-config software-properties-common xz-utils
ppa_added=0
for attempt in 1 2 3; do
  if add-apt-repository -y ppa:ubuntu-toolchain-r/test; then
    ppa_added=1
    break
  fi
  if [[ "$attempt" -lt 3 ]]; then
    sleep $((attempt * 15))
  fi
done
if [[ "$ppa_added" -ne 1 ]]; then
  echo "Unable to add the Ubuntu toolchain PPA after 3 attempts" >&2
  exit 1
fi
apt_with_timeout update
apt_with_timeout install -y --no-install-recommends gcc-11 g++-11

export UV_INSTALL_DIR=/usr/local/bin
curl -LsSf https://astral.sh/uv/install.sh | sh
uv venv --python 3.12 /opt/f4-build-venv
# The current x86_64 CMake wheel still supports the Ubuntu 18.04 baseline,
# while the aarch64 wheel for newer CMake releases is manylinux_2_28 and
# fails before Conan can build anything.  The manylinux2014 CMake 3.27 wheel
# is our ARM bootstrap compiler; build the matching 3.31 release from source
# with it before configuring the Qt host.  Qt 6.11's WHOLE_ARCHIVE graph
# requires that newer CMake's link-feature handling.
cmake_version=3.31.6
bootstrap_cmake_version="${cmake_version}"
if [[ "${TARGET_ARCH:-amd64}" == "arm64" ]]; then
    bootstrap_cmake_version=3.27.9
fi
uv pip install --python /opt/f4-build-venv/bin/python \
    'conan==2.29.1' "cmake==${bootstrap_cmake_version}" 'ninja==1.13.0'
export PATH="/opt/f4-build-venv/bin:/opt/go/bin:${PATH}"
export CC=gcc-11
export CXX=g++-11
export CONAN_HOME="${CONAN_HOME:-$PWD/.conan2-portable-linux}"

# Always reduce Conan's cache to finished packages before returning to the
# GitHub runner. This trap runs after a failed conan install as well, so the
# workflow can persist expensive completed Qt dependencies instead of losing
# them with the failed build's source and object trees.
checkpoint_conan_packages() {
    build_status=$?
    trap - EXIT
    set +e
    if [[ -d "${CONAN_HOME}/p" ]] && conan cache clean '*' \
        --source --build --temp --backup-sources
    then
        touch "${CONAN_HOME}/p/.f4-package-cache-ready"
    else
        echo "warning: unable to prepare Conan package checkpoint" >&2
    fi
    exit "${build_status}"
}
trap checkpoint_conan_packages EXIT

TARGET_ARCH="${TARGET_ARCH:-amd64}"
build_dir="qt/host/build-portable-linux-${TARGET_ARCH}"
dist_dir="dist/f4-linux-${TARGET_ARCH}"

cmake_executable="/opt/f4-build-venv/bin/cmake"
if [[ "${TARGET_ARCH}" == "arm64" ]]; then
    cmake_tools_dir="${CONAN_HOME}/.f4-build-tools"
    cmake_install_dir="${cmake_tools_dir}/cmake-${cmake_version}"
    cmake_executable="${cmake_install_dir}/bin/cmake"
    if [[ ! -x "${cmake_executable}" ]]; then
        cmake_archive="${cmake_tools_dir}/cmake-${cmake_version}.tar.gz"
        cmake_source_dir="${cmake_tools_dir}/cmake-${cmake_version}-src"
        cmake_build_dir="${cmake_tools_dir}/cmake-${cmake_version}-build"
        mkdir -p "${cmake_tools_dir}" "${cmake_source_dir}"
        if ! printf '%s  %s\n' \
            '653427f0f5014750aafff22727fb2aa60c6c732ca91808cfb78ce22ddd9e55f0' \
            "${cmake_archive}" | sha256sum --check --status 2>/dev/null
        then
            curl --fail --location --retry 5 --connect-timeout 30 \
                --output "${cmake_archive}.new" \
                "https://github.com/Kitware/CMake/releases/download/v${cmake_version}/cmake-${cmake_version}.tar.gz"
            printf '%s  %s\n' \
                '653427f0f5014750aafff22727fb2aa60c6c732ca91808cfb78ce22ddd9e55f0' \
                "${cmake_archive}.new" | sha256sum --check --status
            mv "${cmake_archive}.new" "${cmake_archive}"
        fi
        if [[ ! -f "${cmake_source_dir}/.f4-source-ready" ]]; then
            tar -xzf "${cmake_archive}" --strip-components=1 -C "${cmake_source_dir}"
            touch "${cmake_source_dir}/.f4-source-ready"
        fi
        /opt/f4-build-venv/bin/cmake -S "${cmake_source_dir}" -B "${cmake_build_dir}" \
            -G 'Unix Makefiles' -DCMAKE_BUILD_TYPE=Release \
            -DCMAKE_INSTALL_PREFIX="${cmake_install_dir}"
        /opt/f4-build-venv/bin/cmake --build "${cmake_build_dir}" --parallel 4
        /opt/f4-build-venv/bin/cmake --install "${cmake_build_dir}"
    fi
fi

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
        -c "tools.cmake:cmake_program=${cmake_executable}" \
        -c tools.system.package_manager:mode=install \
        -c tools.system.package_manager:sudo=False \
        --output-folder="${build_dir}"
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
touch "${build_dir}/.f4-conan-ready"
touch "$baseline_marker"

bash ci/build-qwindowkit.sh "$PWD/${build_dir}" Release static
"${cmake_executable}" -S qt/host -B "${build_dir}" -G Ninja \
    -DCMAKE_TOOLCHAIN_FILE="$PWD/${build_dir}/conan_toolchain.cmake" \
    -DCMAKE_BUILD_TYPE=Release \
    -DCMAKE_PREFIX_PATH="$PWD/build/qwindowkit-install" \
    -DQWindowKit_DIR="$PWD/build/qwindowkit-install/lib/cmake/QWindowKit" \
    -DBUILD_TESTING=ON -DUSE_QWK=ON -DF4_PORTABLE_STATIC=ON
# Keep the glibc-baseline runner deterministic.  Some hosted Linux images
# expose a very large virtual CPU count; letting Ninja use all of it can
# starve Qt's long-running AUTOMOC/moc --collect-json jobs and leave the job
# alive without progress.  Four workers still parallelize the native build
# without oversubscribing the runner.
"${cmake_executable}" --build "${build_dir}" --config Release --parallel 4
export QML_IMPORT_PATH="$PWD/${build_dir}/ZoinGallery:$PWD/${build_dir}/qml"
export QML2_IMPORT_PATH="$PWD/${build_dir}/ZoinGallery:$PWD/${build_dir}/qml"
ctest --test-dir "${build_dir}" -C Release --output-on-failure \
    -R '^(F4|QtShellController|WindowGeometryPersistence)'

host="$PWD/${build_dir}/bin/Release/f4-qt-host"
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
echo "Embedded Qt payload generated"
go test -tags f4_embedded_qt_host \
    -run 'TestMaterializeEmbeddedQtHost|TestGeneratedEmbeddedQtHostPayload' .
echo "Embedded Qt payload tests passed"
mkdir -p "${dist_dir}"
echo "Building static Go launcher"
# Keep the launcher independent of the host libc; the resulting artifact is
# audited for zero dynamic dependencies below.
CGO_ENABLED=0 GOOS=linux GOARCH="${TARGET_ARCH}" go build -trimpath \
    -buildmode=exe \
    -tags f4_embedded_qt_host \
    -ldflags='-s -w' \
    -o "${dist_dir}/f4" .
# Go 1.26 may emit an otherwise-unused PT_INTERP even for a CGO-free internal
# link.  The launcher has no DT_NEEDED entries; remove that inert header so the
# portable artifact meets the explicit no-interpreter contract.
if readelf -l "${dist_dir}/f4" | grep -q 'INTERP'; then
    echo "Removing Go launcher PT_INTERP"
    python ci/remove-elf-interpreter.py "${dist_dir}/f4"
fi
# The cgo-free FFI implementation uses Go's cgo_import_dynamic metadata for
# optional plugin calls.  The embedded Qt launcher never exercises that path;
# remove the metadata-only system-library edges so the distributed launcher
# remains a genuinely self-contained ELF.  Keep the final audit below as the
# guard against any new dynamic dependency.
for runtime_lib in libc.so.6 libdl.so.2 libpthread.so.0; do
    if readelf -d "${dist_dir}/f4" | grep -Fq "Shared library: [$runtime_lib]"; then
        echo "Removing optional Go launcher DT_NEEDED $runtime_lib"
        patchelf --remove-needed "$runtime_lib" "${dist_dir}/f4"
    fi
done
echo "Auditing static Go launcher"
bash ci/audit-static-go-linux.sh "${dist_dir}/f4"

artifact_files="$(find "${dist_dir}" -maxdepth 1 -type f -printf '%f\n')"
if [[ "${artifact_files}" != "f4" ]]; then
    echo "error: portable Linux artifact must contain only the Go launcher" >&2
    printf '%s\n' "${artifact_files}" >&2
    exit 1
fi
