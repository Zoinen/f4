#!/usr/bin/env bash
set -euo pipefail

# libiconv/1.17's Windows recipe adds a resource object even for static
# packages.  The generic windres on GitHub's Windows runner emits an x64 COFF
# object, which cannot be linked into the ARM64 target.  Keep the upstream
# recipe and source patch, but remove that unused object for static ARM64.
conan download libiconv/1.17 --only-recipe --remote=conancenter
libiconv_recipe="$(conan cache path libiconv/1.17 | tail -1)"
libiconv_recipe_copy="$(mktemp -d "${TMPDIR:-/tmp}/f4-libiconv-recipe.XXXXXX")"
cp "${libiconv_recipe}/conanfile.py" \
    "${libiconv_recipe}/conandata.yml" \
    "${libiconv_recipe_copy}/"
mkdir -p "${libiconv_recipe_copy}/patches"
cp ci/patches/libiconv-1.17-fix-error-function-declaration-without-prototype.patch \
    "${libiconv_recipe_copy}/patches/1.17-001-fix-error-function-declaration-without-prototype.patch"

python_command=python
if ! command -v "${python_command}" >/dev/null 2>&1; then
    python_command=python3
fi
"${python_command}" ci/patch-libiconv-recipe.py "${libiconv_recipe_copy}/conanfile.py"
grep -Fq 'Skipping the Windows ARM64 resource object for a static package' \
    "${libiconv_recipe_copy}/conanfile.py"
conan export "${libiconv_recipe_copy}" --name=libiconv --version=1.17
