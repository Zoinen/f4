#!/usr/bin/env bash
set -euo pipefail

# Conan Center's libgettext/0.22 recipe leaves CPP at Conan's default for the
# MSVC/Autotools graph.  Gnulib then cannot discover the absolute names of
# MSVC system headers and generates #include <> wrappers.  Use cl's /E mode,
# which preserves #line markers; /EP suppresses them.  Normalize an older
# cached export in place before exporting the patched recipe revision.
python_command=python
if ! command -v "${python_command}" >/dev/null 2>&1; then
    python_command=python3
fi
libgettext_recipe_revision="$(
    conan list 'libgettext/0.22:*' -r conancenter --format=json |
        "${python_command}" -c 'import json, sys; data = json.load(sys.stdin); revisions = data["conancenter"]["libgettext/0.22"]["revisions"]; print(max(revisions, key=lambda revision: revisions[revision].get("timestamp", 0)))'
)"
conan download "libgettext/0.22#${libgettext_recipe_revision}" --only-recipe --remote=conancenter
libgettext_recipe="$(conan cache path "libgettext/0.22#${libgettext_recipe_revision}")"
libgettext_recipe_copy="$(mktemp -d "${TMPDIR:-/tmp}/f4-libgettext-recipe.XXXXXX")"
cp "${libgettext_recipe}/conanfile.py" \
    "${libgettext_recipe}/conandata.yml" \
    "${libgettext_recipe_copy}/"

"${python_command}" ci/patch-libgettext-recipe.py "${libgettext_recipe_copy}/conanfile.py"
grep -Fq 'env.define("CPP", "cl -nologo -E")' "${libgettext_recipe_copy}/conanfile.py"
grep -Fq 'env.define("CXXCPP", "cl -nologo -E")' "${libgettext_recipe_copy}/conanfile.py"
conan export "${libgettext_recipe_copy}" --name=libgettext --version=0.22
