#!/usr/bin/env bash
set -euo pipefail

# Start from ConanCenter's recipe on every run so an interrupted build cannot
# accumulate local edits.  The freetype pin applies to both the target and
# native build contexts of Qt's cross-build graph.  ARM64 additionally needs
# the deliberately small native QML and shader-tool exports used by the target build.
target_arch="${1:-}"
python_command=python
if ! command -v "${python_command}" >/dev/null 2>&1; then
    python_command=python3
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

def has_arch(revision, arch):
    return any(
        package.get("info", {}).get("settings", {}).get("arch") == arch
        for package in revision.get("packages", {}).values()
    )

candidates = [
    (revision_id, revision)
    for revision_id, revision in revisions.items()
    if has_arch(revision, "armv8") and has_arch(revision, "x86_64")
]
if candidates:
    revision_id, _ = max(
        candidates,
        key=lambda item: item[1].get("timestamp", 0),
    )
    print(revision_id)
')"
    if [[ -n "${reusable_qt_recipe_revision}" ]]; then
        conan download "qt/6.11.1#${reusable_qt_recipe_revision}" \
            --only-recipe --remote=f4-conan
        echo "Reusing Qt recipe revision ${reusable_qt_recipe_revision} with complete ARM/native packages"
        exit 0
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

"${python_command}" ci/patch-qt-dependencies.py "${qt_recipe_copy}/conanfile.py"

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
fi

grep -Fq 'self.requires("freetype/2.13.2")' \
    "${qt_recipe_copy}/conanfile.py"
if [[ "${target_arch}" == "arm64" ]]; then
    grep -Fq 'QT_ADDITIONAL_PACKAGES_PREFIX_PATH' \
        "${qt_recipe_copy}/conanfile.py"
    grep -Fq 'missing_quick_libraries = []' \
        "${qt_recipe_copy}/conanfile.py"
fi
conan export "${qt_recipe_copy}" --name=qt --version=6.11.1
