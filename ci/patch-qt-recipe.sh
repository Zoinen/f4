#!/usr/bin/env bash
set -euo pipefail

# Start from ConanCenter's recipe on every run so an interrupted build cannot
# accumulate local edits.  The freetype pin applies to both the target and
# native build contexts of Qt's cross-build graph.  ARM64 additionally needs
# the deliberately small native QML and shader-tool exports used by the target build.
target_arch="${1:-}"
conan download qt/6.11.1 --only-recipe --remote=conancenter
qt_recipe="$(conan cache path qt/6.11.1 | tail -1)"
qt_recipe_copy="$(mktemp -d "${TMPDIR:-/tmp}/f4-qt-recipe.XXXXXX")"
cp "${qt_recipe}/conanfile.py" \
    "${qt_recipe}/conandata.yml" \
    "${qt_recipe}/qtmodules6.11.1.conf" \
    "${qt_recipe_copy}/"

python_command=python
if ! command -v "${python_command}" >/dev/null 2>&1; then
    python_command=python3
fi
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
