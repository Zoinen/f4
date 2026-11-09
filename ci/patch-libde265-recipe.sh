#!/usr/bin/env bash
set -euo pipefail

# ConanCenter's libde265/1.1.3 recipe also builds the optional dec265 command
# line tool.  Its bundled getopt declaration uses a non-const option string,
# which GCC accepts but MSVC rejects under /Zc:strictStrings.  The Qt host only
# needs the codec library, so disable that example before exporting the recipe.
python_command=python
if ! command -v "${python_command}" >/dev/null 2>&1; then
    python_command=python3
fi
libde265_recipe_revision="$({
    conan list 'libde265/1.1.3:*' -r conancenter --format=json 2>/dev/null || true
} | "${python_command}" -c 'import json, sys; data = json.load(sys.stdin); revisions = data["conancenter"]["libde265/1.1.3"]["revisions"]; print(max(revisions, key=lambda revision: revisions[revision].get("timestamp", 0)))')"
conan download "libde265/1.1.3#${libde265_recipe_revision}" --only-recipe --remote=conancenter
libde265_recipe="$(conan cache path "libde265/1.1.3#${libde265_recipe_revision}")"
libde265_recipe_copy="$(mktemp -d "${TMPDIR:-/tmp}/f4-libde265-recipe.XXXXXX")"
cp "${libde265_recipe}/conanfile.py" \
    "${libde265_recipe}/conandata.yml" \
    "${libde265_recipe_copy}/"
if [[ -d "${libde265_recipe}/patches" ]]; then
    cp -R "${libde265_recipe}/patches" "${libde265_recipe_copy}/"
fi

"${python_command}" ci/patch-libde265-recipe.py "${libde265_recipe_copy}/conanfile.py"
grep -Fq 'tc.variables["ENABLE_DECODER"] = False' \
    "${libde265_recipe_copy}/conanfile.py"
conan export "${libde265_recipe_copy}" --name=libde265 --version=1.1.3
