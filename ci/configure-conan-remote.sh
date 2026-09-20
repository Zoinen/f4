#!/usr/bin/env bash
set -euo pipefail

# The Artifactory virtual repository is deliberately optional.  This keeps
# local builds and forked pull requests working when the repository variables
# or credentials are not present.  ConanCenter stays authoritative for recipe
# revisions: the Artifactory virtual remote is a secondary binary/cache source
# and may contain an older cached copy of a ConanCenter recipe.
conan_home="${CONAN_HOME:-$(conan config home)}"
global_conf="${conan_home}/global.conf"
cmake_policy_conf='*/*:tools.cmake.cmaketoolchain:extra_variables={"CMAKE_POLICY_VERSION_MINIMUM":{"cache":True,"type":"STRING","value":"3.5"}}'
cmake_configure_args='*/*:tools.cmake:configure_args=["-DCMAKE_POLICY_VERSION_MINIMUM=3.5"]'
mkdir -p "${conan_home}"
for conan_conf in "${cmake_policy_conf}" "${cmake_configure_args}"; do
    if [[ ! -f "${global_conf}" ]] || ! grep -Fqx "${conan_conf}" "${global_conf}"; then
        printf '%s\n' "${conan_conf}" >> "${global_conf}"
    fi
done

remote_url="${F4_CONAN_REMOTE_URL:-}"
if [[ -z "${remote_url}" ]]; then
    echo "Conan binary remote is not configured; using the existing remotes"
    exit 0
fi

remote_name="${F4_CONAN_REMOTE_NAME:-f4-conan}"
conan remote add "${remote_name}" "${remote_url}" --index 1 --force

if [[ -n "${F4_CONAN_TOKEN:-}" ]]; then
    conan remote login "${remote_name}" "${F4_CONAN_USERNAME:-admin}" \
        --password "${F4_CONAN_TOKEN}"
else
    echo "Conan binary remote configured without credentials (read-only mode)"
fi
