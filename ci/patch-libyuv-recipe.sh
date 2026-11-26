#!/usr/bin/env bash
set -euo pipefail

python_command=python
if ! command -v "${python_command}" >/dev/null 2>&1; then
    python_command=python3
fi

libyuv_recipe_revision="$(
    conan list 'libyuv/1892:*' -r conancenter --format=json |
        "${python_command}" -c 'import json, sys; data = json.load(sys.stdin); revisions = data["conancenter"]["libyuv/1892"]["revisions"]; print(max(revisions, key=lambda revision: revisions[revision].get("timestamp", 0)))'
)"
libyuv_ref="libyuv/1892#${libyuv_recipe_revision}"
conan download "${libyuv_ref}" --only-recipe --remote=conancenter
libyuv_recipe="$(conan cache path "${libyuv_ref}" | tail -n 1)"
libyuv_export_sources="$(
    conan cache path "${libyuv_ref}" --folder=export_source | tail -n 1
)"
libyuv_recipe_copy="$(mktemp -d "${TMPDIR:-/tmp}/f4-libyuv-recipe.XXXXXX")"
cp "${libyuv_recipe}/conanfile.py" \
    "${libyuv_recipe}/conandata.yml" \
    "${libyuv_recipe_copy}/"
if [[ ! -d "${libyuv_export_sources}/patches" ]]; then
    echo "error: Conan cache is missing libyuv exported patches" >&2
    exit 1
fi
cp -R "${libyuv_export_sources}/patches" "${libyuv_recipe_copy}/"

"${python_command}" ci/patch-libyuv-recipe.py \
    "${libyuv_recipe_copy}/conanfile.py"
grep -Fq 'f4: the glibc-2.27 ARM64 toolchain supports dot-product' \
    "${libyuv_recipe_copy}/conanfile.py"
conan export "${libyuv_recipe_copy}" --name=libyuv --version=1892
