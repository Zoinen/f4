#!/usr/bin/env bash
set -euo pipefail

# Conan Center's libffi/3.4.4 recipe can infer an x86 host triplet for the
# Windows ARM64 MSVC/Autotools graph.  Patch only that generated configure
# argument so libffi selects its ARM64 assembler; the package settings and
# package identity remain unchanged apart from the intentional recipe revision.
conan download libffi/3.4.4 --only-recipe --remote=conancenter
libffi_recipe="$(conan cache path libffi/3.4.4 | tail -1)"
libffi_recipe_copy="$(mktemp -d "${TMPDIR:-/tmp}/f4-libffi-recipe.XXXXXX")"
cp "${libffi_recipe}/conanfile.py" \
    "${libffi_recipe}/conandata.yml" \
    "${libffi_recipe_copy}/"
cp -R "$(dirname "${libffi_recipe}")/es/patches" "${libffi_recipe_copy}/"

python_command=python
if ! command -v "${python_command}" >/dev/null 2>&1; then
    python_command=python3
fi
"${python_command}" ci/patch-libffi-recipe.py "${libffi_recipe_copy}/conanfile.py"
grep -Fq 'aarch64-win64-mingw64' "${libffi_recipe_copy}/conanfile.py"
conan export "${libffi_recipe_copy}" --name=libffi --version=3.4.4
