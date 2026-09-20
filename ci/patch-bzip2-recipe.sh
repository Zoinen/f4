#!/usr/bin/env bash
set -euo pipefail

# Conan Center's bzip2/1.0.8 recipe still configures an old CMake project.
# Patch only the generated toolchain variable, preserving the upstream source,
# recipe metadata, and package identity apart from the intentional recipe
# revision change.  Always start from ConanCenter so a previously patched
# remote recipe remains idempotent and cannot accumulate edits.
conan download bzip2/1.0.8 --only-recipe --remote=conancenter
bzip2_recipe="$(conan cache path bzip2/1.0.8 | tail -1)"
bzip2_recipe_copy="$(mktemp -d "${TMPDIR:-/tmp}/f4-bzip2-recipe.XXXXXX")"
cp "${bzip2_recipe}/conanfile.py" "${bzip2_recipe}/conandata.yml" "${bzip2_recipe_copy}/"
cp "$(dirname "${bzip2_recipe}")/es/CMakeLists.txt" "${bzip2_recipe_copy}/"
python_command=python
if ! command -v "${python_command}" >/dev/null 2>&1; then
    python_command=python3
fi
"${python_command}" ci/patch-bzip2-recipe.py "${bzip2_recipe_copy}/conanfile.py"
conan export "${bzip2_recipe_copy}" --name=bzip2 --version=1.0.8
