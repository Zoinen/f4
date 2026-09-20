#!/usr/bin/env bash
set -euo pipefail

# Conan Center's libgettext/0.22 recipe leaves CPP at Conan's default for the
# MSVC/Autotools graph.  Gnulib then cannot discover the absolute names of
# MSVC system headers and generates #include <> wrappers.  Use cl's explicit
# preprocessor-output mode; this changes only the generated recipe revision.
conan download libgettext/0.22 --only-recipe --remote=conancenter
libgettext_recipe="$(conan cache path libgettext/0.22 | tail -1)"
libgettext_recipe_copy="$(mktemp -d "${TMPDIR:-/tmp}/f4-libgettext-recipe.XXXXXX")"
cp "${libgettext_recipe}/conanfile.py" \
    "${libgettext_recipe}/conandata.yml" \
    "${libgettext_recipe_copy}/"

python_command=python
if ! command -v "${python_command}" >/dev/null 2>&1; then
    python_command=python3
fi
"${python_command}" ci/patch-libgettext-recipe.py "${libgettext_recipe_copy}/conanfile.py"
grep -Fq 'env.define("CPP", "cl -nologo -EP")' "${libgettext_recipe_copy}/conanfile.py"
conan export "${libgettext_recipe_copy}" --name=libgettext --version=0.22
