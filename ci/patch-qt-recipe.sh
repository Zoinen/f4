#!/usr/bin/env bash
set -euo pipefail

# Start from ConanCenter's recipe on every run so an interrupted build cannot
# accumulate local edits.  The freetype pin applies to both the target and
# native build contexts of Qt's cross-build graph.  ARM64 additionally needs
# the deliberately small native QML and shader-tool exports used by the target build.
target_arch="${1:-}"
qt_recipe_mode="${2:-}"
target_os="${3:-}"
python_command=python
if ! command -v "${python_command}" >/dev/null 2>&1; then
    python_command=python3
fi
if [[ -z "${target_os}" ]]; then
    case "$(uname -s)" in
        Linux*) target_os=Linux ;;
        Darwin*) target_os=Macos ;;
        MINGW*|MSYS*|CYGWIN*) target_os=Windows ;;
    esac
fi
if [[ "${qt_recipe_mode}" == "linux-audio" && "${target_os}" != "Linux" ]]; then
    echo "error: linux-audio Qt recipe mode is valid only for Linux targets" >&2
    exit 2
fi

# A portable ARM job needs both the ARM target Qt package and the native x64
# build-context Qt package.  If a previous run published a complete pair,
# reuse that exact recipe revision: exporting the current patched recipe would
# make Conan treat the already-built binaries as missing and start a full Qt
# source build again.  A fresh graph still falls through to the patching path
# below and will publish a reusable pair for the next run.
if [[ "${target_arch}" == "arm64" ]]; then
    reusable_qt_recipe_revision="$({
        conan list 'qt/6.11.1:*' -r f4-conan --format=json 2>/dev/null || true
    } | "${python_command}" -c '
import json
import sys

try:
    data = json.load(sys.stdin)
    revisions = data["f4-conan"]["qt/6.11.1"]["revisions"]
except (KeyError, TypeError, json.JSONDecodeError):
    raise SystemExit(0)

def has_arch(revision, arch, os_name):
    return any(
        package.get("info", {}).get("settings", {}).get("arch") == arch
        and package.get("info", {}).get("settings", {}).get("os") == os_name
        for package in revision.get("packages", {}).values()
    )

candidates = [
    (revision_id, revision)
    for revision_id, revision in revisions.items()
    if has_arch(revision, "armv8", sys.argv[1])
    and has_arch(revision, "x86_64", sys.argv[1])
]
if candidates:
    revision_id, _ = max(
        candidates,
        key=lambda item: item[1].get("timestamp", 0),
    )
    print(revision_id)
' "${target_os}")"
    if [[ -n "${reusable_qt_recipe_revision}" ]]; then
        conan download "qt/6.11.1#${reusable_qt_recipe_revision}" \
            --only-recipe --remote=f4-conan
        reusable_qt_recipe="$(
            conan cache path "qt/6.11.1#${reusable_qt_recipe_revision}" |
                tail -n 1
        )"
        # A previous interrupted experiment may have published a complete
        # ARM/native pair for a recipe that still omitted the Qt::qsb alias.
        # Only reuse a recipe carrying the current cross-build guard; stale
        # binaries must be rebuilt under the fixed recipe revision.
        if [[ -f "${reusable_qt_recipe}/conanfile.py" ]] && \
            grep -Fq '"with_ffmpeg": [True, False]' "${reusable_qt_recipe}/conanfile.py" && \
            grep -Fq 'tc.cache_variables["FFMPEG_DIR"]' "${reusable_qt_recipe}/conanfile.py" && \
            grep -Fq 'qtmultimedia_configure = os.path.join' "${reusable_qt_recipe}/conanfile.py" && \
            grep -Fq 'qt_find_package(FFmpeg MODULE OPTIONAL_COMPONENTS' "${reusable_qt_recipe}/conanfile.py" && \
            [[ "$(grep -Fc 'qt_find_package(VAAPI MODULE COMPONENTS' "${reusable_qt_recipe}/conanfile.py")" -ge 2 ]] && \
            grep -Fq 'find_package(ffmpeg CONFIG QUIET)' "${reusable_qt_recipe}/conanfile.py" && \
            grep -Fq 'TARGET ffmpeg::${_lowerComponent}' "${reusable_qt_recipe}/conanfile.py" && \
            grep -Fq '#ifndef V4L2_PIX_FMT_BGRA32' "${reusable_qt_recipe}/conanfile.py" && \
            grep -Fq '"ffmpeg::avcodec"' "${reusable_qt_recipe}/conanfile.py" && \
            grep -Fq 'if(NOT TARGET Qt::qsb)' "${reusable_qt_recipe}/conanfile.py" && \
            grep -Fq 'native_qsb_config = os.path.join' "${reusable_qt_recipe}/conanfile.py" && \
            grep -Fq 'tc.cache_variables["Qt6QuickTools_DIR"]' "${reusable_qt_recipe}/conanfile.py" && \
            grep -Fq 'native_quick_tools_config = os.path.join' "${reusable_qt_recipe}/conanfile.py" && \
            grep -Fq 'add_executable(Qt6::svgtoqml IMPORTED GLOBAL)' "${reusable_qt_recipe}/conanfile.py" && \
            grep -Fq 'native_svgtoqml_name = "svgtoqml.exe"' "${reusable_qt_recipe}/conanfile.py"; then
            if [[ "${qt_recipe_mode}" == "linux-audio" ]] && \
                { ! grep -Fq 'tc.variables["FEATURE_pulseaudio"] = "ON"' "${reusable_qt_recipe}/conanfile.py" || \
                  ! grep -Fq 'tc.variables["FEATURE_alsa"] = "OFF"' "${reusable_qt_recipe}/conanfile.py" || \
                  ! grep -Fq 'pulseaudio_finder = os.path.join' "${reusable_qt_recipe}/conanfile.py" || \
                  ! grep -Fq 'find_package(pulseaudio CONFIG QUIET)' "${reusable_qt_recipe}/conanfile.py" || \
                  grep -Fq 'OR QNX OR LINUX OR' "${reusable_qt_recipe}/conanfile.py"; }; then
                echo "Ignoring Qt recipe revision ${reusable_qt_recipe_revision}; its Linux audio configuration is stale"
            else
                echo "Reusing Qt recipe revision ${reusable_qt_recipe_revision} with complete ARM/native packages"
                exit 0
            fi
        fi
        echo "Ignoring stale Qt recipe revision ${reusable_qt_recipe_revision}; applying the current ARM cross-build patch"
    fi
fi

# A checkpoint can contain both ConanCenter's pristine recipe and an older
# locally exported patched revision. Resolve the upstream revision first and
# use that exact reference; an unqualified `conan cache path` may otherwise
# select the stale patched export from the checkpoint.
qt_recipe_revision="$(
    conan list 'qt/6.11.1:*' -r conancenter --format=json |
        "${python_command}" -c 'import json, sys; data = json.load(sys.stdin); revisions = data["conancenter"]["qt/6.11.1"]["revisions"]; print(max(revisions, key=lambda revision: revisions[revision].get("timestamp", 0)))'
)"
conan download "qt/6.11.1#${qt_recipe_revision}" --only-recipe --remote=conancenter
qt_recipe="$(conan cache path "qt/6.11.1#${qt_recipe_revision}")"
qt_recipe_copy="$(mktemp -d "${TMPDIR:-/tmp}/f4-qt-recipe.XXXXXX")"
cp "${qt_recipe}/conanfile.py" \
    "${qt_recipe}/conandata.yml" \
    "${qt_recipe}/qtmodules6.11.1.conf" \
    "${qt_recipe_copy}/"

if [[ "${qt_recipe_mode}" == "linux-audio" ]]; then
    "${python_command}" ci/patch-qt-dependencies.py \
        --linux-audio "${qt_recipe_copy}/conanfile.py"
else
    "${python_command}" ci/patch-qt-dependencies.py \
        "${qt_recipe_copy}/conanfile.py"
fi

if [[ "${target_arch}" == "arm64" ]]; then
    "${python_command}" ci/patch-qt-qmltools-recipe.py "${qt_recipe_copy}/conanfile.py"
    grep -Fq 'set(Qt6QmlTools_FOUND TRUE)' "${qt_recipe_copy}/conanfile.py"
    grep -Fq 'add_executable(Qt6::${_qt_qml_tool} IMPORTED GLOBAL)' "${qt_recipe_copy}/conanfile.py"
    grep -Fq 'qmljsrootgen' "${qt_recipe_copy}/conanfile.py"
    grep -Fq 'Qt6QmlToolsConfigVersion.cmake' "${qt_recipe_copy}/conanfile.py"
    grep -Fq 'set(PACKAGE_VERSION "6.11.1")' "${qt_recipe_copy}/conanfile.py"
    grep -Fq 'Qt6ShaderToolsToolsConfig.cmake' "${qt_recipe_copy}/conanfile.py"
    grep -Fq 'Qt6ShaderToolsToolsConfigVersion.cmake' "${qt_recipe_copy}/conanfile.py"
    grep -Fq 'set(Qt6ShaderToolsTools_FOUND TRUE)' "${qt_recipe_copy}/conanfile.py"
    grep -Fq 'add_executable(Qt6::qsb IMPORTED GLOBAL)' "${qt_recipe_copy}/conanfile.py"
    grep -Fq 'if(NOT TARGET Qt::qsb)' "${qt_recipe_copy}/conanfile.py"
    grep -Fq 'add_executable(Qt6::svgtoqml IMPORTED GLOBAL)' \
        "${qt_recipe_copy}/conanfile.py"
    grep -Fq 'Qt6SvgToQmlMacros.cmake' \
        "${qt_recipe_copy}/conanfile.py"
    grep -Fq 'tc.cache_variables["Qt6QuickTools_DIR"]' "${qt_recipe_copy}/conanfile.py"
    grep -Fq 'native_quick_tools_config = os.path.join' "${qt_recipe_copy}/conanfile.py"
    grep -Fq 'native_svgtoqml_name = "svgtoqml.exe"' "${qt_recipe_copy}/conanfile.py"
fi

grep -Fq 'self.requires("freetype/2.13.2")' \
    "${qt_recipe_copy}/conanfile.py"
grep -Fq '"with_ffmpeg": [True, False]' \
    "${qt_recipe_copy}/conanfile.py"
grep -Fq 'tc.cache_variables["FFMPEG_DIR"]' \
    "${qt_recipe_copy}/conanfile.py"
grep -Fq 'qtmultimedia_configure = os.path.join' \
    "${qt_recipe_copy}/conanfile.py"
grep -Fq 'qt_find_package(FFmpeg MODULE OPTIONAL_COMPONENTS' \
    "${qt_recipe_copy}/conanfile.py"
[[ "$(grep -Fc 'qt_find_package(VAAPI MODULE COMPONENTS' "${qt_recipe_copy}/conanfile.py")" -ge 2 ]]
grep -Fq 'find_package(ffmpeg CONFIG QUIET)' \
    "${qt_recipe_copy}/conanfile.py"
grep -Fq 'TARGET ffmpeg::${_lowerComponent}' \
    "${qt_recipe_copy}/conanfile.py"
grep -Fq '#ifndef V4L2_PIX_FMT_BGRA32' \
    "${qt_recipe_copy}/conanfile.py"
grep -Fq '"ffmpeg::avcodec"' \
    "${qt_recipe_copy}/conanfile.py"
if [[ "${qt_recipe_mode}" == "linux-audio" ]]; then
    grep -Fq 'tc.variables["FEATURE_pulseaudio"] = "ON"' \
        "${qt_recipe_copy}/conanfile.py"
    grep -Fq 'tc.variables["FEATURE_alsa"] = "OFF"' \
        "${qt_recipe_copy}/conanfile.py"
    grep -Fq 'pulseaudio_finder = os.path.join' \
        "${qt_recipe_copy}/conanfile.py"
    grep -Fq 'find_package(pulseaudio CONFIG QUIET)' \
        "${qt_recipe_copy}/conanfile.py"
    if grep -Fq 'OR QNX OR LINUX OR' "${qt_recipe_copy}/conanfile.py"; then
        echo "error: Linux audio recipe unexpectedly bypasses Qt's FFmpeg gate" >&2
        exit 1
    fi
fi
if [[ "${target_arch}" == "arm64" ]]; then
    grep -Fq 'QT_ADDITIONAL_PACKAGES_PREFIX_PATH' \
        "${qt_recipe_copy}/conanfile.py"
    grep -Fq 'missing_quick_libraries = []' \
        "${qt_recipe_copy}/conanfile.py"
fi
conan export "${qt_recipe_copy}" --name=qt --version=6.11.1
